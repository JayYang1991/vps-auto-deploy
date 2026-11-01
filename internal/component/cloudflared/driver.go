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

func (d *Driver) GetInstalledVersion() (string, error) {
	if _, err := os.Stat(BinaryPath); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	out, err := executil.RunCommand(3*time.Second, BinaryPath, "version")
	if err != nil {
		return "", err
	}
	// "cloudflared version 2024.10.0 (built ...)"
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

	// 匹配 cloudflared-linux-amd64 / cloudflared-linux-arm64
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

	if err := component.DownloadFile(downloadURL, tmpBinary); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	_ = os.MkdirAll(filepath.Dir(BinaryPath), 0755)

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

	return nil
}

// ConfigureService 依据隧道模式生成并配置 Systemd 单元
func (d *Driver) ConfigureService(mode, token string, localPort int, protocol string) error {
	var execArgs string
	if mode == "token" && token != "" {
		protoArg := ""
		if protocol != "" && protocol != "auto" {
			protoArg = fmt.Sprintf("--protocol %s ", protocol)
		}
		execArgs = fmt.Sprintf("tunnel %srun --token %s", protoArg, token)
	} else {
		// Quick Tunnel
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

