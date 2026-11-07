package subconverter

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"vps-auto-deploy/internal/component"
)

const (
	DriverName  = "subconverter"
	Owner       = "MetaCubeX"
	Repo        = "subconverter"
	UnitName    = "subconverter.service"
	DefaultPort = 25500
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
	return "Subconverter 订阅转换"
}

func (d *Driver) Description() string {
	return "强大的跨协议多客户端订阅转换引擎，支持 ACL4SSR / 远程规则集分流与模板定制。"
}

func (d *Driver) Category() string {
	return "converter"
}

// GetComponentDir 获取 subconverter 的独立资产与运行时目录
func GetComponentDir() string {
	return component.GetDefaultComponentDir(DriverName)
}

// GetBinaryPath 动态探测可执行文件绝对路径
func GetBinaryPath() string {
	compDir := GetComponentDir()
	return component.FindBinary(
		DriverName,
		filepath.Join(compDir, DriverName),
		component.GetDefaultBinaryPath(DriverName),
		"/usr/local/bin/subconverter",
		"/usr/bin/subconverter",
	)
}

// GetConfigPath 获取 pref.ini 配置文件路径
func GetConfigPath() string {
	return filepath.Join(GetComponentDir(), "pref.ini")
}

// GetInstalledVersion 探测本地安装的版本号
func (d *Driver) GetInstalledVersion() (string, error) {
	// 1. 若服务运行中，优先直接通过 HTTP /version 快速获取
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/version", DefaultPort))
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if v := component.ExtractVersion(string(body)); v != "" {
			return v, nil
		}
	}

	// 2. 检查安装落盘的版本标记文件
	versionFile := filepath.Join(GetComponentDir(), "version")
	if data, err := os.ReadFile(versionFile); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v, nil
		}
	}

	// 3. 检查本地二进制是否存在
	bin := GetBinaryPath()
	if bin == "" {
		return "", nil
	}

	// 4. 从可执行文件文本块中提取 SubConverter vX.Y.Z
	if binData, err := os.ReadFile(bin); err == nil {
		// 搜索头部前 4MB 字符
		maxLen := len(binData)
		if maxLen > 4*1024*1024 {
			maxLen = 4 * 1024 * 1024
		}
		sample := string(binData[:maxLen])
		if idx := strings.Index(sample, "SubConverter v"); idx != -1 {
			end := idx + 40
			if end > len(sample) {
				end = len(sample)
			}
			return component.ExtractVersion(sample[idx:end]), nil
		}
	}

	return "", nil
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

	logFn(fmt.Sprintf("正在查询 GitHub 仓库 %s/%s 发布信息...", Owner, Repo))
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
	var pattern string
	switch arch {
	case "amd64":
		pattern = "linux64.tar.gz"
	case "arm64":
		pattern = "aarch64.tar.gz"
	case "arm":
		pattern = "armv7.tar.gz"
	case "386":
		pattern = "linux32.tar.gz"
	default:
		return fmt.Errorf("unsupported arch for subconverter: %s", arch)
	}

	var downloadURL string
	for _, asset := range rel.Assets {
		if strings.Contains(asset.Name, pattern) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("asset for arch %s (pattern %s) not found in release %s", arch, pattern, rel.TagName)
	}

	tmpArchive := filepath.Join(os.TempDir(), fmt.Sprintf("subconverter-%s.tar.gz", rel.TagName))
	defer os.Remove(tmpArchive)

	logFn(fmt.Sprintf("开始下载 Subconverter 资产包: %s", downloadURL))
	if err := component.DownloadFile(downloadURL, tmpArchive); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	logFn("资产包下载完成，开始解压目录级资源与二进制...")

	compDir := GetComponentDir()
	if err := os.MkdirAll(compDir, 0755); err != nil {
		return fmt.Errorf("create component dir failed: %w", err)
	}

	// 完整解压 tar.gz 目录资产
	if err := extractTarGzToDir(tmpArchive, compDir); err != nil {
		return fmt.Errorf("extract assets error: %w", err)
	}

	binPath := filepath.Join(compDir, DriverName)
	if err := os.Chmod(binPath, 0755); err != nil {
		logFn(fmt.Sprintf("[WARN] 设置二进制执行权限失败: %v", err))
	}

	// 默认配置文件初始化 (pref.ini)
	prefPath := filepath.Join(compDir, "pref.ini")
	prefExample := filepath.Join(compDir, "pref.example.ini")
	if _, err := os.Stat(prefPath); os.IsNotExist(err) {
		if _, err := os.Stat(prefExample); err == nil {
			logFn("初始化默认配置文件 pref.ini...")
			if content, err := os.ReadFile(prefExample); err == nil {
				// 替换默认监听地址为 127.0.0.1，并开启缓存保障分流规则集高性能解析
				modified := strings.Replace(string(content), "listen=0.0.0.0", "listen=127.0.0.1", 1)
				modified = strings.Replace(modified, "enable_cache=false", "enable_cache=true", 1)
				_ = os.WriteFile(prefPath, []byte(modified), 0644)
			}
		} else {
			// 若无 example，创建最小安全默认配置
			defaultIni := fmt.Sprintf(`[common]
api_mode=false
api_access_token=
default_url=
enable_cache=true
cache_subscription=60
cache_config=300
cache_ruleset=21600
listen=127.0.0.1
port=%d
`, DefaultPort)
			_ = os.WriteFile(prefPath, []byte(defaultIni), 0644)
		}
	} else {
		logFn("保留现有用户自定义 pref.ini 配置文件。")
	}

	// 在用户全局 BIN 目录建立符号链接或拷贝，方便 CLI 探测与调用
	globalBin := component.GetDefaultBinaryPath(DriverName)
	_ = os.MkdirAll(filepath.Dir(globalBin), 0755)
	if globalBin != binPath {
		_ = os.Remove(globalBin)
		_ = os.Symlink(binPath, globalBin)
	}

	// 写入当前版本标记
	_ = os.WriteFile(filepath.Join(compDir, "version"), []byte(targetVersion), 0644)

	logFn("配置并更新 Systemd 服务单元...")
	unitContent := fmt.Sprintf(`[Unit]
Description=Subconverter - Advanced Subscription Converter
Documentation=https://github.com/MetaCubeX/subconverter
After=network.target network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`, compDir, binPath)

	if err := component.WriteServiceUnit(UnitName, unitContent); err != nil {
		logFn(fmt.Sprintf("[WARN] 写入服务单元失败: %v", err))
	}
	_ = d.systemd.Enable()

	logFn("正在启动 Subconverter 服务...")
	_ = d.systemd.Restart()

	component.SetCachedLatestVersion(DriverName, rel.TagName)
	logFn("Subconverter 安装/升级完成并已成功启动！")
	return nil
}

