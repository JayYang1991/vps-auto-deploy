<template>
  <div style="max-width: 820px;">
    <div style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1.5rem;">
      <span style="font-size: 1.5rem;">⚙️</span>
      <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">面板设置与安全</h1>
    </div>

    <!-- 1. 版本与离线安装包升级专区 -->
    <div class="card" style="margin-bottom: 1.5rem; border: 1px solid rgba(99, 102, 241, 0.3); background: linear-gradient(180deg, rgba(30, 41, 59, 0.7) 0%, rgba(15, 23, 42, 0.8) 100%);">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.25rem;">📦</span>
          <h3 style="font-size: 1.1rem; font-weight: 600; color: #818cf8; margin: 0;">面板版本与安装包升级</h3>
        </div>
        <div style="display: flex; gap: 0.5rem; align-items: center;">
          <span class="badge" style="background: rgba(99, 102, 241, 0.2); color: #a5b4fc; border: 1px solid rgba(99, 102, 241, 0.4); padding: 0.25rem 0.6rem; border-radius: 9999px; font-size: 0.8rem; font-weight: 600;">
            当前版本: {{ sysInfo.version || 'v1.0.1' }}
          </span>
          <span class="badge" style="background: rgba(56, 189, 248, 0.15); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.3); padding: 0.25rem 0.6rem; border-radius: 9999px; font-size: 0.8rem;">
            {{ sysInfo.os }}/{{ sysInfo.arch }}
          </span>
        </div>
      </div>

      <!-- 当前运行程序指纹 -->
      <div style="display: flex; align-items: center; justify-content: space-between; background: rgba(15, 23, 42, 0.6); padding: 0.6rem 0.85rem; border-radius: 0.375rem; border: 1px solid #334155; margin-bottom: 1rem; font-size: 0.8125rem;">
        <div style="display: flex; align-items: center; gap: 0.5rem; overflow: hidden;">
          <span style="color: #94a3b8; flex-shrink: 0;">运行程序指纹 (SHA-256):</span>
          <code style="color: #cbd5e1; font-family: monospace; text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">
            {{ sysInfo.sha256 || '计算中...' }}
          </code>
        </div>
        <button
          v-if="sysInfo.sha256"
          type="button"
          class="btn-text-copy"
          @click="copyToClipboard(sysInfo.sha256, '运行程序 SHA-256 指纹')"
          style="flex-shrink: 0; color: #818cf8; background: transparent; border: none; cursor: pointer; font-size: 0.75rem; padding: 0.2rem 0.5rem;"
        >
          📋 复制
        </button>
      </div>

      <!-- 拖拽/点击上传区域 -->
      <div
        class="upload-dropzone"
        :class="{ 'dropzone-active': isDragging }"
        @dragover.prevent="isDragging = true"
        @dragleave.prevent="isDragging = false"
        @drop.prevent="handleDrop"
        @click="triggerFileInput"
        style="border: 2px dashed #475569; border-radius: 0.5rem; padding: 1.5rem; text-align: center; cursor: pointer; background: rgba(15, 23, 42, 0.4); transition: all 0.2s;"
      >
        <input
          ref="fileInputRef"
          type="file"
          style="display: none;"
          @change="handleFileSelected"
        />
        <div v-if="!uploading">
          <div style="font-size: 2rem; margin-bottom: 0.5rem;">📥</div>
          <div style="font-size: 0.95rem; font-weight: 600; color: #f8fafc; margin-bottom: 0.25rem;">
            点击或拖拽上传新版安装包
          </div>
          <div style="font-size: 0.8rem; color: #94a3b8;">
            支持 <code>vps-panel</code> 单二进制文件，或包含该程序的 <code>.tar.gz</code> / <code>.zip</code> 发布包
          </div>
        </div>
        <div v-else>
          <div style="font-size: 1.5rem; margin-bottom: 0.5rem;">⏳</div>
          <div style="font-size: 0.9rem; font-weight: 600; color: #38bdf8; margin-bottom: 0.5rem;">
            正在上传并校验安装包 ELF 架构与完整性...
          </div>
          <div style="width: 100%; max-width: 300px; height: 6px; background: #334155; border-radius: 3px; margin: 0 auto; overflow: hidden;">
            <div style="height: 100%; background: #6366f1; width: 100%; animation: indeterminate 1.5s infinite linear;"></div>
          </div>
        </div>
      </div>

      <!-- 已导入安装包列表 -->
      <div v-if="packages.length > 0" style="margin-top: 1.25rem;">
        <h4 style="font-size: 0.9rem; font-weight: 600; color: #cbd5e1; margin-bottom: 0.75rem; display: flex; align-items: center; justify-content: space-between;">
          <span>📦 已就绪的安装包仓库 ({{ packages.length }})</span>
          <span style="font-size: 0.75rem; color: #94a3b8; font-weight: normal;">点击下方操作升级本机或分发至集群</span>
        </h4>

        <div style="display: flex; flex-direction: column; gap: 0.75rem;">
          <div
            v-for="pkg in packages"
            :key="pkg.package_id"
            style="background: rgba(15, 23, 42, 0.7); border: 1px solid #334155; border-radius: 0.5rem; padding: 0.85rem; display: flex; flex-direction: column; gap: 0.6rem;"
          >
            <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 0.5rem;">
              <div>
                <div style="display: flex; align-items: center; gap: 0.5rem;">
                  <span style="font-weight: 600; color: #f8fafc; font-size: 0.9rem;">{{ pkg.file_name }}</span>
                  <span
                    :style="{
                      background: pkg.match_current_arch ? 'rgba(16, 185, 129, 0.2)' : 'rgba(245, 158, 11, 0.2)',
                      color: pkg.match_current_arch ? '#34d399' : '#fbbf24',
                      border: `1px solid ${pkg.match_current_arch ? 'rgba(16, 185, 129, 0.4)' : 'rgba(245, 158, 11, 0.4)'}`,
                      padding: '0.15rem 0.45rem',
                      borderRadius: '0.25rem',
                      fontSize: '0.75rem',
                      fontWeight: '600'
                    }"
                  >
                    {{ pkg.arch }}
                  </span>
                  <span style="font-size: 0.75rem; color: #818cf8; background: rgba(99, 102, 241, 0.15); padding: 0.15rem 0.45rem; border-radius: 0.25rem;">
                    {{ pkg.version }}
                  </span>
                </div>
                <div style="font-size: 0.75rem; color: #64748b; margin-top: 0.25rem; font-family: monospace;">
                  大小: {{ (pkg.size / 1024 / 1024).toFixed(2) }} MB | SHA-256: {{ pkg.sha256.substring(0, 16) }}...
                </div>
              </div>

              <!-- 操作按钮 -->
              <div style="display: flex; gap: 0.5rem; align-items: center;">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="!pkg.match_current_arch || isLocalUpgrading"
                  @click="triggerLocalUpgrade(pkg)"
                  style="font-size: 0.8125rem; padding: 0.35rem 0.75rem;"
                  :title="pkg.match_current_arch ? '将当前主机面板升级到此版本' : '此安装包架构与本机不匹配'"
                >
                  ⚡ 升级本机节点
                </button>
                <button
                  type="button"
                  class="btn"
                  @click="toggleClusterDrawer(pkg)"
                  style="font-size: 0.8125rem; padding: 0.35rem 0.75rem; background: rgba(56, 189, 248, 0.2); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.4);"
                >
                  🚀 分发至集群节点 ({{ remoteNodes.length }})
                </button>
              </div>
            </div>

            <!-- 纳管节点分发选择面板 (点击展开) -->
            <div
              v-if="activeDistPkgId === pkg.package_id"
              style="margin-top: 0.5rem; border-top: 1px dashed #334155; padding-top: 0.75rem; background: rgba(15, 23, 42, 0.9); border-radius: 0.375rem; padding: 0.75rem;"
            >
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
                <span style="font-size: 0.8125rem; font-weight: 600; color: #38bdf8;">
                  🚀 选择要免密推送升级的纳管节点：
                </span>
                <div style="display: flex; gap: 0.5rem;">
                  <button
                    type="button"
                    class="btn-link"
                    @click="selectAllNodes"
                    style="font-size: 0.75rem; color: #818cf8; background: none; border: none; cursor: pointer;"
                  >
                    全选
                  </button>
                  <button
                    type="button"
                    class="btn-link"
                    @click="deselectAllNodes"
                    style="font-size: 0.75rem; color: #94a3b8; background: none; border: none; cursor: pointer;"
                  >
                    取消全选
                  </button>
                </div>
              </div>

              <div v-if="remoteNodes.length === 0" style="font-size: 0.8rem; color: #64748b; padding: 0.5rem 0;">
                暂无纳管节点，可前往主控导航「纳管服务器」纳管更多 VPS。
              </div>

              <div v-else style="display: flex; flex-direction: column; gap: 0.4rem; max-height: 200px; overflow-y: auto;">
                <label
                  v-for="node in remoteNodes"
                  :key="node.id"
                  style="display: flex; align-items: center; justify-content: space-between; padding: 0.4rem 0.6rem; background: rgba(30, 41, 59, 0.5); border-radius: 0.25rem; cursor: pointer;"
                >
                  <div style="display: flex; align-items: center; gap: 0.5rem;">
                    <input
                      type="checkbox"
                      :value="node.id"
                      v-model="selectedNodeIds"
                      style="accent-color: #6366f1;"
                    />
                    <span style="font-size: 0.8125rem; color: #f8fafc; font-weight: 500;">{{ node.name }}</span>
                    <span style="font-size: 0.75rem; color: #64748b;">({{ node.host }})</span>
                    <span style="font-size: 0.7rem; color: #38bdf8; background: rgba(56, 189, 248, 0.1); padding: 0.1rem 0.3rem; border-radius: 0.2rem;">
                      {{ node.os_info || 'Linux (amd64)' }}
                    </span>
                  </div>

                  <div style="display: flex; align-items: center; gap: 0.5rem;">
                    <span
                      :style="{
                        fontSize: '0.75rem',
                        color: node.status === 'online' ? '#10b981' : '#f43f5e',
                        fontWeight: '500'
                      }"
                    >
                      {{ node.status === 'online' ? '● 在线' : '● 离线' }}
                    </span>
                    <span v-if="upgradingNodes[node.id]" style="font-size: 0.75rem; color: #818cf8;">
                      ⏳ 升级中...
                    </span>
                    <button
                      type="button"
                      class="btn"
                      @click.stop="upgradeSingleRemoteNode(node, pkg)"
                      :disabled="upgradingNodes[node.id]"
                      style="font-size: 0.75rem; padding: 0.15rem 0.45rem; background: #4f46e5; color: #ffffff;"
                    >
                      🚀 免密升级
                    </button>
                  </div>
                </label>
              </div>

              <!-- 批量操作 -->
              <div v-if="remoteNodes.length > 0" style="margin-top: 0.75rem; display: flex; justify-content: flex-end; gap: 0.5rem;">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="selectedNodeIds.length === 0 || isBatchUpgrading"
                  @click="batchUpgradeRemoteNodes(pkg)"
                  style="font-size: 0.8125rem; padding: 0.35rem 0.85rem;"
                >
                  {{ isBatchUpgrading ? '⏳ 批量升级中...' : `⚡ 一键免密批量升级已选 (${selectedNodeIds.length})` }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. 系统基础参数 -->
    <div class="card" style="margin-bottom: 1.5rem;">
      <h3 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem; color: #38bdf8;">系统基础配置</h3>
      <form @submit.prevent="saveSettings" style="display: flex; flex-direction: column; gap: 1rem;">
        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            面板安全路径 (Secret Base Path)
          </label>
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span style="color: #64748b;">/</span>
            <input v-model="settings.secret_path" class="input" required style="font-family: monospace;" />
            <span style="color: #64748b;">/</span>
          </div>
          <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">
            用于隐藏管理面板入口（如 /a8f9d2/），修改后请务必记牢新 URL，未授权请求将直接返回 404。
          </p>
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            公网 IP 强制覆盖 (可选)
          </label>
          <input v-model="settings.public_ip" class="input" placeholder="留空则自动检测主机公网 IP" />
          <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">
            若 VPS 处于 NAT 之后或使用了 CDN/中转，可在此处指定生成订阅和节点链接时填入的 IP 或域名。
          </p>
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            会话空闲自动登出 (Idle Timeout)
          </label>
          <select v-model.number="settings.session_idle_timeout" class="input" style="max-width: 320px;">
            <option :value="15">15 分钟无操作自动退出</option>
            <option :value="30">30 分钟无操作自动退出 (推荐)</option>
            <option :value="60">1 小时无操作自动退出</option>
            <option :value="240">4 小时无操作自动退出</option>
            <option :value="0">关闭自动登出 (仅依赖长周期 Token，不推荐)</option>
          </select>
          <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">
            当在指定时间内未检测到键盘、鼠标或 API 请求交互时，系统将自动安全注销并返回登录页，防范工位脱离风险。
          </p>
        </div>

        <div style="display: flex; gap: 0.75rem; align-items: center; flex-wrap: wrap; margin-top: 0.5rem;">
          <button type="submit" class="btn btn-primary">保存系统设置</button>
          <button
            type="button"
            class="btn btn-secondary"
            @click="revokeAllSessions"
            style="color: #f87171; border-color: rgba(239, 68, 68, 0.4);"
            title="立即废止所有设备上已签发的有效访问凭据"
          >
            ⚡ 强制注销所有设备会话
          </button>
        </div>
      </form>
    </div>

    <!-- 3. Telegram 审计与安全告警外发 -->
    <div class="card" style="margin-bottom: 1.5rem; border: 1px solid rgba(14, 165, 233, 0.3); background: linear-gradient(180deg, rgba(30, 41, 59, 0.7) 0%, rgba(15, 23, 42, 0.8) 100%);">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.75rem;">
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.35rem;">✈️</span>
          <div>
            <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;">
              <h3 style="font-size: 1.1rem; font-weight: 600; color: #38bdf8; margin: 0;">Telegram 审计与安全告警外发</h3>
              <button
                type="button"
                @click="showTgGuide = !showTgGuide"
                style="background: rgba(56, 189, 248, 0.12); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.3); font-size: 0.75rem; padding: 0.15rem 0.5rem; border-radius: 9999px; cursor: pointer; display: flex; align-items: center; gap: 0.25rem; transition: all 0.2s;"
              >
                <span>{{ showTgGuide ? '收起配置指引 ▲' : '📖 极速配置教程 ▼' }}</span>
              </button>
            </div>
            <p style="font-size: 0.75rem; color: #94a3b8; margin: 0.2rem 0 0 0;">
              将管理员登录、IP 封禁、凭据查阅、终端运维与实例生命周期事件秒级推送至 Telegram
            </p>
          </div>
        </div>
        <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer; user-select: none; background: rgba(15, 23, 42, 0.6); padding: 0.35rem 0.75rem; border-radius: 9999px; border: 1px solid #334155;">
          <input type="checkbox" v-model="tgConfig.enabled" style="accent-color: #0284c7; width: 1rem; height: 1rem; cursor: pointer;" />
          <span style="font-size: 0.85rem; font-weight: 600;" :style="{ color: tgConfig.enabled ? '#38bdf8' : '#94a3b8' }">
            {{ tgConfig.enabled ? '● 告警已启用' : '○ 告警已暂停' }}
          </span>
        </label>
      </div>

      <!-- 极速配置指南看板 (Step-by-step visual onboarding guide) -->
      <div
        v-if="showTgGuide"
        style="background: rgba(15, 23, 42, 0.75); border: 1px solid rgba(56, 189, 248, 0.25); border-radius: 0.5rem; padding: 1rem; margin-bottom: 1.25rem;"
      >
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
          <div style="font-size: 0.875rem; font-weight: 600; color: #f8fafc; display: flex; align-items: center; gap: 0.4rem;">
            <span>💡 3 步完成 Telegram 审计机器人配置</span>
          </div>
          <button
            type="button"
            @click="showTgGuide = false"
            style="background: transparent; border: none; color: #64748b; cursor: pointer; font-size: 0.8rem;"
          >
            ✕ 暂时收起
          </button>
        </div>

        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 0.75rem;">
          <!-- 步骤 1 -->
          <div style="background: rgba(30, 41, 59, 0.6); border: 1px solid #334155; border-radius: 0.375rem; padding: 0.75rem; display: flex; flex-direction: column; gap: 0.4rem;">
            <div style="display: flex; align-items: center; justify-content: space-between;">
              <span style="font-size: 0.8rem; font-weight: 700; color: #38bdf8; background: rgba(56, 189, 248, 0.15); padding: 0.1rem 0.4rem; border-radius: 0.25rem;">
                Step 1: 创建机器人
              </span>
              <a
                href="https://t.me/BotFather"
                target="_blank"
                rel="noreferrer"
                style="font-size: 0.75rem; color: #38bdf8; text-decoration: none; display: flex; align-items: center; gap: 0.2rem;"
              >
                <span>@BotFather</span> ↗
              </a>
            </div>
            <p style="font-size: 0.75rem; color: #cbd5e1; line-height: 1.4; margin: 0;">
              在 Telegram 中打开官方 <b>@BotFather</b>，发送
              <code style="background: rgba(0,0,0,0.3); padding: 0.1rem 0.3rem; border-radius: 0.2rem; color: #f1f5f9; cursor: pointer;" @click="copyToClipboard('/newbot', '指令')" title="点击复制">/newbot</code>
              按照提示设置机器人昵称与用户名（须以 <code>bot</code> 结尾）。
            </p>
            <p style="font-size: 0.72rem; color: #94a3b8; margin: 0;">
              获取形如 <code>123456:ABC-DEF...</code> 的 API Token 并填入下方输入框。
            </p>
          </div>

          <!-- 步骤 2 -->
          <div style="background: rgba(30, 41, 59, 0.6); border: 1px solid #334155; border-radius: 0.375rem; padding: 0.75rem; display: flex; flex-direction: column; gap: 0.4rem;">
            <div style="display: flex; align-items: center; justify-content: space-between;">
              <span style="font-size: 0.8rem; font-weight: 700; color: #818cf8; background: rgba(129, 140, 248, 0.15); padding: 0.1rem 0.4rem; border-radius: 0.25rem;">
                Step 2: 获取 Chat ID
              </span>
              <a
                href="https://t.me/userinfobot"
                target="_blank"
                rel="noreferrer"
                style="font-size: 0.75rem; color: #818cf8; text-decoration: none; display: flex; align-items: center; gap: 0.2rem;"
              >
                <span>@userinfobot</span> ↗
              </a>
            </div>
            <div style="background: rgba(245, 158, 11, 0.1); border-left: 2px solid #f59e0b; padding: 0.25rem 0.4rem; border-radius: 0.2rem; font-size: 0.72rem; color: #fbbf24;">
              ⚠️ <b>关键一步</b>：先打开你刚建好的机器人，点击底部 <b>START</b> 发送首条消息，否则 Telegram 会拦截推送！
            </div>
            <p style="font-size: 0.75rem; color: #cbd5e1; line-height: 1.4; margin: 0;">
              <b>个人通知</b>：私聊 <b>@userinfobot</b> 获取纯数字 <code>Id</code>。<br/>
              <b>群组通知</b>：将机器人拉入群，把群消息转发给 <b>@userinfobot</b> 获取群组 ID（以 <code>-100</code> 开头）。
            </p>
          </div>

          <!-- 步骤 3 -->
          <div style="background: rgba(30, 41, 59, 0.6); border: 1px solid #334155; border-radius: 0.375rem; padding: 0.75rem; display: flex; flex-direction: column; gap: 0.4rem;">
            <div style="display: flex; align-items: center; justify-content: space-between;">
              <span style="font-size: 0.8rem; font-weight: 700; color: #34d399; background: rgba(52, 211, 153, 0.15); padding: 0.1rem 0.4rem; border-radius: 0.25rem;">
                Step 3: 测试并保存
              </span>
              <span style="font-size: 0.75rem; color: #34d399;">即配即通</span>
            </div>
            <p style="font-size: 0.75rem; color: #cbd5e1; line-height: 1.4; margin: 0;">
              填写 Token 与 Chat ID 后，点击右下角 <b>[ 🔔 发送测试告警 ]</b>。<br/>
              确认 Telegram 客户端收到测试推送后，勾选关注的事件类别并点击 <b>[ 💾 保存配置 ]</b>。
            </p>
            <div style="margin-top: auto; padding-top: 0.25rem;">
              <button
                type="button"
                @click="showProxyGuide = !showProxyGuide"
                style="background: transparent; border: none; color: #38bdf8; font-size: 0.72rem; cursor: pointer; padding: 0; text-decoration: underline;"
              >
                {{ showProxyGuide ? '收起境内网络加速指南 ▲' : '🌐 境内 VPS / 网络受限如何解决？▼' }}
              </button>
            </div>
          </div>
        </div>

        <!-- 境内网络加速 / Cloudflare Workers 教程抽屉 (Expandable Proxy Tip) -->
        <div
          v-if="showProxyGuide"
          style="margin-top: 0.75rem; background: rgba(15, 23, 42, 0.9); border: 1px dashed #475569; border-radius: 0.375rem; padding: 0.75rem;"
        >
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.4rem;">
            <div style="font-size: 0.78rem; font-weight: 600; color: #38bdf8;">
              🌐 境内 VPS 免费反代解决方案 (Cloudflare Workers 1 分钟搞定)
            </div>
            <button
              type="button"
              class="btn-text-copy"
              @click="copyToClipboard(CF_WORKER_SNIPPET, 'Cloudflare Worker 代理代码')"
              style="font-size: 0.72rem; color: #818cf8; background: transparent; border: none; cursor: pointer;"
            >
              📋 复制 Worker 代码
            </button>
          </div>
          <p style="font-size: 0.72rem; color: #94a3b8; line-height: 1.4; margin-bottom: 0.4rem;">
            若您的 VPS 位于中国大陆直连 <code>api.telegram.org</code> 超时，可免费在 Cloudflare 创建一个 Worker，粘贴下方代码并部署。将生成的 <code>https://xxx.workers.dev</code> 填入下方的「自定义 Telegram API 地址」即可顺畅推送：
          </p>
          <pre style="margin: 0; padding: 0.5rem; background: rgba(0,0,0,0.4); border-radius: 0.25rem; font-size: 0.72rem; color: #a5f3fc; font-family: monospace; overflow-x: auto;">{{ CF_WORKER_SNIPPET }}</pre>
        </div>
      </div>

      <form @submit.prevent="saveTelegramConfig" style="display: flex; flex-direction: column; gap: 1rem;">
        <!-- Bot Token -->
        <div>
          <label style="display: flex; justify-content: space-between; align-items: center; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            <span>Telegram Bot Token <span style="color: #f43f5e;">*</span></span>
            <span v-if="tgConfig.masked_token" style="font-size: 0.75rem; color: #10b981;">
              已加密绑定: <code>{{ tgConfig.masked_token }}</code>
            </span>
          </label>
          <div style="position: relative; display: flex; align-items: center;">
            <input
              :type="showBotToken ? 'text' : 'password'"
              v-model="tgConfig.bot_token"
              class="input"
              :placeholder="tgConfig.masked_token ? '留空保留已配置 Token，输入新 Token 将覆盖' : '例如: 123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ'"
              style="padding-right: 2.5rem; font-family: monospace;"
            />
            <button
              type="button"
              @click="showBotToken = !showBotToken"
              style="position: absolute; right: 0.5rem; background: transparent; border: none; color: #94a3b8; cursor: pointer; font-size: 1rem; padding: 0.25rem;"
              title="切换明文显示"
            >
              {{ showBotToken ? '🙈' : '👁️' }}
            </button>
          </div>
          <div style="display: flex; gap: 0.75rem; margin-top: 0.35rem; align-items: center; flex-wrap: wrap;">
            <a href="https://t.me/BotFather" target="_blank" rel="noreferrer" style="font-size: 0.75rem; color: #38bdf8; text-decoration: none;">
              🤖 私聊 @BotFather 创建机器人 ↗
            </a>
            <button type="button" @click="copyToClipboard('/newbot', '指令')" style="background: transparent; border: none; color: #94a3b8; font-size: 0.75rem; cursor: pointer; padding: 0;">
              📋 复制 <code>/newbot</code>
            </button>
            <span style="font-size: 0.72rem; color: #64748b;">落盘时使用 AES-256-GCM 强加密存储</span>
          </div>
        </div>

        <!-- Chat ID -->
        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            接收通知的 Chat ID <span style="color: #f43f5e;">*</span>
          </label>
          <input
            v-model="tgConfig.chat_id"
            class="input"
            placeholder="例如个人 ID: 123456789 或群组 ID: -1001234567890"
            style="font-family: monospace;"
          />
          <div style="display: flex; gap: 0.75rem; margin-top: 0.35rem; align-items: center; flex-wrap: wrap;">
            <a href="https://t.me/userinfobot" target="_blank" rel="noreferrer" style="font-size: 0.75rem; color: #818cf8; text-decoration: none;">
              🔍 私聊 @userinfobot 获取个人 ID ↗
            </a>
            <span style="font-size: 0.75rem; color: #fbbf24;">
              ⚡ 提示: 必须先在 Telegram 中打开新建的机器人并点击 START 发送任意消息！
            </span>
          </div>
        </div>

        <!-- 自定义 API URL (反代/中转) -->
        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            自定义 Telegram API 地址 (可选，默认官方地址)
          </label>
          <input
            v-model="tgConfig.api_url"
            class="input"
            placeholder="https://api.telegram.org"
            style="font-family: monospace;"
          />
          <div style="display: flex; gap: 0.75rem; margin-top: 0.35rem; align-items: center;">
            <button type="button" @click="showProxyGuide = !showProxyGuide" style="background: transparent; border: none; color: #38bdf8; font-size: 0.75rem; cursor: pointer; padding: 0; text-decoration: underline;">
              {{ showProxyGuide ? '收起反代配置代码 ▲' : '📖 查看 Cloudflare Workers 1 分钟反代代码模板 ▼' }}
            </button>
            <span style="font-size: 0.72rem; color: #64748b;">内置 SSRF 私网探测防护</span>
          </div>
        </div>

        <!-- 静默推送 -->
        <div>
          <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer; user-select: none;">
            <input type="checkbox" v-model="tgConfig.silent" style="accent-color: #0284c7; width: 0.95rem; height: 0.95rem; cursor: pointer;" />
            <span style="font-size: 0.85rem; color: #cbd5e1;">🔕 启用静默推送 (消息送达时不发出声音或振动提醒)</span>
          </label>
        </div>

        <!-- 事件类型勾选 -->
        <div>
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; flex-wrap: wrap; gap: 0.5rem;">
            <label style="font-size: 0.85rem; color: #cbd5e1; font-weight: 600;">
              订阅事件类型 ({{ tgConfig.events.length }}/{{ EVENT_DEFINITIONS.length }})
            </label>
            <div style="display: flex; gap: 0.5rem;">
              <button
                type="button"
                @click="selectAllEvents"
                style="background: transparent; border: 1px solid #475569; color: #94a3b8; font-size: 0.75rem; padding: 0.2rem 0.5rem; border-radius: 0.25rem; cursor: pointer;"
              >
                全选
              </button>
              <button
                type="button"
                @click="selectCriticalEvents"
                style="background: transparent; border: 1px solid #475569; color: #38bdf8; font-size: 0.75rem; padding: 0.2rem 0.5rem; border-radius: 0.25rem; cursor: pointer;"
              >
                仅关键安全
              </button>
            </div>
          </div>

          <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 0.6rem; background: rgba(15, 23, 42, 0.5); padding: 0.75rem; border-radius: 0.375rem; border: 1px solid #334155;">
            <label
              v-for="evt in EVENT_DEFINITIONS"
              :key="evt.key"
              style="display: flex; align-items: flex-start; gap: 0.45rem; cursor: pointer; user-select: none; padding: 0.35rem; border-radius: 0.25rem; background: rgba(30, 41, 59, 0.4);"
            >
              <input
                type="checkbox"
                :value="evt.key"
                v-model="tgConfig.events"
                style="accent-color: #0284c7; margin-top: 0.2rem; cursor: pointer;"
              />
              <div style="flex: 1; min-width: 0;">
                <div style="display: flex; align-items: center; gap: 0.35rem;">
                  <span style="font-size: 0.8rem; font-weight: 500; color: #f1f5f9;">{{ evt.label }}</span>
                  <span
                    :style="{
                      fontSize: '0.65rem',
                      padding: '0.05rem 0.3rem',
                      borderRadius: '0.2rem',
                      background: evt.level === 'critical' ? 'rgba(239, 68, 68, 0.2)' : evt.level === 'warn' ? 'rgba(245, 158, 11, 0.2)' : 'rgba(56, 189, 248, 0.15)',
                      color: evt.level === 'critical' ? '#f87171' : evt.level === 'warn' ? '#fbbf24' : '#38bdf8',
                    }"
                  >
                    {{ evt.level === 'critical' ? '紧急' : evt.level === 'warn' ? '警告' : '通知' }}
                  </span>
                </div>
                <div style="font-size: 0.7rem; color: #64748b; margin-top: 0.1rem;">{{ evt.desc }}</div>
              </div>
            </label>
          </div>
        </div>

        <!-- 连通性测试诊断反馈条 (Diagnostic Result Banner) -->
        <div
          v-if="testResult"
          :style="{
            background: testResult.success ? 'rgba(16, 185, 129, 0.15)' : 'rgba(239, 68, 68, 0.15)',
            border: `1px solid ${testResult.success ? '#10b981' : '#ef4444'}`,
            padding: '0.65rem 0.85rem',
            borderRadius: '0.375rem',
            fontSize: '0.8125rem',
            color: testResult.success ? '#34d399' : '#f87171'
          }"
        >
          <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 0.5rem;">
            <div>
              <div style="font-weight: 600; margin-bottom: 0.2rem;">
                {{ testResult.success ? '✅ 测试告警推送成功' : '❌ 连通性测试失败' }}
              </div>
              <div style="color: #cbd5e1; font-size: 0.78rem;">{{ testResult.message }}</div>
              <div
                v-if="!testResult.success && (testResult.message.includes('blocked by the user') || testResult.message.includes('chat not found'))"
                style="margin-top: 0.35rem; color: #fbbf24; font-size: 0.75rem; line-height: 1.4;"
              >
                👉 <b>排查提示</b>：Telegram 限制机器人向未主动互动的用户推流。请先在 Telegram 中打开你的机器人点击底部的 <b>[START]</b> 发送第一条消息后再试！
              </div>
            </div>
            <button
              type="button"
              @click="testResult = null"
              style="background: transparent; border: none; color: inherit; cursor: pointer; font-size: 1rem; padding: 0.1rem 0.3rem;"
            >
              ✕
            </button>
          </div>
        </div>

        <!-- 按钮操作栏 -->
        <div style="display: flex; gap: 0.75rem; align-items: center; margin-top: 0.5rem; flex-wrap: wrap;">
          <button type="submit" class="btn btn-primary" :disabled="savingTelegram">
            {{ savingTelegram ? '⏳ 保存中...' : '💾 保存 Telegram 配置' }}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            @click="testTelegram"
            :disabled="testingTelegram"
            style="color: #38bdf8; border-color: rgba(56, 189, 248, 0.4);"
          >
            {{ testingTelegram ? '⏳ 正在推送测试消息...' : '🔔 发送测试告警' }}
          </button>
        </div>
      </form>
    </div>

    <!-- 4. 修改管理员密码 -->
    <div class="card">
      <h3 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem; color: #fbbf24;">修改管理员密码</h3>
      <form @submit.prevent="updatePassword" style="display: flex; flex-direction: column; gap: 1rem;">
        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">旧密码</label>
          <input v-model="passForm.old_password" type="password" class="input" required />
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">新密码</label>
          <input v-model="passForm.new_password" type="password" class="input" required />
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">确认新密码</label>
          <input v-model="passForm.confirm_password" type="password" class="input" required />
        </div>

        <button type="submit" class="btn btn-primary" style="align-self: flex-start;">更新密码</button>
      </form>
    </div>

    <!-- 4. 本地升级全屏重连探活浮层 (Reconnection Overlay) -->
    <div
      v-if="reconnecting"
      style="position: fixed; inset: 0; background: rgba(15, 23, 42, 0.85); backdrop-filter: blur(8px); z-index: 9999; display: flex; align-items: center; justify-content: center; padding: 1.5rem;"
    >
      <div style="background: rgba(30, 41, 59, 0.95); border: 1px solid #475569; border-radius: 0.75rem; padding: 2rem; max-width: 440px; width: 100%; text-align: center; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);">
        <div style="font-size: 3rem; margin-bottom: 1rem; animation: spin 2s linear infinite; display: inline-block;">
          ⚡
        </div>
        <h3 style="font-size: 1.2rem; font-weight: 700; color: #f8fafc; margin-bottom: 0.5rem;">
          服务平滑重启中...
        </h3>
        <p style="font-size: 0.875rem; color: #94a3b8; line-height: 1.5; margin-bottom: 1.25rem;">
          新版二进制程序已原子落盘生效，Systemd 守护进程正在重新载入并拉起服务，正在持续探活等待重连...
        </p>
        <div style="display: flex; align-items: center; justify-content: center; gap: 0.5rem; font-size: 0.8125rem; color: #38bdf8;">
          <span class="pulse-indicator pulse-active"></span>
          <span>已发起探活 {{ reconnectAttempts }} 次...</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'
