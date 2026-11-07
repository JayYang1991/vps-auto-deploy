# VPS Auto Deploy - 代理与隧道自动化管理服务

面向个人与小团队自用的轻量级 Linux 代理与穿透隧道管理面板。基于 **Go + Vue 3 (TypeScript)** 单二进制内嵌架构，零外部依赖，极低内存占用（< 30MB）。

---

## ✨ 核心特性

- **单二进制零依赖交付**：
  - 前端编译产物直接内嵌至服务端二进制中（`embed.FS`），集成纯 Go SQLite 驱动（无需 gcc/CGO）。
  - **初次部署零捆绑**：初始部署仅运行控制面板，不硬编码下载任何第三方组件，保持系统极度干净。
- **Web 端软件中心 (Component Hub)**：
  - 在界面中按需一键安装、升级或卸载 **Sing-box**、**Cloudflared**、**Subconverter 转换引擎** 等底层组件。
  - 异步安装任务配合 SSE 实时终端日志流，清晰掌控下载、解压、校验及 Systemd 守护进程注册全流程。
- **协议与穿透开箱即用**：
  - **直连首选：VLESS-XTLS-Vision-Reality**：集成 X25519 密钥协商，一键生成服务端私钥、客户端公钥 (pbk) 与 ShortID。
  - **隧道穿透：VLESS-WS + Cloudflared (Argo)**：支持 Quick Tunnel 免登录临时隧道与 Zero Trust Token 固定域名隧道。
- **跨客户端订阅分发与 Subconverter 引擎**：
  - **双引擎分发**：内置轻量级 Go 订阅生成器，同时深度集成 MetaCubeX/subconverter 订阅转换引擎。
  - **ACL4SSR 远程分流模板**：一键生成全套 Clash 策略组（自动测速 / 手动选择 / 漏网之鱼）及丰富分流规则集。
  - **Cloudflare 优选 IP 自动化**：支持定时自动抓取与手动刷新优选 IP/端口，并与节点分发联动。
  - **全集群节点清单透视**：按节点主机聚合展示全集群入站规则，支持批量复制、二维码扫码及 Clash YAML 片段导出。
- **多节点集群纳管 (Cluster Management)**：
  - 单主控管理多台被纳管 VPS，通过 SSH 极速纳管，反向代理透明管理各节点配置与状态。
  - 支持 ECDSA P-256 证书指纹锁定与 AES-256-GCM 凭据二次鉴权保护。
- **安全与防护**：
  - **动态安全入口路径 (Secret Base Path)**：默认随机生成访问路径（如 `http://<IP>:20888/fa480cbd/`），未授权直接 404 伪装。
  - **多成员独立授权**：团队成员拥有专属 UUID 与独立订阅 Token，随时可重置或启停。

---

## 🚀 快速开始

### 方式一：下载 Release 压缩包一键部署（推荐，解压即用）

