package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"vps-auto-deploy/internal/version"
)

// PackageInfo 描述导入的升级安装包元数据
type PackageInfo struct {
	PackageID        string `json:"package_id"`
	FileName         string `json:"file_name"`
	Size             int64  `json:"size"`
	Arch             string `json:"arch"`
	SHA256           string `json:"sha256"`
	Version          string `json:"version"`
	CurrentArch      string `json:"current_arch"`
	CurrentVersion   string `json:"current_version"`
	MatchCurrentArch bool   `json:"match_current_arch"`
	CreatedAt        string `json:"created_at"`
}

// GetPackageDir 返回安装包存储目录 (Root 环境 /etc/vps-auto-deploy/packages，普通用户环境 ~/.config/vps-auto-deploy/packages)
func GetPackageDir() string {
	if os.Geteuid() == 0 {
		dir := "/etc/vps-auto-deploy/packages"
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dir := filepath.Join(home, ".config", "vps-auto-deploy", "packages")
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	dir := "./packages"
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// ComputeFileSHA256 计算文件的 SHA-256 十六进制哈希串
func ComputeFileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ParseELFArch 解析 Linux ELF 二进制的目标架构
func ParseELFArch(filePath string) (string, error) {
	file, err := elf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("非有效 Linux ELF 可执行程序: %w", err)
	}
	defer file.Close()

	switch file.Machine {
	case elf.EM_X86_64:
		return "amd64", nil
	case elf.EM_AARCH64:
		return "arm64", nil
	case elf.EM_386:
		return "386", nil
	case elf.EM_ARM:
		return "arm", nil
	case elf.EM_RISCV:
		return "riscv64", nil
	case elf.EM_MIPS:
		return "mips", nil
	default:
		return strings.ToLower(file.Machine.String()), nil
	}
}

// ExtractVersionFromBinary 静态分析二进制提取版本号（纯静态读取，绝不被动执行未受信任的可执行文件）
func ExtractVersionFromBinary(filePath string) string {
	// 1. 首选 Go 原生 buildinfo 静态读取
	if bi, err := buildinfo.ReadFile(filePath); err == nil {
		if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "git-" + s.Value[:7]
			}
		}
	}

	// 2. 备选方案：从文件前 4MB 静态扫描语义化版本正则
	f, err := os.Open(filePath)
	if err == nil {
		defer f.Close()
		buf := make([]byte, 4*1024*1024)
		n, _ := io.ReadFull(f, buf)
		if n > 0 {
			re := regexp.MustCompile(`v[0-9]+\.[0-9]+(\.[0-9]+)?(-[a-zA-Z0-9.]+)?`)
			if m := re.FindString(string(buf[:n])); m != "" {
				return m
			}
		}
	}

	return "未知版本"
}

// InspectAndSavePackage 检查并保存导入的安装包 (支持 ELF 二进制、tar.gz 与 zip)
func InspectAndSavePackage(srcPath, origFileName string) (*PackageInfo, error) {
	tempExtractDir, err := os.MkdirTemp("", "vps-pkg-inspect-*")
	if err != nil {
		return nil, fmt.Errorf("创建解包临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempExtractDir)

	candidateBinary := filepath.Join(tempExtractDir, "vps-panel")

	// 1. 判断是否为压缩包格式
	lowerName := strings.ToLower(origFileName)
	if strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz") {
		if err := extractTarGz(srcPath, tempExtractDir); err != nil {
			return nil, fmt.Errorf("解压 tar.gz 失败: %w", err)
		}
	} else if strings.HasSuffix(lowerName, ".zip") {
		if err := extractZip(srcPath, tempExtractDir); err != nil {
			return nil, fmt.Errorf("解压 zip 失败: %w", err)
		}
	} else {
		// 直接作为可执行二进制复制
		if err := copyFile(srcPath, candidateBinary); err != nil {
			return nil, fmt.Errorf("缓存二进制失败: %w", err)
		}
	}

	// 2. 定位解包后的可执行程序
	finalBinary := candidateBinary
	if fi, err := os.Stat(finalBinary); err != nil || fi.IsDir() {
		// 递归查找目录下的可执行文件
		found := ""
		_ = filepath.Walk(tempExtractDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr == nil && !info.IsDir() && strings.Contains(info.Name(), "vps-panel") {
				found = path
				return io.EOF
			}
			return nil
		})
		if found != "" {
			finalBinary = found
		} else {
			return nil, fmt.Errorf("安装包内未找到 vps-panel 可执行程序")
		}
	}

	// 3. 解析 ELF 架构
	arch, err := ParseELFArch(finalBinary)
	if err != nil {
		return nil, err
	}

	// 4. 计算 SHA-256
	hash, err := ComputeFileSHA256(finalBinary)
	if err != nil {
		return nil, fmt.Errorf("计算哈希失败: %w", err)
	}

	// 5. 提取版本号
	ver := "未知版本"
	if arch == runtime.GOARCH {
		ver = ExtractVersionFromBinary(finalBinary)
	}
	if ver == "未知版本" {
		// 从文件名中尝试正则提取版本
		re := regexp.MustCompile(`v[0-9]+\.[0-9]+(\.[0-9]+)?(-[a-zA-Z0-9.]+)?`)
		if m := re.FindString(origFileName); m != "" {
			ver = m
		}
	}

	fi, err := os.Stat(finalBinary)
	if err != nil {
		return nil, err
	}

	shortHash := hash
	if len(shortHash) > 8 {
		shortHash = shortHash[:8]
	}
	pkgID := fmt.Sprintf("vps-panel-%s-%s", arch, shortHash)

	pkgDir := GetPackageDir()
	destBinary := filepath.Join(pkgDir, pkgID+".bin")
	destMeta := filepath.Join(pkgDir, pkgID+".json")

	if err := copyFile(finalBinary, destBinary); err != nil {
		return nil, fmt.Errorf("保存安装包至仓库失败: %w", err)
	}
	_ = os.Chmod(destBinary, 0755)

	info := &PackageInfo{
		PackageID:        pkgID,
		FileName:         origFileName,
		Size:             fi.Size(),
		Arch:             arch,
		SHA256:           hash,
		Version:          ver,
		CurrentArch:      runtime.GOARCH,
		CurrentVersion:   version.Version,
		MatchCurrentArch: arch == runtime.GOARCH,
		CreatedAt:        time.Now().Format("2006-01-02 15:04:05"),
	}

	metaBytes, _ := json.MarshalIndent(info, "", "  ")
	_ = os.WriteFile(destMeta, metaBytes, 0644)

	return info, nil
}

