package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/api"
	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/component/cloudflared"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

var (
	portFlag        = flag.Int("port", 20888, "HTTP server listening port")
	dbPathFlag      = flag.String("db", "/etc/vps-auto-deploy/data.db", "SQLite database path")
	resetUserFlag   = flag.String("reset-username", "", "Reset admin username")
	resetPassFlag   = flag.String("reset-password", "", "Reset admin password")
	resetSecretFlag = flag.String("reset-secret", "", "Reset secret path")
)

// webFS 允许外部通过 embed 方式注入静态资源
var WebFS fs.FS

func init() {
	// 注册组件驱动
	component.Register(singbox.NewDriver())
	component.Register(cloudflared.NewDriver())
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

	// 3. 读取系统关键参数并打印启动信息
	secretPath, _ := model.GetSetting("secret_path")
	publicIP := netutil.GetPublicIP()

	r := api.SetupRouter(WebFS)

	addr := fmt.Sprintf(":%d", *portFlag)
	fmt.Println("==================================================================")
	fmt.Println("           VPS Auto Deploy Server (Control Plane)                 ")
	fmt.Println("==================================================================")
	fmt.Printf(" [!] Access URL:   http://%s:%d/%s/\n", publicIP, *portFlag, secretPath)
	fmt.Printf(" [!] Local URL:    http://127.0.0.1:%d/%s/\n", *portFlag, secretPath)
	fmt.Printf(" [!] Default User: admin\n")
	fmt.Printf(" [!] Default Pass: admin888 (Please change after initial login!)\n")
	fmt.Println("==================================================================")

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Server startup failed: %v", err)
	}
}

