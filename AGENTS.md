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
    Name             string `json:"name"`              // 唯一标识，如 "sing-box", "xray", "cloudflared", "subconverter"
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
- **目录资产依赖型**（如 `subconverter`、`xray` 含 `geoip.dat/geosite.dat`）：
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

### 1. 两大核心版块划分 (Dual-Domain Information Architecture)

界面顶层导航（`web/src/App.vue`）严格划分为两大业务域与一个管理域，**严禁将“安装/启停”与“协议/节点配置”混杂在同一杂乱视图中**：

| 版块分组 | 导航标识 | 核心职责 | 包含路由视图 |
| :--- | :--- | :--- | :--- |
| **版块一：基础软件** | `基础软件` | 主机监控、底层驱动/扩展软件的安装、升级、卸载、启停与日志流查看 | `📊 系统概览` (`/`)<br>`📦 软件中心` (`/components`) |
| **版块二：业务配置** | `业务配置` | 代理协议节点、Argo 隧道映射、成员权限与订阅转换分发 | `🌐 入站节点` (`/inbounds`)<br>`🚇 Argo 隧道` (`/tunnels`)<br>`👥 成员与订阅` (`/subscriptions`) |
| **系统管理** | `设置` | 面板安全路径、公网 IP 覆盖、管理员密码修改 | `⚙️ 设置` (`/settings`) |

### 2. 软件中心 (`web/src/views/Components.vue`) 扩展约束
- **动态分类过滤**：顶部分类 Tab 应支持按 `category` 过滤（`全部服务`、`代理核心 (proxy)`、`隧道穿透 (tunnel)`、`订阅与规则 (converter)` 等），禁止使用非此即彼的二元三目运算符（如 `c.category === 'proxy' ? ... : ...`），必须使用字典映射表 `CATEGORY_MAP[c.category]`。
- **跨版块联动跳转**：
  - 已安装的组件卡片上若存在 `c.config_route`，应提供快捷按钮 `[ 🛠️ 去配置 ]` 跳转到对应业务配置页。
  - 业务配置页检测到依赖组件未安装时，顶部渲染琥珀色渐进式引导卡片，点击 `[ 🚀 前往「软件中心」一键安装 ]` 携带参数跳转 `/components?install=<name>`，软件中心自动高亮该组件并弹出安装确认。
- **辅助增强型组件联动（如 `subconverter`）**：
  - 当 `subconverter` 未安装时，`成员与订阅` (`/subscriptions`) 使用内置轻量 Go 生成器提供基础 Clash/Sing-box/Base64 订阅。
  - 当检测到 `subconverter` 已安装且运行中时，`成员与订阅` 页面自动解锁/激活「高级远程规则模板转换 (Subconverter 引擎)」选项（支持 ACL4SSR 等远程规则配置与多协议混合转换）。

### 3. 全局非阻塞反馈调用规范
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
   - 若该组件赋能现有业务版块（如 `subconverter` 赋能 `/subscriptions`，`xray` 赋能 `/inbounds`），在对应业务页面通过 `/api/components` 检测其 `installed && active` 状态并渐进式解锁对应功能。
4. **零原生弹窗检查**：运行 `grep -rnE "\b(alert|confirm|prompt)\(" web/src` 确保输出为空。
5. **构建与验证**：执行 `go test ./...` 与 `bash build.sh`，确保前后端联编生成单一二进制 `bin/vps-panel` 无任何报错。
