# AGENTS.md — 架构规范、扩展指南与前端交互设计契约

本文件为 **VPS Auto Deploy** 项目的最高工程与设计准则。所有后续参与开发、重构或扩展组件（如 `subconverter`、`hysteria`、`mosdns`、`frp` 等）的 AI Agent 与开发者，**必须严格遵循本文件中的架构约束与 UI/UX 规范**。

---

## 一、 核心设计哲学 (Core Philosophy)

1. **单二进制零依赖交付 (Single-Binary Zero-Dependency)**
   - 编译产物 `bin/vps-panel` 必须内嵌前端静态资源（`embed.FS`）并使用纯 Go SQLite 驱动（`modernc.org/sqlite`，严禁开启 `CGO_ENABLED=1`）。
   - 初始部署（`scripts/install.sh`）**仅安装并启动管理控制端 (`vps-panel`)**，严禁在安装脚本中硬编码强制下载任何第三方代理或穿透组件。
2. **基础设施与业务配置解耦 (Infrastructure vs. Service Configuration Separation)**
   - **基础软件层（安装与生命周期）**：负责二进制探测、下载、解压、资产目录管理、Systemd 守护进程注册、启停与卸载。
   - **业务配置层（节点、隧道、订阅规则）**：负责协议参数建模、配置文件生成、客户端订阅分发。业务配置在底层组件未安装时仍可落盘保存，并通过**渐进式空状态 (Progressive Empty State)** 引导用户前往软件中心安装。
3. **非阻塞式现代交互 (Zero Native Blocking Dialogs)**
   - **严禁在前端任何位置使用浏览器原生阻塞弹窗**：禁止出现 `window.alert()`、`window.confirm()`、`window.prompt()`。
   - 所有反馈必须使用项目内置的非阻塞交互原语：`useToast()`（右上角轻量通知）、`useConfirm()`（毛玻璃内嵌确认框）与 `copyToClipboard()`（静默复制反馈）。

---

## 二、 后端组件驱动架构规范 (Backend Component Architecture)

### 1. 组件驱动接口契约 (`internal/component/driver.go`)

所有受管软件必须实现 `ComponentDriver` 接口，并通过 `component.Register()` 在 `cmd/server/main.go` 中注册。

```go
type ServiceStatus struct {
    Name             string `json:"name"`              // 唯一标识，如 "sing-box", "cloudflared", "subconverter"
    DisplayName      string `json:"display_name"`      // 界面展示名，如 "Subconverter 订阅转换"
    Description      string `json:"description"`       // 简短职责描述
    Category         string `json:"category"`          // 分类枚举: "proxy" | "tunnel" | "converter" | "network"
    CategoryLabel    string `json:"category_label"`    // 分类中文名: "代理核心" | "隧道穿透" | "订阅与规则" | "网络工具"
    ConfigRoute      string `json:"config_route"`      // 关联的业务配置路由，如 "/inbounds", "/tunnels", "/subscriptions"
    Order            int    `json:"order"`             // 固定排序权重，防止 Go map 随机遍历导致前端卡片跳动
    Installed        bool   `json:"installed"`
    InstalledVersion string `json:"installed_version"`
    LatestVersion    string `json:"latest_version"`
    HasUpdate        bool   `json:"has_update"`
    Active           bool   `json:"active"`
    RunningStatus    string `json:"running_status"`    // "running" | "stopped" | "failed" | "not_installed"
}
```

### 2. 预装探测与二进制寻址规范 (`internal/component/util.go`)
- **禁止硬编码单一路径**：宿主机可能在部署本面板前已通过 `apt`、`.deb` 或第三方脚本安装了目标组件。
- 驱动必须通过 `component.FindBinary(name, candidates...)` 动态探测 `$PATH`、`/usr/local/bin`、`/usr/bin`、`~/.local/bin` 及组件专属运行时目录。
- **状态双检机制**：`Status()` 必须同时检查受管 `systemd` 服务状态与宿主机进程（`component.IsProcessRunning`），确保预装或外部守护的进程能正确识别为 `installed: true` 及 `running`。
- **版本号提取**：统一使用 `component.ExtractVersion(output)` 提取语义化版本号，兼容不同软件的 `-v` / `--version` / `version` 输出格式。

