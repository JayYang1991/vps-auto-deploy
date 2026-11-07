# VPS Auto Deploy - 代理与隧道自动化管理服务

面向个人与小团队自用的轻量级 Ubuntu 代理与隧道管理面板。基于 **Go + Vue 3 (TypeScript)** 单二进制内嵌架构，零外部依赖，极低内存占用（< 30MB）。

---

## ✨ 核心特性

- **轻量解耦 & 按需管理**：
  - **初次部署零捆绑**：初始一键安装脚本仅部署管理服务端（Control Plane），不强制安装任何后端代理软件。
  - **Web 端组件商店**：通过 Web 界面按需选择安装、升级或卸载 **sing-box**、**cloudflared** 等组件。
  - **安装过程透明可控**：点击安装/升级时弹出实时终端日志流（SSE），清晰展示下载、解压、校验及 Systemd 服务注册全流程。
- **软件生命周期管理**：
  - 自动查询 GitHub Release 最新版本，对比当前安装版本并高亮显示可更新标识。
  - 原生支持服务的启动、停止、重启、开机自启与一键安全卸载。
  - Systemd 原生托管，提供网页端实时 `journalctl` 日志流监控。
- **协议开箱即用**：
  - **直连首选：VLESS-XTLS-Vision-Reality**：集成标准库与内核 X25519 密钥协商，一键自动生成私钥、公钥 (pbk) 与 ShortID。
  - **隧道救砖：VLESS-WS + Cloudflared (Argo)**：支持 Quick Tunnel 自动免登录试用隧道与 Zero Trust Token 固定域名隧道。
- **自动化多格式订阅生成引擎**：
  - **Clash (Mihomo) YAML**：生成包含 VLESS-Reality、VLESS-WS 完整配置、策略组（SELECT/URL-Test/Fallback）及基础分流规则。
  - **Sing-box 官方客户端 JSON**：生成 Sing-box 客户端专用的完整 Outbounds 配置。
  - **通用 Base64 / URI**：生成标准 `vless://` 链接与二维码，兼容 Shadowrocket、v2rayN。
- **安全与防护**：
  - **动态安全入口路径 (Secret Base Path)**：默认随机生成访问路径（如 `http://<IP>:<PORT>/a8f9d2/`），根路径直接 404 伪装，防扫描器嗅探。
  - **小团队多成员模式**：每个成员拥有独立的 UUID 与免登录专属订阅 Token，共享同一入站端口，支持随时重置或注销。

---

## 🚀 快速开始

### 方式一：Ubuntu 一键脚本安装（推荐）

```bash
# 在 Ubuntu 服务器上运行（需要 root 权限）
sudo bash scripts/install.sh
```

安装完成后，脚本会自动输出当前面板的专属访问路径与初始密码：
```text
==========================================================
 [✓] 安装完成！服务已在后台保持自启运行 (systemd: vps-panel)
==========================================================
 [!] Access URL:   http://<YOUR_IP>:20888/<SECRET_PATH>/
 [!] Default User: admin
 [!] Default Pass: admin888 (请登录后立即修改!)
==========================================================
```

### 方式二：本地自行构建

```bash
# 依赖环境：Go 1.22+，Node.js 20+，pnpm
git clone https://github.com/vps-auto-deploy/vps-auto-deploy.git
cd vps-auto-deploy

# 一键编译前端并打包为单一静态二进制文件
./build.sh

# 启动服务
./bin/vps-panel --port 20888 --db ./data.db
```

---

## 🛠️ CLI 命令与运维

```bash
# 查看面板服务运行状态
systemctl status vps-panel

# 查看实时日志
journalctl -u vps-panel -f

# 忘记密码时重置管理员密码
/usr/local/bin/vps-panel --db /etc/vps-auto-deploy/data.db --reset-password <新密码>

# 重置安全路径前缀
/usr/local/bin/vps-panel --db /etc/vps-auto-deploy/data.db --reset-secret <新路径名>
```

---

## 📡 订阅分发说明

订阅地址支持通过 `target` 参数或客户端 `User-Agent` 自动匹配返回格式：

- **自动识别 (推荐)**: `http://<HOST>:<PORT>/sub/<SUB_TOKEN>`
- **强制 Clash Meta (Mihomo)**: `http://<HOST>:<PORT>/sub/<SUB_TOKEN>?target=clash`
- **强制 Sing-box 客户端**: `http://<HOST>:<PORT>/sub/<SUB_TOKEN>?target=singbox`
- **强制 Base64 / URI**: `http://<HOST>:<PORT>/sub/<SUB_TOKEN>?target=base64`

---

## 📂 项目结构

```text
├── cmd/
│   └── server/             # 程序入口与 Web 资源嵌入 (embed.FS)
├── internal/
│   ├── api/                # Gin 路由、JWT 鉴权中间件与 HTTP Handler
│   ├── component/          # 组件驱动抽象、GitHub Release 下载与 Systemd 管理
│   │   ├── singbox/        # sing-box 内核驱动与 Reality 密钥生成
│   │   └── cloudflared/    # cloudflared 驱动与隧道提取
│   ├── configgen/          # sing-box 服务端配置合成与合法性预校验
│   ├── model/              # SQLite 数据库模型与数据迁移
│   ├── pkg/                # 公网 IP 检测、命令执行与 SSE 日志流
│   └── subgen/             # 多格式订阅生成引擎 (Clash/Sing-box/URI)
├── scripts/
│   └── install.sh          # Ubuntu 一键部署与注册脚本
├── web/                    # Vue 3 + Vite + TypeScript 前端工程
└── build.sh                # 前后端联合一键构建脚本
```