import { clearToken, updateSessionIdleTimeout } from '../utils/auth'

const { toast } = useToast()
const { showConfirm } = useConfirm()

// 系统环境与版本概览
const sysInfo = reactive({
  version: '',
  os: 'linux',
  arch: 'amd64',
  sha256: '',
})

// 安装包仓库与上传
const packages = ref<any[]>([])
const uploading = ref(false)
const isDragging = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)
const isLocalUpgrading = ref(false)
const reconnecting = ref(false)
const reconnectAttempts = ref(0)

// 纳管节点与集群分发
const remoteNodes = ref<any[]>([])
const activeDistPkgId = ref('')
const selectedNodeIds = ref<number[]>([])
const isBatchUpgrading = ref(false)
const upgradingNodes = reactive<Record<number, boolean>>({})

// 系统基础配置与密码表单
const settings = reactive({
  secret_path: '',
  public_ip: '',
  session_idle_timeout: 30,
})

// Telegram 审计告警配置
const tgConfig = reactive({
  enabled: false,
  bot_token: '',
  masked_token: '',
  chat_id: '',
  api_url: 'https://api.telegram.org',
  events: [] as string[],
  silent: false,
})
const showBotToken = ref(false)
const savingTelegram = ref(false)
const testingTelegram = ref(false)
const showTgGuide = ref(false)
const showProxyGuide = ref(false)
const testResult = ref<{ success: boolean; message: string } | null>(null)

