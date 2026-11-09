package handler

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/executil"
	"vps-auto-deploy/internal/pkg/netutil"
	"vps-auto-deploy/internal/pkg/upgrade"
	"vps-auto-deploy/internal/version"
)

type SystemOverview struct {
	PublicIP   string                    `json:"public_ip"`
	OS         string                    `json:"os"`
	Arch       string                    `json:"arch"`
	NumCPU     int                       `json:"num_cpu"`
	BBR        bool                      `json:"bbr"`
	Components []component.ServiceStatus `json:"components"`
	SecretPath string                    `json:"secret_path"`
	Version    string                    `json:"version"`
	SHA256     string                    `json:"sha256"`
}

var (
	cachedSelfSHA256     string
	cachedSelfSHA256Time time.Time
)

func getSelfSHA256() string {
	if cachedSelfSHA256 != "" && time.Since(cachedSelfSHA256Time) < 5*time.Minute {
		return cachedSelfSHA256
	}
	exe, err := os.Executable()
	if err == nil {
		h, err := upgrade.ComputeFileSHA256(exe)
		if err == nil {
			cachedSelfSHA256 = h
			cachedSelfSHA256Time = time.Now()
			return h
		}
	}
	h, err := upgrade.ComputeFileSHA256("/usr/local/bin/vps-panel")
	if err == nil {
		cachedSelfSHA256 = h
		cachedSelfSHA256Time = time.Now()
		return h
	}
	return ""
}

func GetSystemOverview(c *gin.Context) {
	pubIP, _ := model.GetSetting("public_ip")
	if pubIP == "" {
		pubIP = netutil.GetPublicIP()
	}
	if pubIP == "" || pubIP == "127.0.0.1" {
		host, _, err := net.SplitHostPort(c.Request.Host)
		if err == nil && host != "127.0.0.1" && host != "localhost" && host != "" {
			if parsed := net.ParseIP(host); parsed != nil && parsed.To4() != nil {
				pubIP = host
			}
		}
	}

	secretPath, _ := model.GetSetting("secret_path")

	// 检查 BBR 状态
	bbrEnabled := false
	out, err := executil.RunCommand(2*time.Second, "sysctl", "net.ipv4.tcp_congestion_control")
	if err == nil && strings.Contains(out, "bbr") {
		bbrEnabled = true
	}

	comps, _ := component.ListAll(false)

	c.JSON(http.StatusOK, SystemOverview{
		PublicIP:   pubIP,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		NumCPU:     runtime.NumCPU(),
		BBR:        bbrEnabled,
		Components: comps,
		SecretPath: secretPath,
		Version:    version.Version,
		SHA256:     getSelfSHA256(),
	})
}

