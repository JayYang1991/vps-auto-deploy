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
	ConfigPath string
)

func init() {
	ConfigPath = GetConfigPath()
}

func findRunningConfigPath() string {
	out, err := executil.RunCommand(2*time.Second, "pgrep", "-x", DriverName)
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	pids := strings.Fields(out)
	for _, pid := range pids {
		/* #nosec G304 - reading proc cmdline of discovered PID */
		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(string(cmdlineBytes), "\x00")
		for i, arg := range args {
			if arg == "-c" && i+1 < len(args) {
				candidate := filepath.Clean(strings.TrimSpace(args[i+1]))
				if candidate != "" && filepath.IsAbs(candidate) {
					/* #nosec G703 */
					if _, err := os.Stat(candidate); err == nil {
						return candidate
					}
				}
			}
		}
	}
	return ""
}

// SetConfigPathForTest 供单元测试显式指定配置文件路径
func SetConfigPathForTest(p string) {
	ConfigPath = p
}

func GetConfigPath() string {
	if ConfigPath != "" {
		return ConfigPath
	}

	// 1. 若宿主机当前已有运行中的 sing-box 进程，优先继承其实际读取的配置文件路径
	if runningPath := findRunningConfigPath(); runningPath != "" {
		ConfigPath = runningPath
		return runningPath
	}

	// 2. 依次探测宿主机常见的已有标准配置路径
	candidates := []string{
		"/etc/sing-box/config.json",
		"/usr/local/etc/sing-box/config.json",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append([]string{filepath.Join(home, ".config", "sing-box", "config.json")}, candidates...)
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			ConfigPath = c
			return c
		}
	}

	// 3. 检查历史/默认路径是否存在
	defaultPanelPath := component.GetDefaultConfigPath(DriverName, "config.json")
	if fi, err := os.Stat(defaultPanelPath); err == nil && !fi.IsDir() {
		ConfigPath = defaultPanelPath
		return defaultPanelPath
	}

	// 4. 若全都不存在，根据运行身份首选标准路径
	var target string
	if component.IsUserMode() {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			target = filepath.Join(home, ".config", "sing-box", "config.json")
		} else {
			target = defaultPanelPath
		}
	} else {
		target = "/etc/sing-box/config.json"
	}

	ConfigPath = target
	return target
}

type Driver struct {
	systemd *component.SystemdManager
}

func NewDriver() *Driver {
	EnsureServiceUnit()
	return &Driver{
		systemd: component.NewSystemdManager(UnitName),
	}
}

// EnsureServiceUnit 自动检查并补齐 ExecReload，确保热重载毫秒级极速生效
func EnsureServiceUnit() {
	unitPath := filepath.Clean(component.GetSystemdUnitPath(UnitName))
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return
	}
	content := string(data)
	if !strings.Contains(content, "ExecReload=") && strings.Contains(content, "[Service]") {
		newContent := strings.Replace(content, "[Service]", "[Service]\nExecReload=/bin/kill -HUP $MAINPID", 1)
		/* #nosec G703, G306 - systemd unit file requires 0644 */
		_ = os.WriteFile(unitPath, []byte(newContent), 0644)
		_ = component.DaemonReload()
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
	var downloadURL, targetAssetName string
	pattern := fmt.Sprintf("linux-%s.tar.gz", arch)
	for _, asset := range rel.Assets {
		if strings.Contains(asset.Name, pattern) {
			downloadURL = asset.BrowserDownloadURL
			targetAssetName = asset.Name
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("asset for arch %s not found in release %s", arch, rel.TagName)
	}

	// 获取并比对 Release 官方 SHA-256 Checksums (防软件供应链投毒与镜像劫持)
	logFn("正在获取官方 Release 校验和清单...")
	checksums := component.FetchReleaseChecksums(rel)
	expectedHash := checksums[targetAssetName]
	if expectedHash != "" {
		logFn(fmt.Sprintf("已识别到官方 SHA-256 签名: %s...", expectedHash[:16]))
	}

	tmpArchive := filepath.Join(os.TempDir(), fmt.Sprintf("singbox-%s.tar.gz", rel.TagName))
	defer os.Remove(tmpArchive)

	logFn(fmt.Sprintf("开始安全下载资产: %s", downloadURL))
	if err := component.DownloadFileWithChecksum(downloadURL, tmpArchive, expectedHash); err != nil {
		return fmt.Errorf("download/verify failed: %w", err)
	}
	logFn("资产下载并完成供应链完整性校验，开始解压...")

	tmpBinary, err := extractBinary(tmpArchive, "sing-box")
	if err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	defer os.Remove(tmpBinary)

	targetBinary := filepath.Clean(component.GetDefaultBinaryPath(DriverName))
	targetConfig := filepath.Clean(GetConfigPath())

	_ = os.MkdirAll(filepath.Dir(targetBinary), 0750)

	logFn(fmt.Sprintf("安装二进制文件至 %s...", targetBinary))
	if err := os.Rename(tmpBinary, targetBinary); err != nil {
		/* #nosec G304 - reading temporary binary extracted from archive */
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

	_ = os.MkdirAll(filepath.Dir(targetConfig), 0750)

	logFn("配置并更新 Systemd 服务单元...")
	unitContent := fmt.Sprintf(`[Unit]
Description=sing-box service
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target network-online.target

[Service]
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
ExecStart=%s run -c %s
ExecReload=/bin/kill -HUP $MAINPID
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
		CategoryLabel:    "代理核心",
		ConfigRoute:      "/services?tab=singbox&sub=inbounds",
		Order:            1,
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

// DeriveRealityPublicKey 从 X25519 私钥推导 Base64URL 编码的客户端公钥
func DeriveRealityPublicKey(privateKey string) string {
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return ""
	}
	var raw []byte
	var err error
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if b, e := enc.DecodeString(privateKey); e == nil && len(b) == 32 {
			raw = b
			err = nil
			break
		} else if e != nil {
			err = e
		}
	}
	if err != nil || len(raw) != 32 {
		return ""
	}
	curve := ecdh.X25519()
	priv, err := curve.NewPrivateKey(raw)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
}

func extractBinary(archivePath, targetName string) (string, error) {
	/* #nosec G304 - reading validated archive from temporary download */
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
			/* #nosec G302, G304 - temporary binary executable extracted from archive */
			out, err := os.OpenFile(tmpTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
			if err != nil {
				return "", err
			}
			// 限制最大解压体积为 150MB，防御 Decompression Bomb / Tar Bomb 耗尽磁盘
			limitedReader := io.LimitReader(tr, 150*1024*1024)
			written, err := io.Copy(out, limitedReader)
			if err != nil {
				out.Close()
				_ = os.Remove(tmpTarget)
				return "", err
			}
			if written >= 150*1024*1024 {
				out.Close()
				_ = os.Remove(tmpTarget)
				return "", fmt.Errorf("解压异常中止: 单个二进制文件超出安全阈值 (150MB)，疑似 Tar Bomb 攻击")
			}
			out.Close()
			return tmpTarget, nil
		}
	}

	return "", fmt.Errorf("%s not found in archive", targetName)
}
