package cloudflared

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/pkg/executil"
)

const (
	DriverName = "cloudflared"
	Owner      = "cloudflare"
	Repo       = "cloudflared"
	BinaryPath = "/usr/local/bin/cloudflared"
	UnitName   = "cloudflared.service"
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

func (d *Driver) DisplayName() string {
	return "Cloudflared Argo 隧道"
}

func (d *Driver) Description() string {
	return "Cloudflare Zero Trust / Argo 隧道反代客户端，穿透并拯救被墙阻断的公网 IP。"
}

func (d *Driver) Category() string {
	return "tunnel"
}

func (d *Driver) GetInstalledVersion() (string, error) {
	if _, err := os.Stat(BinaryPath); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	out, err := executil.RunCommand(3*time.Second, BinaryPath, "version")
	if err != nil {
		return "", err
	}
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

	var downloadURL string
	targetName := fmt.Sprintf("cloudflared-linux-%s", arch)
	for _, asset := range rel.Assets {
		if asset.Name == targetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("asset %s not found in release %s", targetName, rel.TagName)
	}

	tmpBinary := filepath.Join(os.TempDir(), fmt.Sprintf("cloudflared-%s", rel.TagName))
	defer os.Remove(tmpBinary)

	logFn(fmt.Sprintf("开始下载资产: %s", downloadURL))
	if err := component.DownloadFile(downloadURL, tmpBinary); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	logFn("资产下载完成，开始安装...")

	_ = os.MkdirAll(filepath.Dir(BinaryPath), 0755)

	logFn(fmt.Sprintf("安装二进制文件至 %s...", BinaryPath))
	if err := os.Rename(tmpBinary, BinaryPath); err != nil {
		input, err := os.ReadFile(tmpBinary)
		if err != nil {
			return err
		}
		if err := os.WriteFile(BinaryPath, input, 0755); err != nil {
			return fmt.Errorf("write binary error: %w", err)
		}
	}
	_ = os.Chmod(BinaryPath, 0755)

	component.SetCachedLatestVersion(DriverName, rel.TagName)
	logFn("cloudflared 安装/升级成功！可在「Argo 隧道」页面配置并启动隧道。")
	return nil
}

func (d *Driver) Uninstall() error {
	_ = d.systemd.Stop()
	_ = d.systemd.Disable()
	_ = component.RemoveServiceUnit(UnitName)
	_ = os.Remove(BinaryPath)
	return nil
}

func (d *Driver) ConfigureService(mode, token string, localPort int, protocol string) error {
	var execArgs string
	if mode == "token" && token != "" {
		protoArg := ""
		if protocol != "" && protocol != "auto" {
			protoArg = fmt.Sprintf("--protocol %s ", protocol)
		}
		execArgs = fmt.Sprintf("tunnel %srun --token %s", protoArg, token)
	} else {
		execArgs = fmt.Sprintf("tunnel --url http://127.0.0.1:%d --no-autoupdate", localPort)
	}

	unitContent := fmt.Sprintf(`[Unit]
Description=Cloudflare Tunnel
After=network.target network-online.target

[Service]
TimeoutStartSec=0
Type=notify
ExecStart=%s %s
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, BinaryPath, execArgs)

	if err := component.WriteServiceUnit(UnitName, unitContent); err != nil {
		return err
	}

	return d.systemd.Enable()
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
