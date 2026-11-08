package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/api"
	"vps-auto-deploy/internal/cfpref"
	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/component/cloudflared"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/component/subconverter"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
	"vps-auto-deploy/internal/pkg/crypto"
	"vps-auto-deploy/internal/pkg/netutil"
)

var (
	portFlag        = flag.Int("port", 20888, "HTTP server listening port")
	dbPathFlag      = flag.String("db", "/etc/vps-auto-deploy/data.db", "SQLite database path")
	resetUserFlag   = flag.String("reset-username", "", "Reset admin username")
	resetPassFlag   = flag.String("reset-password", "", "Reset admin password")
	resetSecretFlag = flag.String("reset-secret", "", "Reset secret path")
	tlsFlag         = flag.Bool("tls", false, "Enable HTTPS with self-signed certificate (Phase 1 MITM protection)")
	certFlag        = flag.String("cert", "/etc/vps-auto-deploy/cert.pem", "TLS certificate file path")
	keyFlag         = flag.String("key", "/etc/vps-auto-deploy/key.pem", "TLS private key file path")
	printFpFlag     = flag.Bool("print-fingerprint", false, "Print TLS certificate SHA-256 fingerprint and exit")
)

// webFS 允许外部通过 embed 方式注入静态资源
var WebFS fs.FS

func init() {
	// 注册组件驱动
	component.Register(singbox.NewDriver())
	component.Register(cloudflared.NewDriver())
	component.Register(subconverter.NewDriver())
}

func main() {
	flag.Parse()

	// 1. 初始化数据库
	dbPath := *dbPathFlag
	// 若无 root 权限或指定开发环境，降级到当前目录
	if os.Geteuid() != 0 && dbPath == "/etc/vps-auto-deploy/data.db" {
		dbPath = "./data.db"
	}

	_, err := model.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 自动探测并导入宿主机已有 sing-box 配置（当数据库入站规则为空时）
	var ibCount int
	_ = model.DB.QueryRow("SELECT COUNT(*) FROM inbounds").Scan(&ibCount)
	if ibCount == 0 {
		cfgPath := singbox.GetConfigPath()
		if fi, err := os.Stat(cfgPath); err == nil && !fi.IsDir() {
			stats, err := configgen.ImportExistingSingboxConfig(cfgPath)
			if err == nil && (stats.InboundsImported > 0 || stats.MembersImported > 0) {
				log.Printf("[Auto-Import] 成功从宿主机配置 %s 自动导入 %d 个入站节点、%d 个成员与 %d 个出站规则", cfgPath, stats.InboundsImported, stats.MembersImported, stats.OutboundsImported)
			}
		}
	}

	// 2. 处理重置管理员账号、密码与安全路径的 CLI 参数
	hasReset := false
	if *resetUserFlag != "" {
		_ = model.SetSetting("admin_username", *resetUserFlag)
		log.Printf("[CLI] Admin username set to: %s", *resetUserFlag)
		hasReset = true
	}
	if *resetPassFlag != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(*resetPassFlag), bcrypt.DefaultCost)
		_ = model.SetSetting("admin_password_hash", string(hash))
		log.Printf("[CLI] Admin password has been reset successfully.")
		hasReset = true
	}
	if *resetSecretFlag != "" {
		_ = model.SetSetting("secret_path", *resetSecretFlag)
		log.Printf("[CLI] Secret path reset to: /%s/", *resetSecretFlag)
		hasReset = true
	}
	if hasReset {
		return
	}

	// 3. 处理证书路径适配与指纹打印
	certPath := *certFlag
	keyPath := *keyFlag
	if os.Geteuid() != 0 && certPath == "/etc/vps-auto-deploy/cert.pem" {
		certPath = "./cert.pem"
		keyPath = "./key.pem"
	}

	if *printFpFlag {
		fp, err := crypto.LoadOrGenerateCert(certPath, keyPath, []string{"127.0.0.1", "localhost", netutil.GetPublicIP()})
		if err != nil {
			log.Fatalf("Failed to generate or load certificate: %v", err)
		}
		fmt.Println(fp)
		return
	}

	// 4. 读取系统关键参数并打印启动信息
	secretPath, _ := model.GetSetting("secret_path")
	publicIP := netutil.GetPublicIP()

	var certFingerprint string
	if *tlsFlag {
		var err error
		certFingerprint, err = crypto.LoadOrGenerateCert(certPath, keyPath, []string{"127.0.0.1", "localhost", publicIP})
		if err != nil {
			log.Fatalf("Failed to prepare TLS certificates: %v", err)
		}
	}

	// 5. 启动后台受管节点心跳保活守护协程 (每 30 秒自动巡检与自愈)
	node.StartHealthCheckWorker(30 * time.Second)

	// 6. 启动 Cloudflare 优选 IP 定时拉取与调度协程
	cfpref.StartWorker()

	r := api.SetupRouter(WebFS)

	addr := fmt.Sprintf(":%d", *portFlag)
	protocol := "http"
	if *tlsFlag {
		protocol = "https"
	}

	// 检查当前管理员密码是否仍为初始弱密码 (admin888)
	hashedPass, _ := model.GetSetting("admin_password_hash")
	adminUser, _ := model.GetSetting("admin_username")
	if adminUser == "" {
		adminUser = "admin"
	}
	isDefaultPass := false
	if hashedPass != "" && bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte("admin888")) == nil {
		isDefaultPass = true
	}

	fmt.Println("==================================================================")
	fmt.Println("           VPS Auto Deploy Server (Control Plane)                 ")
	fmt.Println("==================================================================")
	fmt.Printf(" [!] Access URL:   %s://%s:%d/%s/\n", protocol, publicIP, *portFlag, secretPath)
	fmt.Printf(" [!] Local URL:    %s://127.0.0.1:%d/%s/\n", protocol, *portFlag, secretPath)
	fmt.Printf(" [!] Admin User:   %s\n", adminUser)
	if isDefaultPass {
		fmt.Printf(" [!] [ALERT] 当前仍在使用默认弱密码: admin888 (请立即在后台修改!)\n")
	} else {
		fmt.Printf(" [!] Admin Pass:   [CONFIGURED & PROTECTED]\n")
	}
	if *tlsFlag {
		fmt.Printf(" [!] TLS Security: ENABLED (MITM Protection)\n")
		fmt.Printf(" [!] Pinned FP:    %s\n", certFingerprint)
	}
	fmt.Println("==================================================================")

	if *tlsFlag {
		if err := http.ListenAndServeTLS(addr, certPath, keyPath, r); err != nil {
			log.Fatalf("TLS Server startup failed: %v", err)
		}
	} else {
		if err := http.ListenAndServe(addr, r); err != nil {
			log.Fatalf("Server startup failed: %v", err)
		}
	}
}