### 3. 单文件型组件 vs. 目录资产型组件（以 `subconverter` 为例）
受管软件按分发形态分为两类，驱动实现必须区分处理：
- **单二进制型**（如 `cloudflared`、`sing-box`）：
  - 解压后直接将二进制安装至 `/usr/local/bin/<name>`（或非 Root 模式下的 `~/.local/bin/<name>`）。
- **目录资产依赖型**（如 `subconverter` 含 `rules/`、`snippets/` 等资源）：
  - 例如 `subconverter` 运行时强依赖同级目录下的 `base/`、`rules/`、`snippets/`、`pref.ini`。
  - **规范**：必须将其完整解压至独立运行时工作目录 `/etc/vps-auto-deploy/components/<name>/`（非 Root 为 `~/.config/vps-auto-deploy/components/<name>/`）。
  - 注册 Systemd 服务时，必须显式设置 `WorkingDirectory=/etc/vps-auto-deploy/components/<name>`，确保相对路径资源加载正常。
  - 卸载（`Uninstall()`）时，清理二进制与运行时程序文件，但默认保留用户自定义的配置文件或提供明确提示。

### 4. 异步安装任务与 SSE 实时日志流
- 所有组件的安装/升级必须在后台 Goroutine 中通过 `StartInstallTask(name, version)` 异步执行。
- 驱动内部通过 `logFn func(string)` 实时上报每个阶段（架构识别、获取 Release、下载包、解压校验、写入 Systemd、启动验证）。
- 成功必须输出 `[SUCCESS]` 标记，失败必须输出 `[ERROR]` 标记，前端 SSE 终端据此切换状态。

---

## 三、 前端信息架构与 UI/UX 设计约束 (Frontend & UI Design Rules)

### 1. 两大核心版块与业务配置中心化 (Categorized Service Hub)

界面顶层导航（`web/src/App.vue`）严格划分为四大顶级入口，**严禁将各个具体组件的配置按钮直接平铺在顶部导航栏中**：

| 版块分组 | 导航标识 | 核心职责 | 对应路由 |
| :--- | :--- | :--- | :--- |
| **版块一：基础软件** | `📊 系统概览`<br>`📦 软件中心` | 监控硬件与状态、底层驱动与扩展软件的安装、版本升级、卸载、启停与实时终端日志流 | `/`<br>`/components` |
| **版块二：业务配置** | `🛠️ 配置中心` | **所有网络协议节点、反代隧道、用户权限与规则转换的统一分类工作台** | `/services` *(支持 `?tab=inbounds` / `?tab=tunnels` / `?tab=subscriptions`)* |
| **系统管理** | `⚙️ 设置` | 面板安全路径、公网 IP 强制覆盖、管理员密码修改 | `/settings` |

### 2. 业务配置中心 (`web/src/views/Services.vue`) 架构规范
- **禁止顶栏平铺铁律**：后续新增任何组件业务（如 `subconverter` 规则编辑、`hysteria` 高速入站、`mosdns` 分流规则），**严禁在 `App.vue` 顶栏增加新的路由按钮**，必须在 `Services.vue`（业务配置中心）内按组件挂载。
- **两级分层设计（按组件配置 + 子业务工作区）**：
  1. **一级分类（受管组件）**：
     - **📦 Sing-box 核心 (`singbox`)**：底层核心网络引擎。
     - **🚇 Cloudflared 隧道 (`cloudflared`)**：公网 Argo 穿透与端口反代。
     - **👥 订阅与规则分发 (`subscriptions`)**：多协议多客户端订阅与规则模板生成。
  2. **二级子分类（组件专属工作区，以 Sing-box 为例）**：
     - **🌐 入站节点 (Inbounds)**：完整支持增、删、改、查（CRUD）、监听地址与端口管理、一键启用/停用、Reality 密钥伪装生成与二维码/链接导出。
     - **📤 出站规则 (Outbounds)**：完整支持增删改查、直连/拦截/代理出站（SOCKS/HTTP/Shadowsocks/VLESS/自定义 JSON）与优先级权重排序。
     - **🔀 路由分流 (Routes)**：支持 Sing-box 1.14 最新语法（`ip_is_private`、根级 `http_clients`、`route.rule_set` 远程二进制 SRS、`route.rules` 条件组合、优先级次序调整与常用工业级一键预设）。
     - **📜 完整配置透视与热重载**：提供 live 完整服务端 JSON 查看、落盘路径校验、`singbox check` 语法预检及 `[ ⚡ 强制热重载 ]` 触发。
