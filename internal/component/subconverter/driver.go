package subconverter

import (
	"archive/tar"
	"bufio"
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
	"vps-auto-deploy/internal/model"
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
	// 1. 检查本地二进制是否存在
	bin := GetBinaryPath()
	if bin == "" {
		return "", nil
	}

	// 2. 优先检查安装落盘的权威 Release 版本标记文件
	// (解决 MetaCubeX upstream 打 release tag v0.9.2 时源码内漏改硬编码 VERSION "v0.9.1-mihomo" 的问题)
	versionFile := filepath.Join(GetComponentDir(), "version")
	/* #nosec G304 - internal version marker file */
	if data, err := os.ReadFile(versionFile); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v, nil
		}
	}

	// 3. 若无版本标记文件（如外部第三方预装），尝试通过 HTTP /version 探测
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/version", DefaultPort))
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if v := component.ExtractVersion(string(body)); v != "" {
			return v, nil
		}
	}

	// 4. 从可执行文件文本块中提取 SubConverter vX.Y.Z
	/* #nosec G304 - reading validated component binary */
	if binData, err := os.ReadFile(bin); err == nil {
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

	var downloadURL, targetAssetName string
	for _, asset := range rel.Assets {
		if strings.Contains(asset.Name, pattern) {
			downloadURL = asset.BrowserDownloadURL
			targetAssetName = asset.Name
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("asset for arch %s (pattern %s) not found in release %s", arch, pattern, rel.TagName)
	}

	// 抓取并解析 Release 官方 Checksums (防软件供应链投毒)
	logFn("正在获取官方 Release 校验和清单...")
	checksums := component.FetchReleaseChecksums(rel)
	expectedHash := checksums[targetAssetName]
	if expectedHash != "" {
		logFn(fmt.Sprintf("已识别到官方 SHA-256 签名: %s...", expectedHash[:16]))
	}

	tmpArchive := filepath.Join(os.TempDir(), fmt.Sprintf("subconverter-%s.tar.gz", rel.TagName))
	defer os.Remove(tmpArchive)

	logFn(fmt.Sprintf("开始安全下载资产: %s", downloadURL))
	if err := component.DownloadFileWithChecksum(downloadURL, tmpArchive, expectedHash); err != nil {
		return fmt.Errorf("download/verify failed: %w", err)
	}
	logFn("资产包下载并完成供应链完整性校验，正在停止现有服务以释放文件锁并解压资产...")
	_ = d.systemd.Stop()

	compDir := GetComponentDir()
	if err := os.MkdirAll(compDir, 0750); err != nil {
		return fmt.Errorf("create component dir failed: %w", err)
	}

	// 完整解压 tar.gz 目录资产
	if err := extractTarGzToDir(tmpArchive, compDir); err != nil {
		return fmt.Errorf("extract assets error: %w", err)
	}

	binPath := filepath.Join(compDir, DriverName)
	/* #nosec G302 - binary executable requires execute permission */
	if err := os.Chmod(binPath, 0755); err != nil {
		logFn(fmt.Sprintf("[WARN] 设置二进制执行权限失败: %v", err))
	}

	// 默认配置文件初始化 (pref.ini)
	prefPath := filepath.Clean(filepath.Join(compDir, "pref.ini"))
	prefExample := filepath.Clean(filepath.Join(compDir, "pref.example.ini"))
	if _, err := os.Stat(prefPath); os.IsNotExist(err) {
		if _, err := os.Stat(prefExample); err == nil {
			logFn("初始化默认配置文件 pref.ini...")
			if content, err := os.ReadFile(prefExample); err == nil {
				// 替换默认监听地址为 127.0.0.1，并开启缓存与容灾跳过错误链接 (skip_failed_links)
				modified := strings.Replace(string(content), "listen=0.0.0.0", "listen=127.0.0.1", 1)
				modified = strings.Replace(modified, "enable_cache=false", "enable_cache=true", 1)
				modified = strings.Replace(modified, "skip_failed_links=false", "skip_failed_links=true", -1)
				if !strings.Contains(modified, "skip_failed_links=true") {
					if strings.Contains(modified, "[common]") {
						modified = strings.Replace(modified, "[common]", "[common]\nskip_failed_links=true", 1)
					}
				}
				/* #nosec G703 */
				_ = os.WriteFile(prefPath, []byte(modified), 0600)
				_ = os.Chmod(prefPath, 0600)
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
skip_failed_links=true
listen=127.0.0.1
port=%d

[advanced]
skip_failed_links=true
`, DefaultPort)
			_ = os.WriteFile(prefPath, []byte(defaultIni), 0600)
			_ = os.Chmod(prefPath, 0600)
		}
	} else {
		logFn("保留现有用户自定义 pref.ini 配置文件，并同步高可用防护配置。")
		EnsurePrefDefaults()
	}

	// 在用户全局 BIN 目录建立符号链接或拷贝，方便 CLI 探测与调用
	globalBin := component.GetDefaultBinaryPath(DriverName)
	_ = os.MkdirAll(filepath.Dir(globalBin), 0750)
	if globalBin != binPath {
		_ = os.Remove(globalBin)
		_ = os.Symlink(binPath, globalBin)
	}

	// 写入当前版本标记
	_ = os.WriteFile(filepath.Join(compDir, "version"), []byte(targetVersion), 0600)

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
	EnsurePrefDefaults()
	return d.systemd.Start()
}

func (d *Driver) Stop() error {
	return d.systemd.Stop()
}

func (d *Driver) Restart() error {
	EnsurePrefDefaults()
	return d.systemd.Restart()
}

// EnsurePrefDefaults 确保 pref.ini 存在，合并数据库持久化配置，并注入推荐的高可用容灾选项
func EnsurePrefDefaults() {
	prefPath := filepath.Clean(GetConfigPath())
	data, err := os.ReadFile(prefPath)
	if err != nil {
		// 若文件不存在，尝试从 SQLite 数据库还原用户备份配置
		if model.DB != nil {
			if rawIni, _ := model.GetSetting("subconverter_raw_ini"); rawIni != "" {
				_ = os.MkdirAll(filepath.Dir(prefPath), 0750)
				_ = os.WriteFile(prefPath, []byte(rawIni), 0600)
				_ = os.Chmod(prefPath, 0600)
				data = []byte(rawIni)
			}
		}
		if len(data) == 0 {
			return
		}
	}
	content := string(data)
	modified := false

	// 1. 确保开启 skip_failed_links 容灾（避免单节点异常导致整组成员订阅崩溃）
	if strings.Contains(content, "skip_failed_links=false") {
		content = strings.Replace(content, "skip_failed_links=false", "skip_failed_links=true", -1)
		modified = true
	} else if !strings.Contains(content, "skip_failed_links") {
		if strings.Contains(content, "[common]") {
			content = strings.Replace(content, "[common]", "[common]\nskip_failed_links=true", 1)
		} else {
			content += "\n[common]\nskip_failed_links=true\n"
		}
		modified = true
	}

	// 2. 从数据库恢复同步用户保存的关键业务配置 (exclude_remarks, include_remarks, default_external_config)
	if model.DB != nil {
		if dbExclude, _ := model.GetSetting("subconverter_exclude_remarks"); dbExclude != "" {
			if curVal := GetSettingFromContent(content, "exclude_remarks"); curVal != dbExclude {
				content = UpdateIniKey(content, "exclude_remarks", dbExclude)
				modified = true
			}
		}
		if dbInclude, _ := model.GetSetting("subconverter_include_remarks"); dbInclude != "" {
			if curVal := GetSettingFromContent(content, "include_remarks"); curVal != dbInclude {
				content = UpdateIniKey(content, "include_remarks", dbInclude)
				modified = true
			}
		}
		if dbConfig, _ := model.GetSetting("subconverter_default_external_config"); dbConfig != "" {
			if curVal := GetSettingFromContent(content, "default_external_config"); curVal != dbConfig {
				content = UpdateIniKey(content, "default_external_config", dbConfig)
				modified = true
			}
		}
	}

	if modified {
		/* #nosec G703 */
		_ = os.WriteFile(prefPath, []byte(content), 0600)
		_ = os.Chmod(prefPath, 0600)
	}

	// 3. 检查并从数据库还原 custom_rules.ini (若磁盘丢失)
	if model.DB != nil {
		customRulesPath := filepath.Join(GetComponentDir(), "config", "custom_rules.ini")
		if _, err := os.Stat(customRulesPath); os.IsNotExist(err) {
			if dbRules, _ := model.GetSetting("subconverter_custom_rules"); dbRules != "" {
				_ = os.MkdirAll(filepath.Dir(customRulesPath), 0750)
				_ = os.WriteFile(customRulesPath, []byte(dbRules), 0600)
			}
		}
	}
}

// GetSettingFromPref 从 pref.ini 中读取指定 key 的配置值
func GetSettingFromPref(key string) string {
	prefPath := GetConfigPath()
	/* #nosec G304 - reading internal pref.ini */
	data, err := os.ReadFile(prefPath)
	if err != nil {
		return ""
	}
	return GetSettingFromContent(string(data), key)
}

// GetSettingFromContent 从 INI 内容中解析 key 对应的 value
func GetSettingFromContent(content, key string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

// UpdateIniKey 更新或插入 INI 内容中的 key=value
func UpdateIniKey(content, key, val string) string {
	var lines []string
	found := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, ";") && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "[") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
				lines = append(lines, fmt.Sprintf("%s=%s", key, val))
				found = true
				continue
			}
		}
		lines = append(lines, line)
	}
	if !found {
		// 插入到 [common] 之后或末尾
		inserted := false
		var out []string
		for _, l := range lines {
			out = append(out, l)
			if strings.TrimSpace(l) == "[common]" {
				out = append(out, fmt.Sprintf("%s=%s", key, val))
				inserted = true
			}
		}
		if !inserted {
			out = append(out, fmt.Sprintf("%s=%s", key, val))
		}
		lines = out
	}
	return strings.Join(lines, "\n")
}

// extractTarGzToDir 将 tar.gz 解包并剥离最外层的 subconverter/ 根目录
func extractTarGzToDir(archivePath, destDir string) error {
	/* #nosec G304 - archivePath verified in temporary download directory */
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
	var totalExtracted int64
	const maxArchiveTotal = 250 * 1024 * 1024 // 整个压缩包累计解压上限 250MB
	const maxPerFile = 100 * 1024 * 1024      // 单个文件解压上限 100MB

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
		case tar.TypeSymlink, tar.TypeLink:
			// 严密拦截符号链接与硬链接，防范软链接穿越攻击 (Symlink/Hardlink Traversal)
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
				return err
			}

			// 保护用户自定义配置文件：若目标 pref.ini 或 custom_rules.ini 已存在，严禁被 release 压缩包覆盖！
			baseName := filepath.Base(targetPath)
			if baseName == "pref.ini" || baseName == "custom_rules.ini" {
				if _, err := os.Stat(targetPath); err == nil {
					targetPath = targetPath + ".example"
				}
			}

			tmpPath := targetPath + ".tmp"
			// 安全清洗权限位，严防 SUID/SGID 特殊权限位继承
			safeMode := (header.FileInfo().Mode() & 0755) &^ (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
			safeMode |= 0600
			/* #nosec G304 - temporary destination file verified by prefix check */
			outFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, safeMode)
			if err != nil {
				return err
			}

			// 单文件解压限制 + 累计体积限制，防御 Tar Bomb DoS
			limited := io.LimitReader(tarReader, maxPerFile)
			written, err := io.Copy(outFile, limited)
			if err != nil {
				outFile.Close()
				_ = os.Remove(tmpPath)
				return err
			}
			if written >= maxPerFile {
				outFile.Close()
				_ = os.Remove(tmpPath)
				return fmt.Errorf("解包异常: 单文件超出安全限制 (100MB)")
			}
			totalExtracted += written
			if totalExtracted > maxArchiveTotal {
				outFile.Close()
				_ = os.Remove(tmpPath)
				return fmt.Errorf("解包异常: 累计解压体积超出安全阈值 (250MB)，疑似 Tar Bomb 攻击")
			}
			outFile.Close()

			// 原子重命名替换，防止 Linux 报错 "text file busy"
			if err := os.Rename(tmpPath, targetPath); err != nil {
				_ = os.Remove(targetPath)
				if err := os.Rename(tmpPath, targetPath); err != nil {
					_ = os.Remove(tmpPath)
					return err
				}
			}
		}
	}
	return nil
}