const CF_WORKER_SNIPPET = `export default {
  async fetch(request) {
    const url = new URL(request.url);
    url.hostname = 'api.telegram.org';
    return fetch(new Request(url, request));
  }
}`

const EVENT_DEFINITIONS = [
  { key: 'IP_LOCKED', label: 'IP 连续登录失败被锁定', level: 'critical', desc: '防爆破触发 15 分钟封禁' },
  { key: 'LOGIN_FAILED', label: '管理员登录失败', level: 'warn', desc: '后台密码错误尝试 (内置 30 秒流控)' },
  { key: 'LOGIN_SUCCESS', label: '管理员登录成功', level: 'info', desc: '合法会话登录签发' },
  { key: 'CREDS_REVEALED', label: '查阅节点机密凭据', level: 'warn', desc: '通过二次鉴权解密查看节点密码' },
  { key: 'PASSWORD_CHANGED', label: '管理员密码修改', level: 'critical', desc: '主控管理员密码更新并轮换会话' },
  { key: 'SESSION_REVOKED', label: '全局会话强制注销', level: 'critical', desc: '一键作废所有设备历史 Token' },
  { key: 'TERMINAL_SESSION', label: '特权 Web 终端会话', level: 'warn', desc: 'SSH / PTY 终端连接或连接失败' },
  { key: 'NODE_DEPLOY', label: '节点部署加入集群', level: 'info', desc: '新 VPS 自动纳管完成' },
  { key: 'NODE_DELETED', label: '节点移出集群', level: 'warn', desc: '纳管节点被移出' },
  { key: 'CLOUD_DESTROYED', label: '云端实例彻底销毁', level: 'critical', desc: '物理销毁云主机并停止计费' },
  { key: 'CONFIG_RELOAD', label: 'Sing-box 核心重载', level: 'info', desc: '网络协议核心配置热重载' },
]

