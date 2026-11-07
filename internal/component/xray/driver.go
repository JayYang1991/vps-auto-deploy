package xray

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/pkg/executil"
)

const (
	DriverName = "xray"
	Owner      = "XTLS"
	Repo       = "Xray-core"
	BinaryPath = "/usr/local/bin/xray"
	UnitName   = "xray.service"
	ConfigPath = "/etc/xray/config.json"
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
	return "Xray 代理核心"
}

func (d *Driver) Description() string {
	return "支持 VLESS、VMess、Trojan、Shadowsocks 等协议的通用网络工具。"
}

func (d *Driver) Category() string {
	return "proxy"
}

func GetBinaryPath() string {
	return component.FindBinary("xray", BinaryPath, "/usr/bin/xray", "/bin/xray")
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
	var targetAsset string
	if arch == "amd64" {
		targetAsset = "Xray-linux-64.zip"
	} else if arch == "arm64" {
		targetAsset = "Xray-linux-arm64-v8a.zip"
	} else {
		return fmt.Errorf("unsupported arch: %s", arch)
	}

	var downloadURL string
	for _, asset := range rel.Assets {
		if asset.Name == targetAsset {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("asset %s not found in release %s", targetAsset, rel.TagName)
	}

	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("xray-%s.zip", rel.TagName))
	defer os.Remove(tmpZip)

	logFn(fmt.Sprintf("开始下载资产: %s", downloadURL))
	if err := component.DownloadFile(downloadURL, tmpZip); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	logFn("资产下载完成，开始解压与安装...")

	// 解压 zip 提取 xray 二进制
	zr, err := zip.OpenReader(tmpZip)
	if err != nil {
		return fmt.Errorf("open zip failed: %w", err)
	}
	defer zr.Close()

	var extracted bool
	_ = os.MkdirAll(filepath.Dir(BinaryPath), 0755)

	for _, f := range zr.File {
		if f.Name == "xray" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(BinaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			out.Close()
			rc.Close()
			if err != nil {
				return err
			}
			extracted = true
			break
		}
	}

	if !extracted {
		return fmt.Errorf("xray binary not found in zip archive")
	}

	_ = os.Chmod(BinaryPath, 0755)
	_ = os.MkdirAll(filepath.Dir(ConfigPath), 0755)

	logFn("注册并配置 Systemd 服务单元...")
	unitContent := fmt.Sprintf(`[Unit]
Description=Xray Service
Documentation=https://github.com/xtls
After=network.target nss-lookup.target

[Service]
User=root
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ExecStart=%s run -config %s
Restart=on-failure
RestartPreventExitStatus=23
LimitNPROC=10000
LimitNOFILE=1000000

[Install]
WantedBy=multi-user.target
`, BinaryPath, ConfigPath)

	if err := component.WriteServiceUnit(UnitName, unitContent); err != nil {
		logFn(fmt.Sprintf("[WARN] 写入服务单元失败: %v", err))
	}
	_ = d.systemd.Enable()

	component.SetCachedLatestVersion(DriverName, rel.TagName)
	logFn("Xray 安装/升级成功！")
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
		active = d.systemd.IsActive() || component.IsProcessRunning("xray")
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