- **权责对齐与防冗余设计铁律 (Scope Alignment & Redundancy Prevention)**：
  - **全局高危动作统一收敛**：面板遵循“数据库即单一真相源 (Database as Single Source of Truth)”。凡涉及“读取磁盘文件全量反向覆盖数据库”或“将数据库全量渲染写入磁盘并重启内核”等跨实体的全局性、破坏性操作，**必须统一收敛在组件顶层工作区操作栏 (`Services.vue`) 或配置透视对比弹窗 (`ConfigViewer`) 中**。
  - **严禁在子实体视图下钻散落全局按钮**：严禁在具体子列表（如 `Inbounds.vue` 入站列表、`Outbounds.vue` 出站列表、`Routes.vue` 分流列表）顶部重复放置“从磁盘覆盖数据库”等全局高危按钮。
    > **违背理由**：严重违反最小惊讶原则（Principle of Least Astonishment）。用户在“出站规则”页点击覆盖，后端却同时覆盖了入站与成员，极易造成权责认知错位与误操作。子视图顶部必须保持干净聚焦，**仅允许放置该子实体的核心增删改查动作**（如 `[ + 新增节点 ]`、`[ + 新增出站规则 ]`）。
  - **冲突决策渐进呈现与对称闭环**：“以磁盘为准导入面板”与“以面板为准写入磁盘”属于不可逆决策，**最佳交互路径是在 `[ 🔍 配置透视与对比 ]` 弹窗检测到冲突时以对称按钮呈现**（`[ 📥 以磁盘为准导入面板 ]` vs `[ ⚡ 以面板为准写入磁盘 ]`），让管理员看清差异后再行决策，严禁在子页面散落盲测式覆盖按钮。
- **状态持久化与平滑切换**：
  - 子视图之间切换使用 `<keep-alive>`，确保在配置入站或出站表单时临时切换 Tab 不会丢失表单输入状态。
  - 路由兼容历史与快捷直达：`/services?tab=singbox&sub=inbounds`、`/services?tab=singbox&sub=outbounds`、`/services?tab=singbox&sub=routes`、`/services?tab=cloudflared`，历史路由 `?tab=inbounds`、`?tab=outbounds`、`?tab=routes`、`?tab=tunnels` 自动平滑重定向。

### 3. 软件中心 (`web/src/views/Components.vue`) 扩展约束
- **动态分类过滤**：顶部分类 Tab 应支持按 `category` 过滤（`全部服务`、`代理核心 (proxy)`、`隧道穿透 (tunnel)`、`订阅与规则 (converter)` 等），禁止使用非此即彼的二元三目运算符（如 `c.category === 'proxy' ? ... : ...`），必须使用字典映射表 `CATEGORY_MAP[c.category]`。
- **跨版块联动跳转**：
  - 已安装的组件卡片上若存在关联配置，提供快捷按钮 `[ 🛠️ 去配置 ]` 跳转到 `/services?tab=<key>`。
  - 业务配置页检测到依赖组件未安装时，顶部渲染琥珀色渐进式引导卡片，点击 `[ 🚀 前往「软件中心」一键安装 ]` 携带参数跳转 `/components?install=<name>`，软件中心自动高亮该组件并弹出安装确认。
- **辅助增强型组件联动（如 `subconverter`）**：
  - 当 `subconverter` 未安装时，`订阅与规则` Tab 使用内置轻量 Go 生成器提供基础 Clash/Sing-box/Base64 订阅。
  - 当检测到 `subconverter` 已安装且运行中时，`订阅与规则` Tab 自动激活「高级远程规则模板转换 (Subconverter 引擎)」配置面板。