const passForm = reactive({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

async function loadSystemOverview() {
  try {
    const res = await api.get('/system/overview')
    sysInfo.version = res.data.version || 'v1.0.1'
    sysInfo.os = res.data.os || 'linux'
    sysInfo.arch = res.data.arch || 'amd64'
    sysInfo.sha256 = res.data.sha256 || ''
  } catch {
    // 忽略错误
  }
}

async function loadPackages() {
  try {
    const res = await api.get('/system/upgrade/packages')
    packages.value = res.data || []
  } catch {
    packages.value = []
  }
}

async function loadRemoteNodes() {
  try {
    const res = await api.get('/nodes')
    remoteNodes.value = (res.data || []).filter((n: any) => !n.is_master)
  } catch {
    remoteNodes.value = []
  }
}

async function loadSettings() {
  try {
    const res = await api.get('/settings')
    settings.secret_path = res.data.secret_path
    settings.public_ip = res.data.public_ip
    if (typeof res.data.session_idle_timeout === 'number') {
      settings.session_idle_timeout = res.data.session_idle_timeout
      updateSessionIdleTimeout(res.data.session_idle_timeout)
    }
  } catch (err: any) {
    toast.error('加载系统设置失败: ' + (err.response?.data?.error || err.message))
  }
}

function triggerFileInput() {
  if (uploading.value) return
  fileInputRef.value?.click()
}

function handleFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    uploadPackageFile(target.files[0])
    target.value = ''
  }
}

