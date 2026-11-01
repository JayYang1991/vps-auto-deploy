package singbox

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/pkg/executil"
)

const (
	DriverName  = "sing-box"
	Owner       = "SagerNet"
	Repo        = "sing-box"
	BinaryPath  = "/usr/local/bin/sing-box"
	UnitName    = "sing-box.service"
	ConfigPath  = "/etc/sing-box/config.json"
)

type Driver struct {
	systemd *component.SystemdManager
}

func NewDriver() *Driver {
	return &Driver{
		systemd: component.NewSystemdManager(UnitName),
	}
}

func (d *Driver) Name() string {
	return DriverName
}

func (d *Driver) GetInstalledVersion() (string, error) {
	if _, err := os.Stat(BinaryPath); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	out, err := executil.RunCommand(3*time.Second, BinaryPath, "version")
	if err != nil {
		return "", err
	}
	// "sing-box version 1.11.0 ..."
	lines := strings.Split(out, "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			return parts[2], nil
		}
	}
	return strings.TrimSpace(out), nil
}

func (d *Driver) GetLatestRelease() (string, error) {
	rel, err := component.FetchLatestRelease(Owner, Repo)
	if err != nil {
		return "", err
	}
	return rel.TagName, nil
}

func (d *Driver) InstallOrUpgrade(version string) error {
	rel, err := component.FetchLatestRelease(Owner, Repo)
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}

	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported arch: %s", arch)
	}

	// 匹配类似 sing-box-1.11.0-linux-amd64.tar.gz
	var downloadURL string
	pattern := fmt.Sprintf("linux-%s.tar.gz", arch)
	for _, asset := range rel.Assets {
		if strings.Contains(asset.Name, pattern) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("asset for arch %s not found in release %s", arch, rel.TagName)
	}

	tmpArchive := filepath.Join(os.TempDir(), fmt.Sprintf("singbox-%s.tar.gz", rel.TagName))
	defer os.Remove(tmpArchive)

	if err := component.DownloadFile(downloadURL, tmpArchive); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// 解压 tar.gz 获取 sing-box 二进制
	tmpBinary, err := extractBinary(tmpArchive, "sing-box")
	if err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	defer os.Remove(tmpBinary)

	// 确保 /usr/local/bin 存在
	_ = os.MkdirAll(filepath.Dir(BinaryPath), 0755)

	// 移动并替换
	if err := os.Rename(tmpBinary, BinaryPath); err != nil {
		// 跨分区则使用拷贝
		input, err := os.ReadFile(tmpBinary)
		if err != nil {
			return err
		}
		if err := os.WriteFile(BinaryPath, input, 0755); err != nil {
			return fmt.Errorf("write binary error: %w", err)
		}
	}
	_ = os.Chmod(BinaryPath, 0755)

	// 配置 /etc/sing-box 目录
	_ = os.MkdirAll(filepath.Dir(ConfigPath), 0755)

	// 写入 Systemd 服务文件
	unitContent := fmt.Sprintf(`[Unit]
Description=sing-box service
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target network-online.target

[Service]
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
ExecStart=%s run -c %s
Restart=on-failure
RestartSec=10s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`, BinaryPath, ConfigPath)

	_ = component.WriteServiceUnit(UnitName, unitContent)
	_ = d.systemd.Enable()

	return nil
}

func (d *Driver) Status() (component.ServiceStatus, error) {
	installedVer, _ := d.GetInstalledVersion()
	installed := installedVer != ""
	active := false
	runningStatus := "not_installed"

	if installed {
		active = d.systemd.IsActive()
		if active {
			runningStatus = "running"
		} else {
			runningStatus = d.systemd.Status()
			if runningStatus == "" {
				runningStatus = "stopped"
			}
		}
	}

	return component.ServiceStatus{
		Name:             DriverName,
		Installed:        installed,
		InstalledVersion: installedVer,
		Active:           active,
		RunningStatus:    runningStatus,
	}, nil
}

func (d *Driver) Start() error {
	return d.systemd.Start()
}

func (d *Driver) Stop() error {
	return d.systemd.Stop()
}

func (d *Driver) Restart() error {
	return d.systemd.Restart()
}

func (d *Driver) ReloadOrRestart() error {
	return d.systemd.ReloadOrRestart()
}

// CheckConfig 校验配置文件格式是否合法
func CheckConfig(cfgPath string) error {
	out, err := executil.RunCommand(5*time.Second, BinaryPath, "check", "-c", cfgPath)
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), out)
	}
	return nil
}

// GenerateRealityKeypair 生成 Reality 公私钥对与随机 short_id
func GenerateRealityKeypair() (privateKey, publicKey, shortID string, err error) {
	// 优先调用系统已安装的 sing-box generate reality-keypair
	if _, statErr := os.Stat(BinaryPath); statErr == nil {
		out, runErr := executil.RunCommand(3*time.Second, BinaryPath, "generate", "reality-keypair")
		if runErr == nil {
			lines := strings.Split(out, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "PrivateKey:") {
					privateKey = strings.TrimSpace(strings.TrimPrefix(line, "PrivateKey:"))
				} else if strings.HasPrefix(line, "PublicKey:") {
					publicKey = strings.TrimSpace(strings.TrimPrefix(line, "PublicKey:"))
				}
			}
		}
	}

	// 纯 Go 标准库 X25519 降级实现
	if privateKey == "" || publicKey == "" {
		curve := ecdh.X25519()
		priv, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			return "", "", "", err
		}
		privateKey = base64.RawURLEncoding.EncodeToString(priv.Bytes())
		publicKey = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	}

	// 生成 8 字节 (16 hex) 或 4 字节 (8 hex) ShortID
	sidBytes := make([]byte, 4)
	_, _ = rand.Read(sidBytes)
	shortID = hex.EncodeToString(sidBytes)

	return privateKey, publicKey, shortID, nil
}

func extractBinary(archivePath, targetName string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == targetName {
			tmpTarget := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d", targetName, time.Now().UnixNano()))
			out, err := os.OpenFile(tmpTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return tmpTarget, nil
		}
	}

	return "", fmt.Errorf("%s not found in archive", targetName)
}