### 4. 全局非阻塞反馈调用规范
修改或新增任何 `.vue` 页面时，必须按以下方式处理用户反馈：
- **消息提示**：
  ```ts
  import { useToast, copyToClipboard } from '../composables/useToast'
  const { toast } = useToast()
  toast.success('保存成功')
  toast.error('操作失败: ' + errMsg)
  toast.warning('请先填写必填项')
  ```
- **剪贴板复制**：
  ```ts
  copyToClipboard(text, '订阅链接')
  ```
- **二次确认（删除/卸载/重置）**：
  ```ts
  import { useConfirm } from '../composables/useConfirm'
  const { showConfirm } = useConfirm()
  const ok = await showConfirm({
    title: '确认删除',
    message: '删除后不可恢复，是否继续？',
    confirmText: '确认删除',
    danger: true,
  })
  if (!ok) return
  ```

### 5. 超大文本、订阅与日志预览防卡顿设计契约 (Large Text & Safe Rendering Contract)

前端在处理组件实时日志流、Subconverter 订阅转换结果、Sing-box 完整配置透视或规则集文件时，往往面临 **数万行代码（2MB ~ 10MB+）** 的超大文本。为杜绝浏览器主线程冻结与 Layout Thrashing 卡顿，**必须严格遵守以下 5 项前端防卡顿契约**：

1. **视口定高虚拟滚动与安全渲染规范（Virtual Viewport Windowing & Safe Rendering）**：
   - 严禁将数万行未分页文本一次性全量挂载进单个 `<pre>` 或 `<div>` 标签中。
   - **首选虚拟滚动方案**：超大文本预览统一使用项目封装的高性能纯原生视口组件 `<VirtualCodePreview :content="..." />`。该组件基于 `Uint32Array` 极速行偏移量索引与 `translateY` 视口滑动窗口，**无论文本多达 5 万行还是 50 万行，DOM 树恒定仅渲染视口内的 25~35 行（$O(1)$ 复杂度）**，支持真实比例的原生滚动条拖拽与首尾/指定行号跳转，保障 60 FPS 丝滑浏览。
   - 若在轻量行内场景使用简易预览，必须采用安全截断展示并辅以一键导出与下载按钮。
2. **严禁在 Vue 模板中执行高开销内存分配**：
   - **绝对禁止在模板表达式中直接调用 `.split('\n')`**（如 `{{ text.split('\n').length }}`）。每次微小渲染都会在 JavaScript 堆中新建数万个小字符串数组，引发连续 Major GC 停顿。
   - 所有文本行数、体积等元数据指标，必须在 `<script setup>` 中使用 `computed` 计算并缓存（推荐使用快速字符遍历统计换行符 `\n`，做到零中间数组内存分配）。
3. **只读超大文本的浅响应式设计**：
   - 对于仅用于展示、下载与复制的静态转换结果或历史日志，必须使用 `shallowRef('')` 代替深层响应式 `ref('')`，杜绝 Vue 对几兆字节文本进行无意义的响应式深度劫持。
4. **完整内容多轨交付机制**：
   - 即使预览进行了截断，必须保障“**复制即全量**”：点击 `[ 📋 复制全量内容 ]` 时必须使用内存中持有的完整源字符串（`copyToClipboard(rawResult.value)`）。
   - 针对超大配置文件，必须额外提供 `[ 📥 下载配置文件 ]` 功能，通过客户端原生 `Blob` 生成 URL，直接触发本地文件保存，免去浏览器复制粘贴超大文本导致剪贴板阻塞。
5. **选择器与高级源码编辑器权责解耦**：
   - 下拉选择框（`<select>`）仅用于配置目标与预设模板的快速切换，**严禁在用户切换下拉项时主动触发全屏或弹出式代码编辑窗口**。
   - 源码编辑必须通过独立且显式的 `[ ✏️ 打开编辑器 ]` 入口触发，并在当前选用本地规则时通过非侵入式的轻量提示条展示落盘状态与跳转入口。

### 6. 模态框与遮罩层防误触关闭铁律 (Anti-Accidental Dismissal Contract)

所有前端模态框（Modal）、浮层抽屉（Drawer）与查看器组件，**严禁在外部遮罩层上绑定任何可能导致弹窗关闭的点击事件**（严格禁止 `@click.self="close"`、`@click.self="modal.show = false"` 或类似逻辑）：