function handleDrop(event: DragEvent) {
  isDragging.value = false
  if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
    uploadPackageFile(event.dataTransfer.files[0])
  }
}

async function uploadPackageFile(file: File) {
  uploading.value = true
  const formData = new FormData()
  formData.append('package', file)

  try {
    const res = await api.post('/system/upgrade/package', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    toast.success(`安装包上传成功！架构: ${res.data.arch}，版本: ${res.data.version}`)
    await loadPackages()
  } catch (err: any) {
    toast.error('安装包校验或上传失败: ' + (err.response?.data?.error || err.message))
  } finally {
    uploading.value = false
  }
}

// 触发升级本机节点
async function triggerLocalUpgrade(pkg: any) {
  const ok = await showConfirm({
    title: '确认升级本机面板服务',
    message: `即将使用安装包 ${pkg.file_name} (${pkg.arch} - ${pkg.version}) 替换本机程序。升级后服务将平滑重启约 2~3 秒，页面将自动重连，是否继续？`,
    confirmText: '立即开始升级',
    danger: true,
  })
  if (!ok) return

  isLocalUpgrading.value = true
  try {
    await api.post('/system/upgrade/local', { package_id: pkg.package_id })
    // 进入全屏重连探活浮层
    startReconnectionLoop()
  } catch (err: any) {
    toast.error('触发本地升级失败: ' + (err.response?.data?.error || err.message))
    isLocalUpgrading.value = false
  }
}

// 本地升级后探活自愈重连
function startReconnectionLoop() {
  reconnecting.value = true
  reconnectAttempts.value = 0

  const interval = setInterval(async () => {
    reconnectAttempts.value++
    try {
      const res = await api.get('/system/overview')
      if (res.status === 200) {
        clearInterval(interval)
        reconnecting.value = false
        isLocalUpgrading.value = false
        toast.success('🎉 本地节点已成功平滑重启并升级完成！', 4000)
        await loadSystemOverview()
        await loadPackages()
      }
    } catch {
      // 正在重启中，等待下一次轮询
    }

    if (reconnectAttempts.value > 30) {
      clearInterval(interval)
      reconnecting.value = false
      isLocalUpgrading.value = false
      toast.warning('探活等待已达上限，请手动刷新页面检查服务状态。')
    }
  }, 1500)
}

// 展开/收起纳管节点分发选择面板
function toggleClusterDrawer(pkg: any) {
  if (activeDistPkgId.value === pkg.package_id) {
    activeDistPkgId.value = ''
  } else {
    activeDistPkgId.value = pkg.package_id
    selectedNodeIds.value = remoteNodes.value.map(n => n.id)
  }
}

function selectAllNodes() {
  selectedNodeIds.value = remoteNodes.value.map(n => n.id)
}

function deselectAllNodes() {
  selectedNodeIds.value = []
}

// 单个纳管节点免密升级
async function upgradeSingleRemoteNode(node: any, pkg: any) {
  upgradingNodes[node.id] = true
  try {
    await api.post('/nodes/upgrade', {
      node_id: node.id,
      mode: 'api',
      package_id: pkg.package_id,
      force: false,
      host: node.host,
    })
    toast.success(`已向节点 ${node.name} 触发免密平滑升级任务！可在「纳管服务器」查看实时日志流。`)
  } catch (err: any) {
    toast.error(`节点 ${node.name} 升级启动失败: ` + (err.response?.data?.error || err.message))
  } finally {
    upgradingNodes[node.id] = false
  }
}

// 批量免密升级选中的纳管节点
async function batchUpgradeRemoteNodes(pkg: any) {
  if (selectedNodeIds.value.length === 0) {
    toast.warning('请至少勾选一个纳管节点')
    return
  }

  const ok = await showConfirm({
    title: '确认批量免密升级',
    message: `即将向已选中的 ${selectedNodeIds.value.length} 个纳管节点推送安装包 ${pkg.file_name} 并触发平滑重启，是否继续？`,
    confirmText: '立即批量升级',
    danger: false,
  })
  if (!ok) return

  isBatchUpgrading.value = true
  let successCount = 0
  let failCount = 0

  for (const id of selectedNodeIds.value) {
    const node = remoteNodes.value.find(n => n.id === id)
    if (!node) continue
    upgradingNodes[id] = true
    try {
      await api.post('/nodes/upgrade', {
        node_id: id,
        mode: 'api',
        package_id: pkg.package_id,
        force: false,
        host: node.host,
      })
      successCount++
    } catch {
      failCount++
    } finally {
      upgradingNodes[id] = false
    }
  }

  isBatchUpgrading.value = false
  if (failCount === 0) {
    toast.success(`🎉 成功触发 ${successCount} 个节点的免密升级任务！`)
  } else {
    toast.warning(`批量升级已启动：成功 ${successCount} 个，失败 ${failCount} 个`)
  }
}

async function saveSettings() {
  try {
    await api.post('/settings', {
      secret_path: settings.secret_path,
      public_ip: settings.public_ip,
      session_idle_timeout: Number(settings.session_idle_timeout),
    })
    updateSessionIdleTimeout(Number(settings.session_idle_timeout))
    toast.success('系统设置已更新！如果修改了安全路径，请使用新路径重新访问。', 5000)
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

async function revokeAllSessions() {
  const ok = await showConfirm({
    title: '强制注销所有设备会话',
    message: '确定要注销所有设备上的登录状态吗？此操作将立即废止所有已签发的访问凭据，所有已登录的浏览器均需重新输入密码登录。',
    confirmText: '立即强制注销',
    danger: true,
  })
  if (!ok) return

  try {
    await api.post('/auth/revoke-all-sessions')
    toast.success('所有会话已强制注销，请重新登录')
    clearToken('manual_revoke')
    setTimeout(() => {
      window.location.hash = '#/login'
    }, 1000)
  } catch (err: any) {
    toast.error('注销会话失败: ' + (err.response?.data?.error || err.message))
  }
}

async function updatePassword() {
  if (passForm.new_password !== passForm.confirm_password) {
    toast.warning('两次输入的新密码不一致！')
    return
  }

  try {
    await api.post('/auth/password', {
      old_password: passForm.old_password,
      new_password: passForm.new_password,
    })
    toast.success('密码修改成功，请使用新密码重新登录！', 3000)
    setTimeout(() => {
      clearToken('password_changed')
      window.location.hash = '#/login'
    }, 1200)
  } catch (err: any) {
    toast.error('修改密码失败: ' + (err.response?.data?.error || err.message))
  }
}

async function loadTelegramConfig() {
  try {
    const res = await api.get('/settings/telegram')
    if (res.data?.config) {
      tgConfig.enabled = res.data.config.enabled
      tgConfig.masked_token = res.data.config.masked_token || ''
      tgConfig.bot_token = ''
      tgConfig.chat_id = res.data.config.chat_id || ''
      tgConfig.api_url = res.data.config.api_url || 'https://api.telegram.org'
      tgConfig.events = res.data.config.events || []
      tgConfig.silent = !!res.data.config.silent

      // 初次使用未配置时，默认展开新手引导教程
      if (!res.data.config.masked_token) {
        showTgGuide.value = true
      }
    }
  } catch {
    // 忽略加载错误
  }
}

async function saveTelegramConfig() {
  if (tgConfig.enabled && !tgConfig.chat_id.trim()) {
    toast.warning('启用告警时，Chat ID 不能为空！')
    return
  }

  savingTelegram.value = true
  try {
    await api.post('/settings/telegram', {
      enabled: tgConfig.enabled,
      bot_token: tgConfig.bot_token.trim(),
      chat_id: tgConfig.chat_id.trim(),
      api_url: tgConfig.api_url.trim(),
      events: tgConfig.events,
      silent: tgConfig.silent,
    })
    toast.success('Telegram 告警配置保存成功！')
    await loadTelegramConfig()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  } finally {
    savingTelegram.value = false
  }
}

async function testTelegram() {
  if (!tgConfig.chat_id.trim()) {
    toast.warning('请先输入接收通知的 Chat ID！')
    return
  }
  if (!tgConfig.bot_token.trim() && !tgConfig.masked_token) {
    toast.warning('请先输入 Telegram Bot Token！')
    return
  }

  testingTelegram.value = true
  testResult.value = null
  try {
    const res = await api.post('/settings/telegram/test', {
      bot_token: tgConfig.bot_token.trim(),
      chat_id: tgConfig.chat_id.trim(),
      api_url: tgConfig.api_url.trim(),
    })
    toast.success('测试告警发送成功，请前往 Telegram 查看机器人推送！')
    testResult.value = {
      success: true,
      message: res.data?.message || '测试消息已成功推送并送达 Telegram，请在客户端查收确认！',
    }
  } catch (err: any) {
    const errMsg = err.response?.data?.error || err.message
    toast.error('连通性测试失败: ' + errMsg)
    testResult.value = {
      success: false,
      message: errMsg,
    }
  } finally {
    testingTelegram.value = false
  }
}

function selectAllEvents() {
  tgConfig.events = EVENT_DEFINITIONS.map(e => e.key)
}

function selectCriticalEvents() {
  tgConfig.events = EVENT_DEFINITIONS.filter(e => e.level === 'critical' || e.level === 'warn').map(e => e.key)
}

onMounted(() => {
  loadSystemOverview()
  loadPackages()
  loadRemoteNodes()
  loadSettings()
  loadTelegramConfig()
})
</script>

<style scoped>
.upload-dropzone:hover {
  border-color: #6366f1 !important;
  background: rgba(99, 102, 241, 0.08) !important;
}
.dropzone-active {
  border-color: #38bdf8 !important;
  background: rgba(56, 189, 248, 0.12) !important;
}

@keyframes indeterminate {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.pulse-indicator {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #38bdf8;
}

.pulse-active {
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(56, 189, 248, 0.7); }
  70% { transform: scale(1); box-shadow: 0 0 0 6px rgba(56, 189, 248, 0); }
  100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(56, 189, 248, 0); }
}
</style>