func (d *Driver) Uninstall() error {
	_ = d.systemd.Stop()
	_ = d.systemd.Disable()
	_ = component.RemoveServiceUnit(UnitName)

	globalBin := component.GetDefaultBinaryPath(DriverName)
	if globalBin != "" {
		_ = os.Remove(globalBin)
	}

	compDir := GetComponentDir()
	// 清理二进制与资源文件，但默认保留 pref.ini 防止意外丢失用户规则配置
	_ = os.Remove(filepath.Join(compDir, DriverName))
	_ = os.Remove(filepath.Join(compDir, "version"))
	_ = os.RemoveAll(filepath.Join(compDir, "rules"))
	_ = os.RemoveAll(filepath.Join(compDir, "snippets"))
	_ = os.RemoveAll(filepath.Join(compDir, "base"))
	_ = os.RemoveAll(filepath.Join(compDir, "templates"))
	_ = os.RemoveAll(filepath.Join(compDir, "config"))

	return nil
}

func (d *Driver) Status() (component.ServiceStatus, error) {
	installedVer, _ := d.GetInstalledVersion()
	installed := installedVer != "" || GetBinaryPath() != ""
	active := false
	runningStatus := "not_installed"

	if installed {
		active = d.systemd.IsActive() || component.IsProcessRunning(DriverName)
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
		CategoryLabel:    "订阅与规则",
		ConfigRoute:      "/services?tab=subscriptions",
		Order:            3,
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

// extractTarGzToDir 将 tar.gz 解包并剥离最外层的 subconverter/ 根目录
func extractTarGzToDir(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(header.Name)
		// 剥离顶层 subconverter/ 前缀
		parts := strings.Split(cleanName, string(filepath.Separator))
		if len(parts) > 0 && parts[0] == DriverName {
			parts = parts[1:]
		}
		if len(parts) == 0 {
			continue
		}
		relPath := filepath.Join(parts...)
		targetPath := filepath.Join(destDir, relPath)

		// 安全检查防 Zip Slip
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(filepath.Separator)) && targetPath != destDir {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}