1. **违背理由与隐患剖析**：
   - **表单编辑灾难性丢失**：用户在录入复杂网络参数（入站端口、UUID、伪装域名、证书私钥、路由条件、出站代理链）时，鼠标移出输入框在窗口边缘空白区域误点，弹窗瞬间关闭，导致所有未落盘的编辑成果全量丢失；
   - **文本划选复制误杀**：在配置透视对比、日志流查看器或凭据查看窗口中，用户鼠标拖拽划选文字时，鼠标在弹窗边界外释放（Mouse Up）极易触发遮罩层的 click 事件，引发弹窗误杀；
   - **高危决策失真**：云主机销毁、节点剔除、服务卸载等高危确认弹窗，必须强制用户通过显式的 `[ 确认 ]` 或 `[ 取消 ]` 按钮做出决策，禁止点击背景隐式退出；
   - **长连接与异步任务视线脱离**：Web 控制台 SSH PTY、组件一键安装流、推流升级正在执行，点击背景关闭会导致进程脱离视线，造成用户误以为任务中断。
2. **唯一合法关闭途径**：
   所有模态框的关闭动作必须且只能由用户显式触发：
   - 弹窗右上角显式的 `[ ✕ ]` 图标按钮；
   - 弹窗底部操作栏显式的 `[ 取消 ]` 或 `[ 关闭 ]` 按钮。
3. **静态扫描规范**：
   每次提交前必须执行静态检查：`grep -rnE "@click\.self" web/src`，输出必须恒定为空。

---

## 四、 新增受管软件的标准 SOP 清单 (Step-by-Step Guide for Agents)

当用户要求“新增支持 XXX 软件（如 `subconverter`、`hysteria` 等）”时，Agent 必须按以下 5 步标准流程执行：

1. **创建驱动包**：在 `internal/component/<name>/driver.go` 实现 `ComponentDriver` 接口：
   - 设置准确的 `Category()`（如 `"converter"`）与 `Order()`。
   - 使用 `component.FindBinary` 支持预装检测。
   - 若包含运行资产目录（如 `subconverter`），解压到 `/etc/vps-auto-deploy/components/<name>` 并配置 Systemd `WorkingDirectory`。
   - 编写对应的单元测试（版本解析与路径探测）。
2. **注册驱动**：在 `cmd/server/main.go` 的 `init()` 中调用 `component.Register(<name>.NewDriver())`。
3. **前端分类与联动适配**：
   - 确认 `web/src/views/Components.vue` 的 `categories` 与分类徽标字典包含该组件的 `category`。
   - 若该组件具备业务配置能力，统一归类挂载进 `web/src/views/Services.vue`（业务配置中心）的对应 Tab 分类下，**严禁在 `App.vue` 顶栏新增平铺导航**。
   - 若该组件赋能现有业务版块（如 `subconverter` 赋能 `订阅与规则`，`hysteria` 赋能 `代理入站`），通过 `/api/components` 检测其 `installed && active` 状态并渐进式解锁对应功能。
4. **零原生弹窗与遮罩防误触检查**：运行 `grep -rnE "\b(alert|confirm|prompt)\(" web/src` 与 `grep -rnE "@click\.self" web/src`，两项输出必须均恒定为空。
5. **构建与验证**：执行 `go test ./...` 与 `bash build.sh`，确保前后端联编生成单一二进制 `bin/vps-panel` 无任何报错。

---

## 五、 多节点集群纳管与二次鉴权规范 (Cluster Management & Step-Up Auth)

### 1. 架构与跨节点透明代理 (Transparent Reverse Proxy)
- **主控与被纳管端单二进制同构**：主控端与被纳管端均运行相同的 `vps-panel` 单一二进制。
- **SSH 极速部署**：主控端利用纯 Go `golang.org/x/crypto/ssh` 连接目标 VPS。同架构机器采用 SSH 管道直接推流复制当前二进制（Zero External Download），并自动注册 Systemd 守护进程。
- **透明反向代理**：前端通过 `X-Target-Node-ID` 请求头（或 SSE `?node_id=`）标识目标节点。主控端 `NodeProxyMiddleware` 自动截获并代理 HTTP/SSE 请求，注入目标节点 Token，实现业务配置（系统概览、软件中心、入站/出站、隧道、日志流等）100% 复用。
- **集群管理端点保护**：所有 `/api/nodes/*` 接口严格在主控端执行，禁止代理，且主控本机节点（ID=1）受持久保护禁止删除。