// ListPackages 列出仓库中所有已缓存的安装包
func ListPackages() ([]PackageInfo, error) {
	pkgDir := GetPackageDir()
	files, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, err
	}

	var list []PackageInfo
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
			metaPath := filepath.Join(pkgDir, f.Name())
			data, err := os.ReadFile(metaPath)
			if err == nil {
				var item PackageInfo
				if err := json.Unmarshal(data, &item); err == nil {
					// 确认对应 .bin 存在
					binPath := filepath.Join(pkgDir, item.PackageID+".bin")
					if _, statErr := os.Stat(binPath); statErr == nil {
						item.CurrentArch = runtime.GOARCH
						item.CurrentVersion = version.Version
						item.MatchCurrentArch = item.Arch == runtime.GOARCH
						list = append(list, item)
					}
				}
			}
		}
	}
	return list, nil
}

// GetPackageBinaryPath 获取指定 packageID 的二进制完整路径 (严格校验路径合法性，防范目录穿越攻击)
func GetPackageBinaryPath(packageID string) (string, error) {
	cleanID := strings.TrimSpace(packageID)
	if cleanID == "" || strings.Contains(cleanID, "..") || strings.Contains(cleanID, "/") || strings.Contains(cleanID, "\\") {
		return "", errors.New("非法的 package_id: 禁止包含路径穿越字符或目录分隔符")
	}
	validPkgIDRegex := regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	if !validPkgIDRegex.MatchString(cleanID) {
		return "", errors.New("非法的 package_id: 仅支持字母、数字、短横线与下划线")
	}

	pkgDir := GetPackageDir()
	binPath := filepath.Join(pkgDir, cleanID+".bin")
	if !isPathSafe(pkgDir, binPath) {
		return "", errors.New("【安全拦截】安装包路径越权")
	}

	if fi, err := os.Stat(binPath); err == nil && !fi.IsDir() {
		return binPath, nil
	}
	return "", fmt.Errorf("安装包 %s 未找到", cleanID)
}

// FindBestPackageForArch 为指定架构寻找最匹配的二进制文件路径
// 优先在 packages 目录查找，若与本机架构相同且无包，降级使用当前正在运行的自身二进制
func FindBestPackageForArch(targetArch string) (string, string, error) {
	packages, err := ListPackages()
	if err == nil {
		for _, p := range packages {
			if p.Arch == targetArch {
				binPath := filepath.Join(GetPackageDir(), p.PackageID+".bin")
				return binPath, p.SHA256, nil
			}
		}
	}

	// 若目标架构与本机一致，使用当前可执行文件
	if targetArch == runtime.GOARCH {
		exePath, err := os.Executable()
		if err == nil {
			if fi, err := os.Stat(exePath); err == nil && fi.Size() > 1000000 {
				h, _ := ComputeFileSHA256(exePath)
				return exePath, h, nil
			}
		}
		if fi, err := os.Stat("/usr/local/bin/vps-panel"); err == nil && fi.Size() > 1000000 {
			h, _ := ComputeFileSHA256("/usr/local/bin/vps-panel")
			return "/usr/local/bin/vps-panel", h, nil
		}
	}

	return "", "", fmt.Errorf("未找到适用于 %s 架构的安装包，请先在面板导入对应架构的 vps-panel 安装包", targetArch)
}