// EnableBBR 一键开启 BBR 拥塞控制算法
func EnableBBR(c *gin.Context) {
	bbrConf := "\nnet.core.default_qdisc=fq\nnet.ipv4.tcp_congestion_control=bbr\n"
	f, err := os.OpenFile("/etc/sysctl.conf", os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(bbrConf)
		f.Close()
		_, _ = executil.RunCommand(3*time.Second, "sysctl", "-p")
		c.JSON(http.StatusOK, gin.H{"message": "BBR enabled successfully"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write sysctl: " + err.Error()})
}

// StreamLogs SSE 实时推送 journalctl 实时日志
func StreamLogs(c *gin.Context) {
	unit := c.Query("unit")
	if unit == "" {
		unit = "sing-box"
	}
	if !strings.HasSuffix(unit, ".service") {
		unit += ".service"
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	// 1. 检查服务的 systemd 注册状态与健康度
	sysManager := component.NewSystemdManager(unit)
	info := sysManager.GetUnitInfo()

	// 立即发送握手连接成功消息，避免用户界面卡在“正在连接”
	c.SSEvent("message", fmt.Sprintf("[系统提示] 已成功建立 %s 实时日志流连接 (SSE / journalctl)", unit))

	if info.LoadState == "not-found" {
		msg := fmt.Sprintf("[系统提示] 服务单元 %s 尚未在 Systemd 中注册。", unit)
		if strings.HasPrefix(unit, "cloudflared") {
			msg += " Cloudflared 客户端已安装，但尚未配置穿透隧道。请前往「配置中心 ➔ Cloudflared 隧道」创建并启动隧道后，系统才会自动生成该服务并输出运行日志。"
		} else {
			msg += " 请先在软件中心完成安装或前往业务配置中心初始化该服务。"
		}
		c.SSEvent("message", msg)
	} else if info.ActiveState == "inactive" || info.ActiveState == "stopped" {
		c.SSEvent("message", fmt.Sprintf("[系统提示] 服务 %s 当前处于「未激活 (已停止)」状态。正在监听历史日志与启动输出...", unit))
	} else if info.ActiveState == "failed" || info.SubState == "auto-restart" {
		c.SSEvent("message", fmt.Sprintf("[系统提示] 服务 %s 当前处于「异常 (failed)」状态，可能存在启动崩溃，请查看下方历史错误输出排查。", unit))
	}
	c.Writer.Flush()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	logChan, err := executil.StreamJournalctlLogs(ctx, unit, 50)
	if err != nil {
		c.SSEvent("error", err.Error())
		return
	}

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case line, ok := <-logChan:
			if !ok {
				return false
			}
			c.SSEvent("message", line)
			return true
		case <-time.After(30 * time.Second):
			// 保活心跳 ping
			c.SSEvent("ping", fmt.Sprintf("%d", time.Now().Unix()))
			return true
		}
	})
}

// UploadUpgradePackage 上传并校验新版安装包 (二进制或 tar.gz/zip)
func UploadUpgradePackage(c *gin.Context) {
	file, header, err := c.Request.FormFile("package")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的安装包文件: " + err.Error()})
		return
	}
	defer file.Close()

	tempFile, err := os.CreateTemp("", "vps-pkg-upload-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时上传文件失败: " + err.Error()})
		return
	}
	defer os.Remove(tempFile.Name())

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入上传文件失败: " + err.Error()})
		return
	}
	tempFile.Close()

	info, err := upgrade.InspectAndSavePackage(tempFile.Name(), header.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "安装包校验失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// ListUpgradePackages 列出已导入的所有安装包
func ListUpgradePackages(c *gin.Context) {
	list, err := upgrade.ListPackages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取安装包列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// ApplyLocalUpgrade 将指定的已导入安装包应用至本机
func ApplyLocalUpgrade(c *gin.Context) {
	var req struct {
		PackageID string `json:"package_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PackageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的 package_id"})
		return
	}

	binPath, err := upgrade.GetPackageBinaryPath(req.PackageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if err := upgrade.ApplyLocalUpgrade(binPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "应用升级失败: " + err.Error()})
		return
	}

	upgrade.RestartServiceAsync(800 * time.Millisecond)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "本地节点升级包已部署生效，服务正在平滑重启...",
	})
}

// SelfUpgradeSystem 供远程节点调用：接收二进制数据流并执行原子替换与自重启
func SelfUpgradeSystem(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 100*1024*1024)

	tempFile, err := os.CreateTemp("", "vps-self-upgrade-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建自升级临时文件失败: " + err.Error()})
		return
	}
	defer os.Remove(tempFile.Name())

	var srcReader io.Reader = c.Request.Body

	// 若使用了 multipart/form-data 传输
	if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		f, _, formErr := c.Request.FormFile("binary")
		if formErr == nil {
			defer f.Close()
			srcReader = f
		}
	}

	if _, err := io.Copy(tempFile, srcReader); err != nil {
		tempFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "接收升级数据流失败: " + err.Error()})
		return
	}
	tempFile.Close()

	// 校验 ELF 架构
	arch, err := upgrade.ParseELFArch(tempFile.Name())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的可执行文件: " + err.Error()})
		return
	}
	if arch != runtime.GOARCH {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("架构不兼容！升级包架构为 %s，本节点运行架构为 %s", arch, runtime.GOARCH)})
		return
	}

	// 执行原子替换
	if err := upgrade.ApplyLocalUpgrade(tempFile.Name()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "执行升级替换失败: " + err.Error()})
		return
	}

	// 异步延迟 800ms 重启服务
	upgrade.RestartServiceAsync(800 * time.Millisecond)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "升级文件已成功落盘并替换，服务正在平滑重启...",
	})
}