### 2. 凭据加密与二次鉴权 (AES-256-GCM & Step-Up Secondary Auth)
- **密码加密存储**：被纳管节点的管理员密码禁止在数据库中存留明文，必须通过 `crypto.EncryptAESGCM` 使用从主控 `jwt_secret` 派生的 32 字节密钥进行密文存储。
- **查看凭据二次鉴权**：
  - 用户请求查看节点密码时，前端弹出二次鉴权模态框，请求输入主控当前管理员密码。
  - 后端 `POST /api/nodes/:id/reveal-credentials` 使用 `bcrypt` 比对主控密码；若不符直接返回 `403 Forbidden`。
  - 验证成功后，解密被纳管节点密码并返回，前端开启 **60 秒倒计时自动锁定**，超时后自动重新隐藏凭据。

### 3. 自签 TLS 证书与证书指纹锁定防中间人劫持 (Certificate Pinning & TOFU)
- **零公网证书依赖**：被纳管节点启动时自动生成高强度 ECDSA P-256 证书与私钥，并计算 SHA-256 证书指纹（冒号分隔十六进制串）。
- **SSH 安全信道指纹提取**：部署阶段通过已认证的 SSH 管道直接执行 `-print-fingerprint` 抓取远端证书指纹，持久化入库 `nodes.cert_fingerprint`。
- **TOFU 主机密钥记录**：首次 SSH 连接捕获目标机器 `ssh_host_key`（`ssh.FingerprintSHA256`），防止后续连接遭遇机器重装或 IP 冒充。
- **反向代理强校验**：主控端 `NodeProxyMiddleware` 代理 HTTPS 请求时，设置 `tls.Config{InsecureSkipVerify: true, VerifyConnection: ...}`，严格比对远端证书 DER 数据的 SHA-256 指纹。若指纹不符直接拦截断开并返回 `502 Bad Gateway`。

### 4. 免 SSH 管理凭据自升级体系与安装包导入规范 (Zero-SSH Self-Upgrade & Package Repository)
- **管理凭据为主，SSH 为辅双轨制**：
  - **首选主轨道 (Zero-SSH)**：纳管节点建立后，其日常升级**严禁强制要求用户再次输入 SSH 密码或私钥**。主控端直接通过已绑定的 `APIToken` + Pinned TLS 指纹安全通道，向目标节点 `POST /api/system/self-upgrade` 推送新版二进制。
  - **备选次轨道 (SSH 救援模式)**：仅在目标节点服务彻底离线或 Web API 失联时，作为紧急修复手段由用户手动切换至 SSH 模式介入重装。
- **Linux Inode 原子覆盖与延迟分离重启**：
  - 节点接收新版本二进制时，先执行 ELF 魔数与架构比对（`debug/elf` 确保 x86_64 与 aarch64 不串刷），校验 SHA-256；
  - 写入临时文件后执行原子替换：`mv /tmp/vps-panel.tmp /usr/local/bin/vps-panel`；
  - 节点先向主控端正常返回 HTTP 200，随后启动孤儿协程延迟 800ms 执行 `systemctl restart vps-panel`，杜绝因服务自杀造成 HTTP 连接中断误报；
  - 主控端接收 200 响应后轮询探活，节点恢复在线后自动执行 `SyncMembersToNode` 同步最新成员凭据。
- **本地与集群安装包离线导入中心**：
  - 在「设置」页面提供「面板版本与离线安装包升级」专区，支持拖拽上传单个 `vps-panel` 二进制或 `.tar.gz`/`.zip` 发布包；
  - 仓库自动解析 ELF 架构并做持久化缓存；
  - 提供 `[ ⚡ 升级本机节点 ]`（带全屏重连探活浮层）与 `[ 🚀 分发升级纳管集群节点 ]`（支持单个免密升级与全量勾选并发批量升级）。