// ApplyLocalUpgrade 将指定二进制原子替换至本机服务程序并验证
func ApplyLocalUpgrade(srcBinaryPath string) error {
	// 1. 验证目标二进制架构是否与本机匹配
	arch, err := ParseELFArch(srcBinaryPath)
	if err != nil {
		return fmt.Errorf("安装包校验失败: %w", err)
	}
	if arch != runtime.GOARCH {
		return fmt.Errorf("架构不兼容！升级包架构为 %s，本机系统架构为 %s", arch, runtime.GOARCH)
	}

	// 2. 获取本机运行二进制路径
	destPath := "/usr/local/bin/vps-panel"
	exePath, err := os.Executable()
	if err == nil && !strings.Contains(exePath, "/tmp/") && !strings.Contains(exePath, "\\Temp\\") {
		destPath = exePath
	}

	// 3. 备份现有程序
	bakPath := destPath + ".bak"
	_ = copyFile(destPath, bakPath)

	// 4. 原子覆盖
	tmpDest := destPath + ".upgrade.tmp"
	if err := copyFile(srcBinaryPath, tmpDest); err != nil {
		return fmt.Errorf("写入临时程序失败: %w", err)
	}
	if err := os.Chmod(tmpDest, 0755); err != nil {
		_ = os.Remove(tmpDest)
		return fmt.Errorf("赋予执行权限失败: %w", err)
	}

	if err := os.Rename(tmpDest, destPath); err != nil {
		_ = os.Remove(tmpDest)
		return fmt.Errorf("原子替换二进制失败 (mv): %w", err)
	}

	return nil
}

// RestartServiceAsync 异步分离延迟重启当前服务守护进程
func RestartServiceAsync(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		// 1. 优先使用 systemctl 重载并重启
		cmd := exec.Command("systemctl", "restart", "vps-panel")
		if err := cmd.Run(); err != nil {
			// 2. 备用 service
			_ = exec.Command("service", "vps-panel", "restart").Run()
		}
	}()
}

// 工具函数：文件复制
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

const (
	maxExtractSingleFileSize = 150 * 1024 * 1024 // 单个文件最多 150MB
	maxExtractTotalSize      = 300 * 1024 * 1024 // 整个压缩包累计解压最多 300MB
)

// 解压 tar.gz
func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	var totalExtracted int64
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, header.Name)
		if !isPathSafe(destDir, target) {
			continue // 防 Zip Slip 路径穿透
		}

		switch header.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			// 安全清洗权限位，严防 SUID/SGID 特殊权限逃逸与越权
			perm := uint32(header.Mode & 0777)
			safeMode := (os.FileMode(perm) & 0755) &^ (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
			safeMode |= 0600
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, safeMode)
			if err != nil {
				return err
			}

			// 防 Zip Bomb：限制单文件写入上限与总解压上限
			remainTotal := int64(maxExtractTotalSize) - totalExtracted
			if remainTotal <= 0 {
				outFile.Close()
				return fmt.Errorf("解压数据量超出总上限 (300MB)，已拦截防解压炸弹")
			}
			maxCopy := int64(maxExtractSingleFileSize)
			if remainTotal < maxCopy {
				maxCopy = remainTotal
			}

			written, err := io.Copy(outFile, io.LimitReader(tr, maxCopy+1))
			outFile.Close()
			if err != nil {
				return err
			}
			if written > maxExtractSingleFileSize {
				return fmt.Errorf("解压单个文件 %s 超过大小限制 (150MB)，已拦截防解压炸弹", header.Name)
			}
			totalExtracted += written
			if totalExtracted > maxExtractTotalSize {
				return fmt.Errorf("解压数据总量超过上限 (300MB)，已拦截防解压炸弹")
			}
		}
	}
	return nil
}

// 解压 zip
func extractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	var totalExtracted int64
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if !isPathSafe(destDir, target) {
			continue // 防 Zip Slip 路径穿透
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0755)
			continue
		}
		// 严密拦截符号链接，防范软链接穿越攻击 (Symlink Traversal)
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(target), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		// 安全清洗权限位，剥离 SUID/SGID 特殊位
		safeMode := (f.Mode() & 0755) &^ (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
		safeMode |= 0600
		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, safeMode)
		if err != nil {
			rc.Close()
			return err
		}

		// 防 Zip Bomb：限制单文件写入上限与总解压上限
		remainTotal := int64(maxExtractTotalSize) - totalExtracted
		if remainTotal <= 0 {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("解压数据量超出总上限 (300MB)，已拦截防解压炸弹")
		}
		maxCopy := int64(maxExtractSingleFileSize)
		if remainTotal < maxCopy {
			maxCopy = remainTotal
		}

		written, err := io.Copy(outFile, io.LimitReader(rc, maxCopy+1))
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
		if written > maxExtractSingleFileSize {
			return fmt.Errorf("解压单个文件 %s 超过大小限制 (150MB)，已拦截防解压炸弹", f.Name)
		}
		totalExtracted += written
		if totalExtracted > maxExtractTotalSize {
			return fmt.Errorf("解压数据总量超过上限 (300MB)，已拦截防解压炸弹")
		}
	}
	return nil
}

// isPathSafe 严密检查目标路径是否完全限定在目标目录之内（防御 Zip Slip / Tar Slip 穿越）
func isPathSafe(destDir, targetPath string) bool {
	cleanDest := filepath.Clean(destDir)
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(cleanDest, cleanTarget)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}


