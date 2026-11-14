package cloudflared

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

func GetBinaryPath() string {
	return component.FindBinary(DriverName, component.GetDefaultBinaryPath(DriverName), "/usr/local/bin/cloudflared", "/usr/bin/cloudflared", "/bin/cloudflared")
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

	// 抓取并解析 Release 官方 Checksums (防软件供应链投毒)
	logFn("正在获取官方 Release 校验和清单...")
	checksums := component.FetchReleaseChecksums(rel)
	expectedHash := checksums[targetName]
	if expectedHash != "" {
		logFn(fmt.Sprintf("已识别到官方 SHA-256 签名: %s...", expectedHash[:16]))
	}

	tmpBinary := filepath.Join(os.TempDir(), fmt.Sprintf("cloudflared-%s", rel.TagName))
	defer os.Remove(tmpBinary)

	logFn(fmt.Sprintf("开始下载资产并强校验哈希: %s", downloadURL))
	if err := component.DownloadFileWithChecksum(downloadURL, tmpBinary, expectedHash); err != nil {
		return fmt.Errorf("download/verify failed: %w", err)
	}
	logFn("资产下载并完成供应链完整性校验，开始安装...")

	targetBinary := filepath.Clean(component.GetDefaultBinaryPath(DriverName))
	_ = os.MkdirAll(filepath.Dir(targetBinary), 0750)

	logFn(fmt.Sprintf("安装二进制文件至 %s...", targetBinary))
	if err := os.Rename(tmpBinary, targetBinary); err != nil {
		/* #nosec G304 - reading validated downloaded binary from temp path */
		input, err := os.ReadFile(tmpBinary)
		if err != nil {
			return err
		}
		/* #nosec G703, G306 - binary executable requires execute permission */
		if err := os.WriteFile(targetBinary, input, 0755); err != nil {
			return fmt.Errorf("write binary error: %w", err)
		}
	}
	/* #nosec G302 - binary executable requires execute permission */
	_ = os.Chmod(targetBinary, 0755)

	component.SetCachedLatestVersion(DriverName, rel.TagName)
	logFn("cloudflared 安装/升级成功！可在「Argo 隧道」页面配置并启动隧道。")
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

func (d *Driver) ConfigureService(mode, token string, localPort int, protocol string) error {
	bin := GetBinaryPath()
	if bin == "" {
		bin = component.GetDefaultBinaryPath(DriverName)
	}

	cleanMode := strings.ToLower(strings.TrimSpace(mode))
	if cleanMode != "token" && cleanMode != "quick" {
		cleanMode = "quick"
	}

	cleanProto := strings.ToLower(strings.TrimSpace(protocol))
	if cleanProto == "" {
		cleanProto = "auto"
	}
	if cleanProto != "auto" && cleanProto != "quic" && cleanProto != "http2" {
		return fmt.Errorf("非法 protocol 传输协议: %s (仅允许 auto, quic, http2)", protocol)
	}

	if localPort <= 0 || localPort > 65535 {
		return fmt.Errorf("非法本地端口: %d", localPort)
	}

	var execArgs string
	var envLine string
	if cleanMode == "token" && token != "" {
		cleanToken := strings.TrimSpace(token)
		// 阻断换行或控制字符，防止 Systemd Unit 文件注入
		if !regexp.MustCompile(`^[A-Za-z0-9_\-\.]{10,512}$`).MatchString(cleanToken) {
			return errors.New("非法 Cloudflared Tunnel Token 格式 (仅允许字母、数字、短横线、下划线与点)")
		}
		protoArg := ""
		if cleanProto != "auto" {
			protoArg = fmt.Sprintf("--protocol %s ", cleanProto)
		}
		// 使用环境变量传递 Token，避免在命令行 /proc 进程树中暴露凭据
		envLine = fmt.Sprintf("Environment=\"TUNNEL_TOKEN=%s\"\n", cleanToken)
		execArgs = fmt.Sprintf("tunnel %srun", protoArg)
	} else {
		execArgs = fmt.Sprintf("tunnel --url http://127.0.0.1:%d --no-autoupdate", localPort)
	}

	unitContent := fmt.Sprintf(`[Unit]
Description=Cloudflare Tunnel
After=network.target network-online.target

[Service]
NoNewPrivileges=true
PrivateTmp=true
TimeoutStartSec=0
Type=notify
%sExecStart=%s %s
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, envLine, bin, strings.TrimSpace(execArgs))

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
		active = d.systemd.IsActive() || component.IsProcessRunning("cloudflared")
		if active {
			runningStatus = "running"
		} else {
			rawStatus := strings.TrimSpace(d.systemd.Status())
			switch rawStatus {
			case "active":
				runningStatus = "running"
				active = true
			case "inactive", "stopped", "":
				runningStatus = "stopped"
			case "activating":
				runningStatus = "activating"
			case "deactivating":
				runningStatus = "deactivating"
			case "failed":
				runningStatus = "failed"
			default:
				runningStatus = rawStatus
			}
		}
	}

	return component.ServiceStatus{
		Name:             DriverName,
		DisplayName:      d.DisplayName(),
		Description:      d.Description(),
		Category:         d.Category(),
		CategoryLabel:    "隧道穿透",
		ConfigRoute:      "/services?tab=cloudflared",
		Order:            2,
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
