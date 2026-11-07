package singbox

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	DriverName = "sing-box"
	Owner      = "SagerNet"
	Repo       = "sing-box"
	UnitName   = "sing-box.service"
)

var (
	ConfigPath = GetConfigPath()
)

func GetConfigPath() string {
	return component.GetDefaultConfigPath(DriverName, "config.json")
}

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

func (d *Driver) DisplayName() string {
	return "sing-box 代理核心"
}

func (d *Driver) Description() string {
	return "通用代理与路由平台，原生支持 VLESS-Reality、Hysteria2、TUIC、Shadowsocks 等全协议。"
}

func (d *Driver) Category() string {
	return "proxy"
}

func GetBinaryPath() string {
	return component.FindBinary(DriverName, component.GetDefaultBinaryPath(DriverName), "/usr/local/bin/sing-box", "/usr/bin/sing-box", "/bin/sing-box", "/snap/bin/sing-box")
}

func (d *Driver) GetInstalledVersion() (string, error) {
	bin := GetBinaryPath()
	if bin == "" {
		return "", nil
	}
	out, err := executil.RunCommand(3*time.Second, bin, "version")
	if err != nil {
		return "", err
	}
	return component.ExtractVersion(out), nil
}

func (d *Driver) GetLatestRelease() (string, error) {
	rel, err := component.FetchLatestRelease(Owner, Repo)
	if err != nil {
		return "", err
	}
	return rel.TagName, nil
}

func (d *Driver) InstallOrUpgrade(version string, logFn func(string)) error {
	if logFn == nil {
		logFn = func(string) {}
	}

	logFn(fmt.Sprintf("正在查询 GitHub 仓库 %s/%s 最新版本信息...", Owner, Repo))
	rel, err := component.FetchLatestRelease(Owner, Repo)
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}

	targetVersion := rel.TagName
	if version != "" && version != "latest" {
		targetVersion = version
	}
	logFn(fmt.Sprintf("已确定目标版本: %s", targetVersion))

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

	logFn(fmt.Sprintf("开始下载资产: %s", downloadURL))
	if err := component.DownloadFile(downloadURL, tmpArchive); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	logFn("资产下载完成，开始解压与校验...")

	tmpBinary, err := extractBinary(tmpArchive, "sing-box")
	if err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	defer os.Remove(tmpBinary)

	targetBinary := component.GetDefaultBinaryPath(DriverName)
	targetConfig := GetConfigPath()

	_ = os.MkdirAll(filepath.Dir(targetBinary), 0755)

	logFn(fmt.Sprintf("安装二进制文件至 %s...", targetBinary))
	if err := os.Rename(tmpBinary, targetBinary); err != nil {
		input, err := os.ReadFile(tmpBinary)
		if err != nil {
			return err
		}
		if err := os.WriteFile(targetBinary, input, 0755); err != nil {
			return fmt.Errorf("write binary error: %w", err)
		}
	}
	_ = os.Chmod(targetBinary, 0755)

	_ = os.MkdirAll(filepath.Dir(targetConfig), 0755)

	logFn("配置并更新 Systemd 服务单元...")
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
`, targetBinary, targetConfig)

	if err := component.WriteServiceUnit(UnitName, unitContent); err != nil {
		logFn(fmt.Sprintf("[WARN] 写入服务单元失败: %v", err))
	}
	_ = d.systemd.Enable()

	// 尝试启动服务（若配置文件已存在）
	if _, err := os.Stat(targetConfig); err == nil {
		logFn("检测到现有配置，正在启动 sing-box 服务...")
		_ = d.systemd.Restart()
	}

	component.SetCachedLatestVersion(DriverName, rel.TagName)
	logFn("sing-box 安装/升级成功！")
	return nil
}

func (d *Driver) Uninstall() error {
	_ = d.systemd.Stop()
	_ = d.systemd.Disable()
	_ = component.RemoveServiceUnit(UnitName)
	if bin := GetBinaryPath(); bin != "" {
		_ = os.Remove(bin)
	}
	return nil
}

func (d *Driver) Status() (component.ServiceStatus, error) {
	installedVer, _ := d.GetInstalledVersion()
	installed := installedVer != ""
	active := false
	runningStatus := "not_installed"

	if installed {
		active = d.systemd.IsActive() || component.IsProcessRunning("sing-box")
		if active {
			runningStatus = "running"
		} else {
			runningStatus = d.systemd.Status()
			if runningStatus == "" || runningStatus == "unknown" {
				runningStatus = "stopped"
			}
		}
	}

	return component.ServiceStatus{
		Name:             DriverName,
		DisplayName:      d.DisplayName(),
		Description:      d.Description(),
		Category:         d.Category(),
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
	bin := GetBinaryPath()
	if bin == "" {
		return fmt.Errorf("sing-box is not installed yet")
	}
	out, err := executil.RunCommand(5*time.Second, bin, "check", "-c", cfgPath)
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), out)
	}
	return nil
}

// GenerateRealityKeypair 生成 Reality 公私钥对与随机 short_id
func GenerateRealityKeypair() (privateKey, publicKey, shortID string, err error) {
	if bin := GetBinaryPath(); bin != "" {
		out, runErr := executil.RunCommand(3*time.Second, bin, "generate", "reality-keypair")
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

	if privateKey == "" || publicKey == "" {
		curve := ecdh.X25519()
		priv, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			return "", "", "", err
		}
		privateKey = base64.RawURLEncoding.EncodeToString(priv.Bytes())
		publicKey = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	}

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