---

## 六、 网络安全防护、特权审计与凭据轮换规范 (Security Hardening, Audit & Session Rotation)

### 1. 会话轮换与全局失效机制 (Session Rotation & Invalidation)
- **改密与安全路径变更即轮换**：
  - 管理员修改登录密码或安全路径时，后端必须原子更新 `auth_min_issued_at`（置为当前 Unix 时间戳），立即在内存与数据库层面废止所有早于该时间戳签发的历史 JWT Token。
  - 为当前合法改密客户端重新签发轮换后的全新 Token，杜绝并发越权与会话劫持残留。
- **一键销毁所有活跃会话**：
  - 提供 `POST /api/auth/revoke-all-sessions` 接口，支持管理员在遭遇异常或设备丢失时一键注销全量客户端。

### 2. 特权二次鉴权与防爆破限流 (Rate Limiting on Step-Up Endpoints)
- **暴力破解防护**：
  - 所有涉及特权提权或查看凭据的端点（`POST /nodes/:id/reveal-credentials`、`POST /nodes/:id/terminal-ticket`、`POST /nodes/:id/ssh-credentials`），必须强制挂载 `middleware.LoginRateLimitMiddleware()`。
  - 校验管理员密码失败时，必须显式调用 `middleware.RecordFailedLogin(c.ClientIP())`，确保 15 分钟内连续 5 次错误立即封禁来源 IP，杜绝撞库字典攻击。

### 3. CSWSH 跨源 WebSocket 劫持防御与同源保护
- **WebSocket CheckOrigin 强校验**：
  - 严禁在 `upgrader.CheckOrigin` 中直接无脑返回 `true`。
  - 必须对客户端请求头 `Origin` 进行域名解析，校验 `strings.EqualFold(u.Host, r.Host)`。当来源跨源且非同源主机名时坚决拒绝升级，彻底杜绝恶意第三方网站通过内网扫描利用浏览器凭据静默拉起交互式 Web 控制台。

### 4. 终端票据脱敏与临时凭据内存绑定 (Zero-Credential in Query)
- **URL Query 脱敏铁律**：
  - 严禁在 WebSocket URL Query 或任何 GET 请求中拼接明文密码或私钥（如 `temp_pass`、`temp_key`）。
  - 用户未持久化 SSH 密码连接控制台时，临时凭据必须在 `POST /nodes/:id/terminal-ticket` 通过加密请求体传入，由后端绑定至带 30 秒有效期的内存 `TerminalTicketInfo` 结构体中。
  - WebSocket 仅携带单次安全随机 Ticket（`?ticket=...`），握手升级时由后端原子消费（防重放）并在内存中提取凭据。

### 5. 终端运维特权审计日志 (Terminal Audit Logging)
- **全生命周期可追溯**：
  - 针对宿主机 Local PTY 与集群节点 Remote SSH 终端会话，后端必须全程追踪并结构化输出审计日志：
    - `CONNECT` / `CONNECT_ATTEMPT`：记录发起时间、操作用户、来源 IP、目标节点、目标地址与登录身份；
    - `CONNECTED`：记录终端握手成功与 Shell 就绪；
    - `CONNECT_FAILED`：记录连接或认证失败原因；
    - `DISCONNECT`：记录终端断开及会话存续总时长。
  - 审计日志双轨写入：同步输出到标准系统日志（供 `journalctl` 收集）并追加持久化至 `/etc/vps-auto-deploy/audit.log`（模式 `0600`，仅管理员可读）。

### 6. SSE 实时日志流短效单次票据化 (Zero-JWT in URL Query & Stream Ticket)
- **禁止在常规 REST 接口透传 Query Token**：
  - `JWTAuthMiddleware` 严禁在常规 REST API 接口接收 URL query `?token=`，防止管理凭据落盘留存于反向代理日志、CDN 日志及浏览器历史中。
  - SSE 实时日志流连接（`system/logs`、`deploy-logs`、`install-logs`）统一通过 `POST /system/stream-ticket` 签发 30 秒有效期的单次随机票据（`?ticket=...`），握手升级时原子消费，彻底杜绝长期 JWT 泄露。