从 [GitHub Releases](https://github.com/JayYang1991/vps-auto-deploy/releases) 下载对应系统架构的压缩包（以 `linux_amd64` 为例）：

```bash
# 1. 下载最新发布压缩包 (amd64 / arm64)
curl -L -O https://github.com/JayYang1991/vps-auto-deploy/releases/latest/download/vps-panel_linux_amd64.tar.gz

# 2. 解压
tar -zxvf vps-panel_linux_amd64.tar.gz

# 3. 执行解压后的一键部署脚本 (支持 root 系统级或非 root 用户空间运行)
sudo bash install.sh
```

> **ARM64 机器（如甲骨文 ARM、树莓派）**：请将下载链接中的 `amd64` 替换为 `arm64` 即可（`vps-panel_linux_arm64.tar.gz`）。

---

### 方式二：在线网络一键脚本直接部署

适用于全新安装的 Ubuntu / Debian / CentOS 服务器：

```bash
curl -fsSL https://raw.githubusercontent.com/JayYang1991/vps-auto-deploy/main/scripts/install.sh | sudo bash
```

安装脚本会自动探测系统环境、创建运行目录、注册 Systemd 守护进程并启动服务。完成后控制台会打印专属访问入口：

```text
==========================================================
 [✓] 部署完成！服务已在后台保持自启运行 (systemd: vps-panel)
==========================================================
 [!] Access URL:   http://<YOUR_IP>:20888/<SECRET_PATH>/
 [!] Local URL:    http://127.0.0.1:20888/<SECRET_PATH>/
 [!] Default User: admin
 [!] Default Pass: admin888 (请登录后立即修改!)
==========================================================
```

---

### 方式三：本地从源码编译与打包

```bash
# 依赖环境：Go 1.22+，Node.js 20+，pnpm
git clone https://github.com/JayYang1991/vps-auto-deploy.git
cd vps-auto-deploy

# 编译生成当前系统的单一静态可执行文件
./build.sh

# 或者：打包跨架构全套 Release 压缩包 (amd64 + arm64)
./scripts/package-release.sh v1.0.0

# 启动面板服务
./bin/vps-panel --port 20888 --db ./data.db
```

---

## 🛠️ CLI 常用运维命令

```bash
# 查看面板服务运行状态
systemctl status vps-panel

# 查看面板实时日志
journalctl -u vps-panel -f

# 重启 / 停止面板服务
systemctl restart vps-panel
systemctl stop vps-panel

# 忘记密码时重置管理员密码
/usr/local/bin/vps-panel --db /etc/vps-auto-deploy/data.db --reset-password <新密码>

# 重置安全访问路径前缀
/usr/local/bin/vps-panel --db /etc/vps-auto-deploy/data.db --reset-secret <新路径名>
```

---

## 📡 客户端订阅分发

订阅地址支持根据客户端 `User-Agent` 自动识别分发，或指定 `target` 参数强制导出：

| 格式 | 请求示例 | 说明 |
| :--- | :--- | :--- |
| **智能识别 (推荐)** | `http://<HOST>:<PORT>/sub/<TOKEN>` | 自动匹配 Clash、Sing-box、Shadowrocket、v2rayN |
| **Clash (Mihomo)** | `http://<HOST>:<PORT>/sub/<TOKEN>?target=clash` | 导出 Clash YAML 规则组与节点配置 |
| **Sing-box 客户端** | `http://<HOST>:<PORT>/sub/<TOKEN>?target=singbox` | 导出 Sing-box 客户端完整 Outbounds JSON |
| **通用 Base64 / URI** | `http://<HOST>:<PORT>/sub/<TOKEN>?target=base64` | 导出通用 Base64 节点 URI 列表 |
| **Subconverter 高级** | `http://<HOST>:<PORT>/sub/<TOKEN>?engine=subconverter&target=clash` | 调用 Subconverter 引擎生成 ACL4SSR 分流规则 |

---

## 📂 项目结构

```text
├── .github/
│   └── workflows/
│       └── release.yml     # GitHub Actions 自动化 Tag 发布与跨架构打包
├── cmd/
│   └── server/             # 程序入口、静态资源嵌入 (embed.FS) 与 CLI 参数
├── internal/
│   ├── api/                # Gin HTTP 路由、JWT 中间件与 API Handler
│   ├── cfpref/             # Cloudflare 优选 IP 定时拉取与规则合并
│   ├── component/          # 组件驱动抽象、GitHub Release 抓取与 Systemd 控制
│   │   ├── singbox/        # sing-box 内核驱动与 Reality 密钥协商
│   │   ├── cloudflared/    # cloudflared Argo 穿透驱动
│   │   └── subconverter/   # subconverter 转换引擎驱动与资产管理
│   ├── configgen/          # 服务端配置合成、JSON 预检与动态热重载
│   ├── model/              # SQLite 数据库模型、数据表结构与加密存储
│   ├── node/               # 多节点集群纳管、SSH 推流与 TOFU 证书指纹锁定
│   ├── pkg/                # 网络探测、AES 加密与 SSE 日志流工具
│   └── subgen/             # 多客户端多协议订阅生成核心
├── scripts/
│   ├── install.sh          # 一键部署与 Systemd 注册脚本（适配 Release 压缩包解压）
│   └── package-release.sh  # 本地多架构 Release 交叉编译与归档脚本
├── web/                    # Vue 3 + Vite + TypeScript 前端工作区
├── build.sh                # 本地单架构快速编译脚本
└── README.md
```