### 7. 软件供应链安全与 SHA-256 Checksums 强校验 (Supply-Chain Hardening)
- **拒绝未验签第三方资产**：
  - 受管软件（`sing-box`、`cloudflared`、`subconverter`）通过 GitHub Release 或镜像下载时，必须抓取官方 Release Assets 中的校验和清单（`*checksums*.txt`、`*.sha256`）。
  - 下载文件落地后，驱动在赋予执行权限前必须调用 `component.VerifyFileSHA256` 强校验；若指纹不匹配立即物理销毁临时文件并抛错拦截，彻底防范公共代理镜像投毒与中间人篡改。
  - 解包 tar/zip 归档时，强制清洗权限位掩码（`& 0755` 且剥离 `setuid/setgid/sticky` 特殊权限位），杜绝恶意解包提权。

### 8. 反向代理真实 IP 判定与本地回环防自封 (Reverse Proxy & Loopback Protection)
- **智能客户端 IP 提取**：
  - 限流器统一调用 `middleware.GetRealClientIP(c)`。
  - 当检测到连接来自本机回环反代（`127.0.0.1`、`::1`）时，自动提取 `CF-Connecting-IP`、`X-Real-IP` 或 `X-Forwarded-For` 中的真实发起方 IP，将爆破攻击精准封禁在来源端。
  - 对直连回环请求实施白名单豁免保护，防止前置反向代理或本机直连触发误判，造成管理员自我拒绝服务 (DoS)。

### 9. 进程命令行参数脱敏与 Stdin 管道 (Zero-Credential in CLI Flags)
- **防范 `/proc/<pid>/cmdline` 窥探**：
  - 严禁将明文密码直接作为命令行 Flag（如 `-reset-password`）传递，避免同一宿主机低权限租户通过 `ps aux` 截获凭据。
  - 统一支持通过 Stdin 管道安全传输凭据 JSON（`vps-panel -stdin-credentials`）。

### 10. SSRF 与 DNS 重绑定防御 (Anti-DNS-Rebinding Safe HTTP Transport)
- **连接建立时原子校验**：
  - 网络拉取外部规则或优选 IP 时，统一采用自定义 `SafeHTTPTransport`。
  - 在 `DialContext` 执行底层 TCP 连接的瞬间原子解析并比对 IP，拦截所有私网、回环与云厂商元数据（`169.254.169.254`）地址，彻底消除 TOCTOU 时序差导致的 DNS 重绑定绕过。
  - 集群纳管端未启用自签 TLS 证书时，反代传输层强制挂载 `SafeHTTPTransport`，阻断利用被纳管节点发起的内网与云元数据横向渗透。

### 11. 响应头安全防护与标头注入阻断 (Header Security & Anti-CRLF)
- **HTTP 响应头注入防护**：
  - 订阅分发下载（`Content-Disposition`）必须使用安全清洗函数 `buildAttachmentHeader`，过滤 `\r`、`\n`、`"` 与控制字符，防范 HTTP Response Splitting。
  - 严格遵循 RFC 6266 / RFC 5987 标准（`filename="..."; filename*=UTF-8''...`），保障中文字符与跨平台客户端兼容。
  - 订阅 Token 截取必须进行切片边界保护，杜绝短 Token 导致运行时 panic。
- **内容安全策略 (Content-Security-Policy)**：
  - 全局安全响应头中间件注入严苛 CSP 策略，显式限定 `default-src 'self'`、`connect-src 'self' ws: wss:` 与 `frame-ancestors 'none'`，防御 XSS 与未授权点击劫持。
- **常数时间路径比对**：
  - 安全入口 `SecretPathMiddleware` 使用 `subtle.ConstantTimeCompare` 比对，杜绝微秒级时序侧信道推测。

### 12. Subconverter 代理鉴权与动态端口闭环
- **感知动态配置与安全令牌**：
  - `ProxySubconverterConvert` 自动动态读取生效端口与 `api_access_token`，在开启访问令牌的高安全模式下自动注入 `&token=...`，确保受管服务在端口变更与凭据保护下的功能闭环。





