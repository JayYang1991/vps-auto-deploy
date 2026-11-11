<template>
  <div v-show="modelValue" class="modal-overlay" @click.self="handleOverlayClick">
    <div class="cluster-modal">
      <!-- 模态框头部 -->
      <div class="modal-header">
        <div class="header-title-box">
          <span class="header-icon">🖥️</span>
          <div>
            <h3 class="header-title">多节点集群管理</h3>
            <p class="header-desc">统一纳管多台远程服务器，实现跨节点业务配置与穿透流转</p>
          </div>
        </div>

        <button class="close-btn" @click="close" type="button">✕</button>
      </div>

      <!-- 顶部分类 Tab -->
      <div class="cluster-tabs">
        <button
          class="tab-btn"
          :class="{ 'tab-btn-active': activeTab === 'nodes' }"
          @click="activeTab = 'nodes'"
        >
          <span>🌐 节点列表</span>
          <span class="tab-badge">{{ nodes.length }}</span>
        </button>

        <button
          class="tab-btn"
          :class="{ 'tab-btn-active': activeTab === 'cloud' }"
          @click="activeTab = 'cloud'"
        >
          <span>☁️ 云端弹性创建</span>
        </button>

        <button
          class="tab-btn"
          :class="{ 'tab-btn-active': activeTab === 'deploy' }"
          @click="switchToDeployTab"
        >
          <span>➕ SSH 远程部署</span>
        </button>

        <button
          class="tab-btn"
          :class="{ 'tab-btn-active': activeTab === 'import' }"
          @click="activeTab = 'import'"
        >
          <span>🔗 纳管已有节点</span>
        </button>
      </div>

      <!-- Tab 1: 节点列表 -->
      <div v-show="activeTab === 'nodes'" class="tab-content">
        <div class="nodes-grid">
          <div
            v-for="item in nodes"
            :key="item.id"
            class="node-card"
            :class="{ 'node-card-active': item.id === activeNodeId }"
          >
            <div class="node-card-header">
              <div class="node-title-group">
                <span class="node-badge-icon">{{ item.is_master ? '🏠' : '🌐' }}</span>
                <div>
                  <div class="node-name-row">
                    <span class="card-name">{{ item.name }}</span>
                    <span v-if="item.is_master" class="tag-master">主控节点</span>
                    <span v-if="item.cloud_provider === 'vultr'" class="tag-vultr">☁️ Vultr · {{ (item.region || '').toUpperCase() }}</span>
                    <span v-if="item.use_tls" class="tag-tls" title="已启用自签 TLS 证书指纹锁定，抵御中间人劫持">🛡️ TLS 防劫持</span>
                    <span v-if="item.id === activeNodeId" class="tag-current">当前视角</span>
                  </div>
                  <div class="card-host">{{ item.use_tls ? 'https://' : 'http://' }}{{ item.host }}:{{ item.panel_port }}</div>
                </div>
              </div>

              <!-- 状态与测速徽标 -->
              <div class="node-status-group">
                <span
                  class="status-pill"
                  :class="{
                    'status-online': item.status === 'online',
                    'status-offline': item.status !== 'online'
                  }"
                >
                  <span class="status-dot"></span>
                  {{ item.status === 'online' ? `${item.latency_ms} ms` : '离线' }}
                </span>
              </div>
            </div>

            <!-- 节点参数属性明细 -->
            <div class="node-props">
              <div v-if="item.cloud_provider" class="prop-item">
                <span class="prop-label">云端规格:</span>
                <span class="prop-val mono">{{ item.cloud_provider }} · {{ item.plan || 'VPS' }}</span>
              </div>
              <div class="prop-item">
                <span class="prop-label">系统环境:</span>
                <span class="prop-val">{{ item.os_info || 'Linux' }}</span>
              </div>
              <div class="prop-item">
                <span class="prop-label">面板入口:</span>
                <span class="prop-val mono">/{{ item.secret_path }}/</span>
              </div>
              <div class="prop-item">
                <span class="prop-label">管理账号:</span>
                <span class="prop-val mono">{{ item.username }}</span>
              </div>
              <div v-if="item.use_tls && item.cert_fingerprint" class="prop-item">
                <span class="prop-label">TLS 锁定指纹:</span>
                <span class="prop-val mono text-fingerprint" :title="item.cert_fingerprint">
                  {{ item.cert_fingerprint.slice(0, 17) }}...
                </span>
              </div>
              <div v-if="item.ssh_host_key" class="prop-item">
                <span class="prop-label">SSH 指纹(TOFU):</span>
                <span class="prop-val mono text-fingerprint" :title="item.ssh_host_key">
                  {{ item.ssh_host_key.slice(0, 19) }}...
                </span>
              </div>
              <div class="prop-item">
                <span class="prop-label">订阅前缀:</span>
                <span class="prop-val font-semibold text-emerald-400">{{ item.sub_prefix || '(未设置，默认用节点名)' }}</span>
              </div>
            </div>

            <!-- 操作按钮组 -->
            <div class="node-card-actions">
              <button
                v-if="item.id !== activeNodeId"
                class="btn-action btn-switch"
                @click="handleSwitch(item.id)"
                type="button"
              >
                <span>⚡ 切换视角</span>
              </button>
              <span v-else class="active-notice">✓ 当前正在管理此节点</span>

              <button
                class="btn-action btn-terminal"
                @click="openTerminalModal(item)"
                type="button"
                title="开启交互式 SSH / 控制台 Web 终端"
              >
                <span>💻 控制台</span>
              </button>

              <button
                class="btn-action btn-prefix"
                @click="openPrefixModal(item)"
                type="button"
                title="定制生成订阅节点信息时的名称前缀 (如 🇭🇰 香港主控, 🇯🇵 日本东京)"
              >
                <span>✏️ 定制前缀</span>
              </button>

              <button
                class="btn-action btn-ping"
                :disabled="pingingMap[item.id]"
                @click="handlePing(item.id)"
                type="button"
                title="重新测量该节点网络延迟"
              >
                <span>{{ pingingMap[item.id] ? '⏳ 测速中' : '📶 测速' }}</span>
              </button>

              <button
                class="btn-action btn-reveal"
                @click="openRevealModal(item)"
                type="button"
                title="需二次验证主控密码以解密查看凭据"
              >
                <span>🔑 查看凭据</span>
              </button>

              <button
                v-if="!item.is_master"
                class="btn-action btn-upgrade"
                @click="openUpgradeModal(item)"
                type="button"
                title="向该节点推流升级最新版 vps-panel 二进制并自动同步全网凭据"
              >
                <span>🚀 升级节点</span>
              </button>

              <button
                v-if="!item.is_master"
                class="btn-action btn-delete"
                @click="handleDelete(item)"
                type="button"
                title="从集群中移出此节点"
              >
                <span>🗑️</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab 2: 云端弹性创建 -->
      <div v-show="activeTab === 'cloud'" class="tab-content" style="padding: 1.25rem;">
        <VultrProvisioner @deployed="handleCloudDeployed" @close="close" @task-change="handleVultrTaskChange" />
      </div>

      <!-- Tab 3: SSH 远程一键部署 -->
      <div v-show="activeTab === 'deploy'" class="tab-content">
        <!-- 部署配置表单 -->
        <div v-if="deployStep === 'form'" class="deploy-form">
          <div class="form-alert-info">
            <span>ℹ️</span>
            <div>
              <strong>零外网依赖高速安装：</strong>
              若目标机器与主控架构相同（如 Linux amd64），主控端将直接通过 SSH 管道推流传输面板程序，数秒内即可完成极速部署与守护进程挂载。
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>节点友好名称 <span class="req">*</span></label>
              <input v-model="deployReq.name" type="text" placeholder="例如：香港 HK-01 或 日本东京" />
            </div>

            <div class="form-group">
              <label>订阅节点前缀 (可选，全协议生效)</label>
              <input v-model="deployReq.sub_prefix" type="text" placeholder="例如：🇭🇰 香港01 (留空默认使用名称，全协议生效)" />
            </div>

            <div class="form-group">
              <label>目标服务器 IP / 域名 <span class="req">*</span></label>
              <input v-model="deployReq.host" type="text" placeholder="例如：192.168.1.50 或 hk.example.com" />
            </div>

            <div class="form-group">
              <label>SSH 端口 <span class="req">*</span></label>
              <input v-model.number="deployReq.ssh_port" type="number" placeholder="22" />
            </div>

            <div class="form-group">
              <label>SSH 用户名 <span class="req">*</span></label>
              <input v-model="deployReq.ssh_user" type="text" placeholder="root" />
            </div>

            <div class="form-group full-width">
              <label>SSH 认证方式</label>
              <div class="radio-group">
                <label class="radio-label">
                  <input type="radio" value="password" v-model="deployReq.auth_type" />
                  <span>密码认证</span>
                </label>
                <label class="radio-label">
                  <input type="radio" value="key" v-model="deployReq.auth_type" />
                  <span>SSH 私钥认证</span>
                </label>
              </div>
            </div>

            <div v-if="deployReq.auth_type === 'password'" class="form-group full-width">
              <label>SSH 登录密码 <span class="req">*</span></label>
              <input v-model="deployReq.ssh_password" type="password" placeholder="输入远程主机 root 密码" />
            </div>

            <div v-else class="form-group full-width">
              <label>SSH 私钥内容 (OpenSSH PEM) <span class="req">*</span></label>
              <textarea
                v-model="deployReq.ssh_private_key"
                rows="4"
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
                class="mono"
              ></textarea>
            </div>

            <div class="form-divider full-width">
              <span>被纳管面板初始化参数</span>
            </div>

            <div class="form-group">
              <label>面板监听端口</label>
              <input v-model.number="deployReq.panel_port" type="number" placeholder="20888" />
            </div>

            <div class="form-group">
              <label>管理账号</label>
              <input v-model="deployReq.admin_username" type="text" placeholder="admin" />
            </div>

            <div class="form-group">
              <label>管理密码 (留空随机生成)</label>
              <input v-model="deployReq.admin_password" type="text" placeholder="留空则自动生成高强度随机密码" />
            </div>

            <div class="form-group">
              <label>安全访问路径 (留空随机生成)</label>
              <input v-model="deployReq.secret_path" type="text" placeholder="留空则生成 8 位随机前缀" />
            </div>

            <!-- Phase 1 防中间人劫持选项 -->
            <div class="form-group full-width" style="margin-top: 0.5rem; background: rgba(99, 102, 241, 0.08); padding: 0.75rem; border-radius: 0.5rem; border: 1px dashed rgba(99, 102, 241, 0.3);">
              <label class="checkbox-container">
                <input type="checkbox" v-model="deployReq.enable_tls" />
                <span class="checkbox-title">🛡️ 启用自签名 TLS 证书与指纹强绑定 (强烈推荐 · 防中间人劫持)</span>
              </label>
              <div class="field-hint">
                目标服务器将生成专属 ECDSA TLS 证书并提取 SHA-256 唯一指纹，主控端在所有日常 API 通信与日志流转发时强制校验指纹，杜绝公网流量窃听、篡改与 DNS 欺骗。
              </div>
            </div>
          </div>

          <div class="form-footer">
            <button class="btn-primary" @click="startDeploy" :disabled="isDeploying">
              <span>🚀 立即开始 SSH 自动化部署</span>
            </button>
          </div>
        </div>

        <!-- 实时部署日志流终端 -->
        <div v-else class="deploy-terminal-box">
          <div class="terminal-header">
            <div class="terminal-status-info">
              <span class="pulse-indicator" :class="{ 'pulse-active': deployTaskStatus === 'running' }"></span>
              <span class="terminal-title">
                {{ deployTaskStatus === 'running' ? '正在执行远程部署与集群握手...' : (deployTaskStatus === 'success' ? '🎉 部署完成' : '❌ 部署失败') }}
              </span>
            </div>

            <span class="terminal-node-name">{{ deployReq.name || deployReq.host }}</span>
          </div>

          <div class="terminal-body" ref="terminalBodyRef">
            <div v-for="(log, idx) in deployLogs" :key="idx" class="terminal-line" :class="getLineClass(log)">
              {{ log }}
            </div>
          </div>

          <div class="terminal-footer">
            <template v-if="deployTaskStatus === 'success'">
              <button
                class="btn-primary"
                @click="finishDeployAndSwitch"
                type="button"
              >
                <span>✅ 完成并切换至该节点视角</span>
              </button>
              <button
                class="btn-secondary"
                @click="resetDeployForm"
                type="button"
                style="margin-left: 0.5rem;"
                title="清空表单并准备部署另一台新服务器"
              >
                <span>➕ 继续部署新节点</span>
              </button>
            </template>
            <button
              v-else-if="deployTaskStatus === 'failed'"
              class="btn-secondary"
              @click="deployStep = 'form'"
              type="button"
            >
              <span>🔄 返回修改参数并重试</span>
            </button>
            <span v-else class="deploy-progress-hint">
              请保持页面开启，系统正在挂载守护进程并执行 Token 交换...
            </span>
          </div>
        </div>
      </div>

      <!-- Tab 4: 纳管已有节点 -->
      <div v-show="activeTab === 'import'" class="tab-content">
        <div class="deploy-form">
          <div class="form-alert-info">
            <span>🔗</span>
            <div>
              <strong>纳管已运行面板：</strong>
              若您已在其他服务器上安装并启动了 VPS Auto Deploy 面板，在此填入其公网地址、端口与管理员凭据即可快速加入本集群。
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>节点友好名称 <span class="req">*</span></label>
              <input v-model="importReq.name" type="text" placeholder="例如：美国西雅图 或 备用节点 02" />
            </div>

            <div class="form-group">
              <label>订阅节点前缀 (可选，全协议生效)</label>
              <input v-model="importReq.sub_prefix" type="text" placeholder="例如：🇺🇸 美西02 (留空默认使用名称，全协议生效)" />
            </div>

            <div class="form-group">
              <label>服务器公网 IP / 域名 <span class="req">*</span></label>
              <input v-model="importReq.host" type="text" placeholder="例如：198.51.100.22" />
            </div>

            <div class="form-group">
              <label>面板监听端口 <span class="req">*</span></label>
              <input v-model.number="importReq.panel_port" type="number" placeholder="20888" />
            </div>

            <div class="form-group full-width">
              <label>安全访问路径 (Secret Path) <span class="req">*</span></label>
              <input v-model="importReq.secret_path" type="text" placeholder="例如：fa480cbd (不含前后斜杠)" />
            </div>

            <div class="form-group">
              <label>管理员账号 <span class="req">*</span></label>
              <input v-model="importReq.username" type="text" placeholder="admin" />
            </div>

            <div class="form-group">
              <label>管理员密码 <span class="req">*</span></label>
              <input v-model="importReq.password" type="password" placeholder="输入该面板的管理员密码" />
            </div>

            <div class="form-group full-width" style="margin-top: 0.25rem;">
              <label class="checkbox-container">
                <input type="checkbox" v-model="importReq.use_tls" />
                <span class="checkbox-title">该节点已开启 TLS 加密 (HTTPS)</span>
              </label>
            </div>

            <div v-if="importReq.use_tls" class="form-group full-width">
              <label>TLS 证书锁定指纹 (可选，留空则在首次连接时信任并绑定)</label>
              <input v-model="importReq.cert_fingerprint" type="text" placeholder="例如：7b:9c:8a... (SHA-256 指纹)" class="mono" />
            </div>
          </div>

          <div class="form-footer">
            <button class="btn-primary" @click="handleImport" :disabled="isImporting">
              <span>{{ isImporting ? '⏳ 正在握手验证...' : '🔗 立即接入集群' }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 二次鉴权查看密码模态框 (Step-Up Authentication Modal) -->
    <div v-if="revealModalVisible" class="reveal-overlay" @click.self="closeRevealModal">
      <div class="reveal-card">
        <div class="reveal-header">
          <div class="reveal-title-box">
            <span class="reveal-icon">🔐</span>
            <div>
              <h4 class="reveal-title">二次安全鉴权</h4>
              <p class="reveal-desc">查看被纳管节点密码需验证主控面板管理员身份</p>
            </div>
          </div>
          <button class="close-btn" @click="closeRevealModal" type="button">✕</button>
        </div>

        <!-- 步骤 1：输入主控密码 -->
        <div v-if="!revealedData" class="reveal-body">
          <div class="reveal-prompt">
            您正在请求解密并查看节点 <strong>{{ targetRevealNode?.name }}</strong> ({{ targetRevealNode?.host }}) 的登录凭据。
          </div>

          <div class="form-group" style="margin-top: 1rem;">
            <label>主控管理员密码 <span class="req">*</span></label>
            <input
              v-model="masterVerifyPassword"
              type="password"
              placeholder="请输入当前主控端的登录密码"
              @keyup.enter="submitReveal"
            />
          </div>

          <div class="reveal-footer">
            <button class="btn-secondary" @click="closeRevealModal">取消</button>
            <button class="btn-primary" @click="submitReveal" :disabled="verifyingReveal || !masterVerifyPassword">
              <span>{{ verifyingReveal ? '⏳ 鉴权中...' : '🔓 验证并查看' }}</span>
            </button>
          </div>
        </div>

        <!-- 步骤 2：凭据解密成功展示与 60s 倒计时锁定 -->
        <div v-else class="reveal-body">
          <div class="credentials-box">
            <div class="countdown-bar">
              <span>⏳ 凭据保护中：将在 <strong>{{ countdownSeconds }}</strong> 秒后自动隐藏锁定</span>
            </div>

            <div class="cred-row">
              <span class="cred-label">节点名称:</span>
              <span class="cred-val">{{ revealedData.name }}</span>
            </div>

            <div class="cred-row">
              <span class="cred-label">管理账号:</span>
              <div class="cred-val-copy">
                <span class="mono">{{ revealedData.username }}</span>
                <button class="btn-mini-copy" @click="copyText(revealedData.username, '账号')">复制</button>
              </div>
            </div>

            <div class="cred-row">
              <span class="cred-label">管理密码:</span>
              <div class="cred-val-copy">
                <span class="mono">{{ showPasswordPlainText ? revealedData.password : '••••••••••••' }}</span>
                <button class="btn-mini-toggle" @click="showPasswordPlainText = !showPasswordPlainText">
                  {{ showPasswordPlainText ? '🙈 隐藏' : '👁️ 显示' }}
                </button>
                <button class="btn-mini-copy" @click="copyText(revealedData.password, '密码')">复制</button>
              </div>
            </div>

            <div class="cred-row">
              <span class="cred-label">安全入口:</span>
              <div class="cred-val-copy">
                <span class="mono">/{{ revealedData.secret_path }}/</span>
                <button class="btn-mini-copy" @click="copyText(`/${revealedData.secret_path}/`, '安全路径')">复制</button>
              </div>
            </div>

            <div v-if="revealedData.use_tls" class="cred-row">
              <span class="cred-label">传输协议:</span>
              <div class="cred-val-copy">
                <span class="tag-tls" style="margin: 0;">🛡️ HTTPS (TLS 指纹锁定)</span>
              </div>
            </div>

            <div v-if="revealedData.cert_fingerprint" class="cred-row">
              <span class="cred-label">TLS 指纹:</span>
              <div class="cred-val-copy">
                <span class="mono text-fingerprint" :title="revealedData.cert_fingerprint">{{ revealedData.cert_fingerprint }}</span>
                <button class="btn-mini-copy" @click="copyText(revealedData.cert_fingerprint, '证书指纹')">复制</button>
              </div>
            </div>

            <div v-if="revealedData.ssh_host_key" class="cred-row">
              <span class="cred-label">SSH 指纹(TOFU):</span>
              <div class="cred-val-copy">
                <span class="mono text-fingerprint" :title="revealedData.ssh_host_key">{{ revealedData.ssh_host_key }}</span>
                <button class="btn-mini-copy" @click="copyText(revealedData.ssh_host_key, '主机指纹')">复制</button>
              </div>
            </div>

            <div class="cred-row">
              <span class="cred-label">面板直连 URL:</span>
              <div class="cred-val-copy">
                <a :href="revealedData.login_url" target="_blank" class="cred-link mono">{{ revealedData.login_url }}</a>
                <button class="btn-mini-copy" @click="copyText(revealedData.login_url, '直达网址')">复制</button>
              </div>
            </div>
          </div>

          <div class="reveal-footer">
            <button class="btn-secondary" @click="closeRevealModal">关闭并锁定</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 云端节点销毁二次安全确认模态框 -->
    <div v-if="cloudDeleteModalVisible" class="reveal-overlay" @click.self="closeCloudDeleteModal">
      <div class="reveal-card">
        <div class="reveal-header">
          <div class="reveal-title-box">
            <span class="reveal-icon">☁️</span>
            <div>
              <h4 class="reveal-title">移出/销毁云端实例确认</h4>
              <p class="reveal-desc">节点由 Vultr 云平台纳管创建，请选择处理策略</p>
            </div>
          </div>
          <button class="close-btn" @click="closeCloudDeleteModal" type="button">✕</button>
        </div>

        <div class="reveal-body">
          <div class="reveal-prompt">
            您正在操作云节点 <strong>{{ targetDeleteNode?.name }}</strong> ({{ targetDeleteNode?.host }} · {{ (targetDeleteNode?.region || '').toUpperCase() }})。
          </div>

          <div class="cloud-delete-option-box" style="margin-top: 1rem; background: rgba(239, 68, 68, 0.08); border: 1px dashed rgba(239, 68, 68, 0.35); padding: 0.85rem; border-radius: 0.5rem;">
            <label class="checkbox-container">
              <input type="checkbox" v-model="destroyCloudInstance" />
              <span class="checkbox-title" style="color: #fca5a5;">💥 同时彻底销毁 Vultr 云端服务器实例 (立即停止扣费)</span>
            </label>
            <div class="field-hint" style="color: #cbd5e1; margin-top: 0.35rem;">
              {{ destroyCloudInstance ? '警告：此操作不可逆！Vultr 远端服务器与系统磁盘将被永久删除并终止计费。' : '仅解除集群纳管，保留 Vultr 云服务器继续运行（将持续产生计费）。' }}
            </div>
          </div>

          <!-- 若选择彻底销毁云服务器，必须二次验证主控管理员密码 -->
          <div v-if="destroyCloudInstance" class="form-group" style="margin-top: 1rem;">
            <label>主控管理员密码 <span class="req">*</span></label>
            <input
              v-model="deleteMasterPassword"
              type="password"
              placeholder="请输入当前主控端管理员密码以授权销毁"
              @keyup.enter="submitCloudDelete"
            />
          </div>

          <div class="reveal-footer" style="margin-top: 1.25rem;">
            <button class="btn-secondary" @click="closeCloudDeleteModal">取消</button>
            <button
              class="btn-action"
              :class="destroyCloudInstance ? 'btn-danger-strong' : 'btn-primary'"
              @click="submitCloudDelete"
              :disabled="deletingNode || (destroyCloudInstance && !deleteMasterPassword)"
            >
              <span>{{ deletingNode ? '⏳ 正在处理...' : (destroyCloudInstance ? '💥 确认销毁并移出' : '🔗 仅解除纳管') }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 定制订阅节点名称前缀模态框 -->
    <div v-if="prefixModalVisible" class="reveal-overlay" @click.self="closePrefixModal">
      <div class="reveal-card">
        <div class="reveal-header">
          <div class="reveal-title-box">
            <span class="reveal-icon">✏️</span>
            <div>
              <h4 class="reveal-title">定制订阅节点前缀</h4>
              <p class="reveal-desc">设置该 VPS 在导出 Clash / Sing-box / Subconverter 时的名称与分流前缀 (全协议统一生效)</p>
            </div>
          </div>
          <button class="close-btn" @click="closePrefixModal" type="button">✕</button>
        </div>

        <div class="reveal-body">
          <div class="form-alert-info" style="margin-bottom: 1rem;">
            <span>💡</span>
            <div>
              <strong>全协议前缀统一生效：</strong>
              此前缀对该节点下的所有协议（VLESS-Reality、Hysteria 2、Shadowsocks、Trojan、TUIC、VMess、SOCKS5、VLESS-WS）完全生效，包含地区旗帜与地名（如 <code>🇭🇰 香港</code>、<code>🇯🇵 日本</code>、<code>🇺🇸 美国</code>），便于 Subconverter 与客户端自动归类至国家分流组。
            </div>
          </div>

          <div class="form-group" style="margin-bottom: 0.85rem;">
            <label>节点友好名称 (Name) <span class="req">*</span></label>
            <input
              v-model="editingNodeName"
              type="text"
              placeholder="例如：🏠 本机 (Master) 或 香港-HK01"
            />
          </div>

          <div class="form-group" style="margin-bottom: 0.85rem;">
            <label>订阅节点前缀 (Sub Prefix) - <span style="color: #34d399; font-weight: normal;">全协议统一生效</span></label>
            <input
              v-model="editingNodePrefix"
              type="text"
              placeholder="例如：🇭🇰 香港主控 或 🇯🇵 日本东京"
            />
          </div>

          <!-- 常用地区快捷填入药丸 -->
          <div class="form-group" style="margin-bottom: 1rem;">
            <label style="font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.35rem;">常用地区预设快捷填入：</label>
            <div class="preset-pill-row">
              <button
                v-for="p in regionPresets"
                :key="p.name"
                type="button"
                class="preset-pill-btn"
                @click="applyPrefixPreset(p)"
              >
                {{ p.emoji }} {{ p.name }}
              </button>
            </div>
          </div>

          <!-- 实时效果预览 (全协议统一) -->
          <div class="preview-box">
            <div style="font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.5rem; font-weight: 600; display: flex; justify-content: space-between; align-items: center;">
              <span>🔍 客户端订阅节点名称效果预览 (全协议统一生效)：</span>
              <span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #34d399; font-size: 0.68rem; padding: 0.15rem 0.45rem;">8 大协议统一支持</span>
            </div>
            <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 0.4rem; max-height: 200px; overflow-y: auto;">
              <div v-for="p in protocolPreviews" :key="p.label" class="preview-item">
                <span class="preview-label" style="min-width: 6.8rem;">{{ p.label }}:</span>
                <span class="preview-code mono">{{ p.name }}</span>
              </div>
            </div>
          </div>

          <div class="reveal-footer" style="margin-top: 1.25rem;">
            <button class="btn-secondary" @click="closePrefixModal" type="button">取消</button>
            <button class="btn-primary" @click="saveNodePrefix" :disabled="savingPrefix" type="button">
              <span>{{ savingPrefix ? '⏳ 保存中...' : '💾 保存前缀设置' }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 远程节点升级模态框 (Remote Node Upgrade Modal) -->
    <div v-if="upgradeModalVisible" class="reveal-overlay" @click.self="closeUpgradeModal">
      <div class="reveal-card" style="max-width: 620px; width: 95%;">
        <div class="reveal-header">
          <div class="reveal-title-box">
            <span class="reveal-icon">🚀</span>
            <div>
              <h4 class="reveal-title">升级远程节点程序</h4>
              <p class="reveal-desc">目标节点：{{ targetUpgradeNode?.name }} ({{ targetUpgradeNode?.host }})</p>
            </div>
          </div>
          <button class="close-btn" @click="closeUpgradeModal" type="button" :disabled="isUpgrading && upgradeTaskStatus === 'running'">✕</button>
        </div>

        <!-- 步骤 1：升级配置选项 -->
        <div v-if="upgradeStep === 'form'" class="reveal-body">
          <!-- 升级模式切换 -->
          <div style="display: flex; gap: 0.5rem; margin-bottom: 1rem; border-bottom: 1px solid #334155; padding-bottom: 0.75rem;">
            <button
              type="button"
              class="btn-tab"
              :class="{ 'btn-tab-active': upgradeReq.mode === 'api' }"
              @click="upgradeReq.mode = 'api'"
              style="padding: 0.4rem 0.85rem; font-size: 0.8125rem; border-radius: 0.375rem; border: 1px solid #475569; cursor: pointer;"
            >
              ⚡ 免密极速升级 (推荐)
            </button>
            <button
              type="button"
              class="btn-tab"
              :class="{ 'btn-tab-active': upgradeReq.mode === 'ssh' }"
              @click="upgradeReq.mode = 'ssh'"
              style="padding: 0.4rem 0.85rem; font-size: 0.8125rem; border-radius: 0.375rem; border: 1px solid #475569; cursor: pointer;"
            >
              🛠️ SSH 救援升级
            </button>
          </div>

          <!-- 模式一：API 免密极速升级 (Zero-SSH) -->
          <div v-if="upgradeReq.mode === 'api'">
            <div class="form-alert-info" style="margin-bottom: 1rem;">
              <span>⚡</span>
              <div>
                <strong>管理凭据免密极速升级 (Zero-SSH)：</strong>
                主控端将直接通过内部加密通道（API Token + TLS 证书指纹锁定）向节点推送最新版程序并触发平滑热自启，<strong>无需输入任何 SSH 密码或私钥！</strong>
                节点所有数据库、TLS 证书、入站/出站及穿透业务配置均完好保留，升级完成后全网凭据自动无缝重连。
              </div>
            </div>

            <div style="background: rgba(15, 23, 42, 0.6); border: 1px solid #334155; border-radius: 0.5rem; padding: 0.85rem; margin-bottom: 1rem; font-size: 0.8125rem;">
              <div style="display: flex; justify-content: space-between; margin-bottom: 0.4rem;">
                <span style="color: #94a3b8;">目标系统架构:</span>
                <span style="color: #38bdf8; font-weight: 600;">{{ targetUpgradeNode?.os_info || 'Linux (amd64)' }}</span>
              </div>
              <div style="display: flex; justify-content: space-between; margin-bottom: 0.4rem;">
                <span style="color: #94a3b8;">管理通信安全:</span>
                <span style="color: #10b981; font-weight: 600;">{{ targetUpgradeNode?.use_tls ? '🔒 HTTPS (Pinned TLS 指纹防护)' : '🌐 HTTP 直连' }}</span>
              </div>
              <div style="display: flex; justify-content: space-between;">
                <span style="color: #94a3b8;">当前在线状态:</span>
                <span :style="{ color: targetUpgradeNode?.status === 'online' ? '#10b981' : '#f43f5e', fontWeight: '600' }">
                  {{ targetUpgradeNode?.status === 'online' ? '● 在线就绪' : '● 离线 (若失联建议切换至 SSH 救援模式)' }}
                </span>
              </div>
            </div>

            <!-- 可选选择导入的安装包 -->
            <div v-if="availablePackages.length > 0" class="form-group full-width" style="margin-bottom: 0.75rem;">
              <label>选择升级程序包</label>
              <select v-model="upgradeReq.package_id" class="input" style="width: 100%; box-sizing: border-box; background: rgba(15, 23, 42, 0.8); border: 1px solid #334155; border-radius: 0.375rem; color: #f8fafc; padding: 0.5rem; font-size: 0.8125rem;">
                <option value="">(默认) 自动适配最佳程序包 / 当前运行版本</option>
                <option v-for="pkg in availablePackages" :key="pkg.package_id" :value="pkg.package_id">
                  {{ pkg.file_name }} ({{ pkg.arch }} - {{ pkg.version }}) - SHA: {{ pkg.sha256.substring(0, 8) }}
                </option>
              </select>
            </div>

            <div class="form-group full-width" style="margin-top: 0.25rem;">
              <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer; color: #94a3b8; font-size: 0.8125rem;">
                <input type="checkbox" v-model="upgradeReq.force" style="accent-color: #6366f1; width: 1rem; height: 1rem;" />
                <span>强制重新推流覆盖 (默认通过 SHA-256 指纹比对：若远端版本已与主控一致则秒级跳过传输与重启，勾选后强制重新覆写并重启服务)</span>
              </label>
            </div>
          </div>

          <!-- 模式二：SSH 救援模式 -->
          <div v-else>
            <div class="form-alert-info" style="margin-bottom: 1rem; border-color: #f59e0b; background: rgba(245, 158, 11, 0.1);">
              <span style="color: #f59e0b;">🚑</span>
              <div>
                <strong>SSH 底层救援重装升级：</strong>
                仅在节点 Web API 彻底崩溃离线或无法连通时使用。主控将通过底层 SSH 管道强制连接宿主机执行二进制推流替换并重启 systemd 守护进程。
              </div>
            </div>

            <div class="form-grid" style="grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div class="form-group">
                <label>SSH 端口 <span class="req">*</span></label>
                <input v-model.number="upgradeReq.ssh_port" type="number" placeholder="22" />
              </div>

              <div class="form-group">
                <label>SSH 用户名 <span class="req">*</span></label>
                <input v-model="upgradeReq.ssh_user" type="text" placeholder="root" />
              </div>

              <div class="form-group full-width">
                <label>SSH 认证方式 <span class="req">*</span></label>
                <div class="radio-group" style="display: flex; gap: 1rem; margin-top: 0.25rem;">
                  <label class="radio-label" style="display: flex; align-items: center; gap: 0.35rem; cursor: pointer; color: #cbd5e1; font-size: 0.8125rem;">
                    <input type="radio" value="password" v-model="upgradeReq.auth_type" />
                    <span>密码认证</span>
                  </label>
                  <label class="radio-label" style="display: flex; align-items: center; gap: 0.35rem; cursor: pointer; color: #cbd5e1; font-size: 0.8125rem;">
                    <input type="radio" value="key" v-model="upgradeReq.auth_type" />
                    <span>SSH 私钥认证</span>
                  </label>
                </div>
              </div>

              <div v-if="upgradeReq.auth_type === 'password'" class="form-group full-width">
                <label>SSH 登录密码 <span class="req">*</span></label>
                <input v-model="upgradeReq.ssh_password" type="password" placeholder="输入远程主机 root 密码" @keyup.enter="startUpgrade" />
              </div>

              <div v-else class="form-group full-width">
                <label>SSH 私钥内容 (PEM) <span class="req">*</span></label>
                <textarea
                  v-model="upgradeReq.ssh_private_key"
                  rows="4"
                  class="mono"
                  style="width: 100%; box-sizing: border-box; background: rgba(15, 23, 42, 0.8); border: 1px solid #334155; border-radius: 0.375rem; color: #f8fafc; padding: 0.5rem; font-size: 0.8125rem;"
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"
                ></textarea>
              </div>

              <div class="form-group full-width" style="margin-top: 0.25rem;">
                <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer; color: #94a3b8; font-size: 0.8125rem;">
                  <input type="checkbox" v-model="upgradeReq.force" style="accent-color: #6366f1; width: 1rem; height: 1rem;" />
                  <span>强制重新推流覆盖</span>
                </label>
              </div>
            </div>
          </div>

          <div class="reveal-footer" style="margin-top: 1.25rem;">
            <button class="btn-secondary" @click="closeUpgradeModal" type="button">取消</button>
            <button class="btn-primary" @click="startUpgrade" :disabled="isUpgrading" type="button">
              <span>{{ isUpgrading ? '⏳ 正在升级...' : (upgradeReq.mode === 'api' ? '⚡ 立即开始免密平滑升级' : '🚑 立即开始 SSH 救援升级') }}</span>
            </button>
          </div>
        </div>

        <!-- 步骤 2：实时推流升级控制台输出 -->
        <div v-else class="reveal-body" style="padding: 0;">
          <div class="terminal-container" style="border-radius: 0 0 0.5rem 0.5rem; border: none;">
            <div class="terminal-header">
              <div class="terminal-status-info">
                <span class="pulse-indicator" :class="{ 'pulse-active': upgradeTaskStatus === 'running' }"></span>
                <span class="terminal-title">
                  {{ upgradeTaskStatus === 'running' ? '正在执行二进制推流与平滑升级...' : (upgradeTaskStatus === 'success' ? '🎉 升级成功' : '❌ 升级失败') }}
                </span>
              </div>
              <span class="terminal-node-name">{{ targetUpgradeNode?.name }}</span>
            </div>

            <div class="terminal-body" ref="upgradeTerminalBodyRef" style="max-height: 280px;">
              <div v-for="(log, idx) in upgradeLogs" :key="idx" class="terminal-line" :class="getLineClass(log)">
                {{ log }}
              </div>
            </div>

            <div class="terminal-footer">
              <button
                v-if="upgradeTaskStatus === 'success'"
                class="btn-primary"
                @click="closeUpgradeModal"
                type="button"
              >
                <span>✅ 完成并返回</span>
              </button>
              <button
                v-else-if="upgradeTaskStatus === 'failed'"
                class="btn-secondary"
                @click="upgradeStep = 'form'"
                type="button"
              >
                <span>🔄 返回修改参数并重试</span>
              </button>
              <span v-else class="deploy-progress-hint">
                推流升级中，请勿关闭弹窗...
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Web 交互式 SSH 控制台组件 -->
  <WebTerminal
    v-model="showTerminal"
    :node="terminalNode"
    :cached-master-password="cachedMasterPassword"
    @authenticated="handleAuthenticated"
  />
</template>

<script setup lang="ts">
import { ref, reactive, watch, nextTick, onUnmounted, computed } from 'vue'
import api, { getSecretPath } from '../api'
import { useNodeContext, NodeItem } from '../composables/useNodeContext'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'
import { getToken } from '../utils/auth'
import VultrProvisioner from './VultrProvisioner.vue'
import WebTerminal from './WebTerminal.vue'

const props = defineProps<{
  modelValue: boolean
  initialTab?: 'nodes' | 'cloud' | 'deploy' | 'import'
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'task-status-change', info: { running: boolean; name: string; tab: 'cloud' | 'deploy' }): void
}>()

const { toast } = useToast()
const { showConfirm } = useConfirm()
const {
  activeNodeId,
  nodes,
  fetchNodes,
  switchNode,
} = useNodeContext()

const activeTab = ref<'nodes' | 'cloud' | 'deploy' | 'import'>('nodes')

// 云端创建/部署任务状态监听
const vultrTaskInfo = reactive({
  running: false,
  name: '',
  tab: 'cloud' as const,
})

function handleVultrTaskChange(info: { running: boolean; name: string; tab: 'cloud' }) {
  vultrTaskInfo.running = info.running
  vultrTaskInfo.name = info.name
  vultrTaskInfo.tab = info.tab
}

// ==================== SSH 远程一键部署状态定义 (置于 activeRunningTask 之前，防 TDZ) ====================
const deployStep = ref<'form' | 'terminal'>('form')
const isDeploying = ref(false)
const deployReq = reactive({
  name: '',
  sub_prefix: '',
  host: '',
  ssh_port: 22,
  ssh_user: 'root',
  auth_type: 'password' as 'password' | 'key',
  ssh_password: '',
  ssh_private_key: '',
  panel_port: 20888,
  admin_username: 'admin',
  admin_password: '',
  secret_path: '',
  enable_tls: true,
})

const deployTaskStatus = ref<'running' | 'success' | 'failed'>('running')
const deployLogs = ref<string[]>([])
const terminalBodyRef = ref<HTMLElement | null>(null)
let eventSource: EventSource | null = null
let deployedNodeId = 0

const isSSHDeployRunning = computed(() => deployStep.value === 'terminal' && deployTaskStatus.value === 'running')

const activeRunningTask = computed(() => {
  if (isSSHDeployRunning.value) {
    return {
      running: true,
      name: deployReq.name || deployReq.host || 'SSH 部署目标',
      tab: 'deploy' as const,
    }
  }
  if (vultrTaskInfo.running) {
    return {
      running: true,
      name: vultrTaskInfo.name || 'Vultr 云主机',
      tab: 'cloud' as const,
    }
  }
  return {
    running: false,
    name: '',
    tab: 'cloud' as const,
  }
})

watch(
  activeRunningTask,
  (val) => {
    emit('task-status-change', val)
  },
  { deep: true, immediate: true }
)

function handleOverlayClick() {
  if (activeRunningTask.value.running || activeTab.value !== 'nodes') {
    toast.info('当前正在配置或纳管中，若需关闭请点击右上角「✕」')
    return
  }
  close()
}

function switchToDeployTab() {
  if (deployStep.value === 'terminal' && deployTaskStatus.value === 'success') {
    resetDeployForm()
  }
  activeTab.value = 'deploy'
}

watch(
  () => props.initialTab,
  (val) => {
    if (val) {
      if (activeRunningTask.value.running && val === 'nodes') {
        return
      }
      if (val === 'deploy' && deployStep.value === 'terminal' && deployTaskStatus.value === 'success') {
        resetDeployForm()
      }
      activeTab.value = val
    }
  },
  { immediate: true }
)

watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      if (props.initialTab && !activeRunningTask.value.running) {
        if (props.initialTab === 'deploy' && deployStep.value === 'terminal' && deployTaskStatus.value === 'success') {
          resetDeployForm()
        }
        activeTab.value = props.initialTab
      }
      fetchNodes()
    }
  }
)

function close() {
  emit('update:modelValue', false)
}

function handleSwitch(id: number) {
  switchNode(id)
  toast.success('已切换至目标节点管理视角')
  close()
}

// 节点测速
const pingingMap = reactive<Record<number, boolean>>({})

async function handlePing(id: number) {
  pingingMap[id] = true
  try {
    const res = await api.post(`/nodes/${id}/ping`)
    if (res.data.status === 'online') {
      toast.success(`测速成功：网络延迟 ${res.data.latency_ms} ms`)
    } else {
      toast.warning('节点离线或未能连通')
    }
    await fetchNodes()
  } catch (err: any) {
    toast.error('测速失败: ' + (err.response?.data?.error || err.message))
  } finally {
    pingingMap[id] = false
  }
}

// ==================== 云端 VPS 创建完成回调 ====================
async function handleCloudDeployed(nodeId: number) {
  await fetchNodes()
  activeTab.value = 'nodes'
  if (nodeId) {
    switchNode(nodeId)
    toast.success('云服务器已创建并完成纳管，已切换管理视角')
    close()
  }
}

// ==================== 云节点销毁与移出 ====================
const cloudDeleteModalVisible = ref(false)
const targetDeleteNode = ref<NodeItem | null>(null)
const destroyCloudInstance = ref(true)
const deleteMasterPassword = ref('')
const deletingNode = ref(false)

function closeCloudDeleteModal() {
  cloudDeleteModalVisible.value = false
  targetDeleteNode.value = null
  deleteMasterPassword.value = ''
}

async function submitCloudDelete() {
  if (!targetDeleteNode.value) return
  if (destroyCloudInstance.value && !deleteMasterPassword.value) {
    toast.warning('请输入主控管理员密码')
    return
  }

  deletingNode.value = true
  try {
    const nodeId = targetDeleteNode.value.id
    await api.delete(`/nodes/${nodeId}`, {
      params: {
        destroy_cloud: destroyCloudInstance.value,
        master_password: deleteMasterPassword.value,
      },
    })

    toast.success(
      destroyCloudInstance.value
        ? '云服务器已成功销毁并移出集群'
        : '节点已成功解除纳管'
    )
    if (activeNodeId.value === nodeId) {
      switchNode(1)
    }
    closeCloudDeleteModal()
    await fetchNodes()
  } catch (err: any) {
    toast.error('操作失败: ' + (err.response?.data?.error || err.message))
  } finally {
    deletingNode.value = false
  }
}

// 删除节点
async function handleDelete(item: NodeItem) {
  if (item.cloud_provider === 'vultr') {
    targetDeleteNode.value = item
    destroyCloudInstance.value = true
    deleteMasterPassword.value = ''
    cloudDeleteModalVisible.value = true
    return
  }

  const ok = await showConfirm({
    title: '移出集群确认',
    message: `确定要将节点「${item.name}」(${item.host}) 移出本主控集群吗？（此操作仅解除纳管，不会删除远端服务器上的数据）`,
    confirmText: '移出集群',
    danger: true,
  })
  if (!ok) return

  try {
    await api.delete(`/nodes/${item.id}`)
    toast.success('节点已成功移出集群')
    if (activeNodeId.value === item.id) {
      switchNode(1)
    }
    await fetchNodes()
  } catch (err: any) {
    toast.error('操作失败: ' + (err.response?.data?.error || err.message))
  }
}

// ==================== SSH 远程一键部署逻辑 ====================
function getLineClass(line: string) {
  if (line.includes('[SUCCESS]')) return 'line-success'
  if (line.includes('[ERROR]')) return 'line-error'
  if (line.includes('SSH') || line.includes('Systemd')) return 'line-highlight'
  return ''
}

function scrollToBottom() {
  nextTick(() => {
    if (terminalBodyRef.value) {
      terminalBodyRef.value.scrollTop = terminalBodyRef.value.scrollHeight
    }
  })
}

async function startDeploy() {
  if (!deployReq.host) {
    toast.warning('请输入目标服务器 IP 或域名')
    return
  }
  if (deployReq.auth_type === 'password' && !deployReq.ssh_password) {
    toast.warning('请输入 SSH 登录密码')
    return
  }
  if (deployReq.auth_type === 'key' && !deployReq.ssh_private_key) {
    toast.warning('请输入 SSH 私钥内容')
    return
  }

  isDeploying.value = true
  deployLogs.value = []
  deployTaskStatus.value = 'running'
  deployStep.value = 'terminal'

  try {
    const res = await api.post('/nodes/deploy', deployReq)
    const taskId = res.data.task_id

    // 建立 SSE 日志连接
    const secret = getSecretPath()
    const token = getToken() || ''
    const streamURL = `/${secret}/api/nodes/deploy-logs?task_id=${taskId}&token=${token}`

    if (eventSource) {
      eventSource.close()
    }
    eventSource = new EventSource(streamURL)

    eventSource.onmessage = (event) => {
      const line = event.data
      deployLogs.value.push(line)
      scrollToBottom()

      if (line.includes('[SUCCESS]')) {
        deployTaskStatus.value = 'success'
        eventSource?.close()
        fetchNodes().then(() => {
          const match = nodes.value.find((n) => n.host === deployReq.host)
          if (match) deployedNodeId = match.id
        })
      } else if (line.includes('[ERROR]')) {
        deployTaskStatus.value = 'failed'
        eventSource?.close()
      }
    }

    eventSource.onerror = async () => {
      eventSource?.close()
      eventSource = null
      if (deployTaskStatus.value !== 'running') return

      // SSE 连接发生抖动或关闭时，自动转为高可靠轮询保底，杜绝误判断连
      let pollDone = false
      for (let i = 0; i < 20; i++) {
        await new Promise((r) => setTimeout(r, 1500))
        if (deployTaskStatus.value !== 'running') return
        try {
          const statusRes = await api.get(`/nodes/deploy-status?task_id=${taskId}`)
          const data = statusRes.data
          if (data && Array.isArray(data.logs)) {
            if (data.logs.length > deployLogs.value.length) {
              deployLogs.value = data.logs
              scrollToBottom()
            }
          }
          if (data.status === 'success') {
            deployTaskStatus.value = 'success'
            fetchNodes().then(() => {
              const match = nodes.value.find((n) => n.host === deployReq.host)
              if (match) deployedNodeId = match.id
            })
            pollDone = true
            return
          } else if (data.status === 'failed') {
            deployTaskStatus.value = 'failed'
            if (data.error && !deployLogs.value.some((l: string) => l.includes(data.error))) {
              deployLogs.value.push(`[ERROR] ${data.error}`)
              scrollToBottom()
            }
            pollDone = true
            return
          }
        } catch {
          // 忽略单次网络抖动继续重试
        }
      }
      if (!pollDone && deployTaskStatus.value === 'running') {
        deployTaskStatus.value = 'failed'
        deployLogs.value.push('[ERROR] 日志流连接中断且状态轮询超时')
        scrollToBottom()
      }
    }
  } catch (err: any) {
    deployTaskStatus.value = 'failed'
    deployLogs.value.push('[ERROR] 启动部署任务失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isDeploying.value = false
  }
}

function resetDeployForm() {
  deployStep.value = 'form'
  deployTaskStatus.value = 'running'
  deployLogs.value = []
  deployedNodeId = 0
  deployReq.name = ''
  deployReq.sub_prefix = ''
  deployReq.host = ''
  deployReq.ssh_password = ''
  deployReq.ssh_private_key = ''
  deployReq.admin_password = ''
  deployReq.secret_path = ''
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
}

function finishDeployAndSwitch() {
  const targetId = deployedNodeId
  resetDeployForm()
  if (targetId > 0) {
    switchNode(targetId)
    toast.success('部署完成，已切换至新节点视角！')
  } else {
    toast.success('部署完成！')
  }
  close()
}

// ==================== 纳管已有节点 ====================
const isImporting = ref(false)
const importReq = reactive({
  name: '',
  sub_prefix: '',
  host: '',
  panel_port: 20888,
  secret_path: '',
  username: 'admin',
  password: '',
  use_tls: false,
  cert_fingerprint: '',
})

async function handleImport() {
  if (!importReq.host || !importReq.secret_path || !importReq.password) {
    toast.warning('请完整填写服务器地址、安全路径及密码')
    return
  }

  isImporting.value = true
  try {
    const res = await api.post('/nodes/import', importReq)
    toast.success('成功纳管远程节点！')
    await fetchNodes()
    if (res.data.node_id) {
      switchNode(res.data.node_id)
    }
    close()
  } catch (err: any) {
    toast.error('纳管失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isImporting.value = false
  }
}

// ==================== Web 控制台 / SSH 终端 ====================
const showTerminal = ref(false)
const terminalNode = ref<any>(null)
const cachedMasterPassword = ref('')

function openTerminalModal(item: any) {
  terminalNode.value = item
  showTerminal.value = true
}

function handleAuthenticated(pass: string) {
  cachedMasterPassword.value = pass
}

// ==================== 二次鉴权查看凭据 (Reveal) ====================
const revealModalVisible = ref(false)
const targetRevealNode = ref<NodeItem | null>(null)
const masterVerifyPassword = ref('')
const verifyingReveal = ref(false)
const revealedData = ref<any>(null)
const showPasswordPlainText = ref(false)
const countdownSeconds = ref(60)
let countdownTimer: any = null

function openRevealModal(item: NodeItem) {
  targetRevealNode.value = item
  masterVerifyPassword.value = ''
  revealedData.value = null
  showPasswordPlainText.value = false
  countdownSeconds.value = 60
  revealModalVisible.value = true
}

function closeRevealModal() {
  revealModalVisible.value = false
  revealedData.value = null
  masterVerifyPassword.value = ''
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

async function submitReveal() {
  if (!masterVerifyPassword.value || !targetRevealNode.value) return
  verifyingReveal.value = true
  try {
    const res = await api.post(`/nodes/${targetRevealNode.value.id}/reveal-credentials`, {
      master_password: masterVerifyPassword.value,
    })
    revealedData.value = res.data
    cachedMasterPassword.value = masterVerifyPassword.value
    toast.success('身份验证通过，凭据已解密')

    // 开启 60 秒倒计时自动锁定
    countdownSeconds.value = 60
    if (countdownTimer) clearInterval(countdownTimer)
    countdownTimer = setInterval(() => {
      countdownSeconds.value--
      if (countdownSeconds.value <= 0) {
        clearInterval(countdownTimer)
        countdownTimer = null
        closeRevealModal()
        toast.info('安全时效已到，节点凭据已自动重新锁定')
      }
    }, 1000)
  } catch (err: any) {
    toast.error(err.response?.data?.error || '主控管理员密码错误，验证失败')
  } finally {
    verifyingReveal.value = false
  }
}

function copyText(text: string, label: string) {
  copyToClipboard(text, label)
}

// ==================== 定制节点订阅前缀 (Sub Prefix) ====================
const prefixModalVisible = ref(false)
const targetPrefixNode = ref<NodeItem | null>(null)
const editingNodeName = ref('')
const editingNodePrefix = ref('')
const savingPrefix = ref(false)

const regionPresets = [
  { emoji: '🇭🇰', name: '香港', full: '🇭🇰 香港' },
  { emoji: '🇯🇵', name: '日本', full: '🇯🇵 日本' },
  { emoji: '🇸🇬', name: '新加坡', full: '🇸🇬 新加坡' },
  { emoji: '🇺🇸', name: '美国', full: '🇺🇸 美国' },
  { emoji: '🇰🇷', name: '韩国', full: '🇰🇷 韩国' },
  { emoji: '🇹🇼', name: '台湾', full: '🇹🇼 台湾' },
  { emoji: '🇬🇧', name: '英国', full: '🇬🇧 英国' },
  { emoji: '🇩🇪', name: '德国', full: '🇩🇪 德国' },
]

function openPrefixModal(item: NodeItem) {
  targetPrefixNode.value = item
  editingNodeName.value = item.name
  editingNodePrefix.value = item.sub_prefix || ''
  prefixModalVisible.value = true
}

function closePrefixModal() {
  prefixModalVisible.value = false
  targetPrefixNode.value = null
}

function applyPrefixPreset(p: { emoji: string; name: string; full: string }) {
  if (targetPrefixNode.value?.is_master) {
    editingNodePrefix.value = `${p.full}主控`
  } else {
    editingNodePrefix.value = `${p.full}`
  }
}

const currentEffectivePrefix = computed(() => {
  const p = editingNodePrefix.value.trim()
  if (p) return p
  const n = editingNodeName.value.trim()
  if (n) return n
  return '节点'
})

const protocolPreviews = computed(() => {
  const p = currentEffectivePrefix.value
  return [
    { label: 'Reality 节点', name: `${p} | vless-reality-443` },
    { label: 'Hysteria 2 极速', name: `${p} | hy2-in-8443` },
    { label: 'Shadowsocks', name: `${p} | ss-in-8388` },
    { label: 'Trojan 节点', name: `${p} | trojan-in-443` },
    { label: 'TUIC v5 节点', name: `${p} | tuic-in-8443` },
    { label: 'VMess 节点', name: `${p} | vmess-ws-8080` },
    { label: 'SOCKS5 / Mixed', name: `${p} | socks-in-1080` },
    { label: 'WS 优选 (Argo)', name: `${p} | vless-ws-Cloudflare优选` },
  ]
})

const previewRealityName = computed(() => {
  return `${currentEffectivePrefix.value} | vless-reality-443`
})

const previewWSName = computed(() => {
  return `${currentEffectivePrefix.value} | vless-ws-Cloudflare优选`
})

async function saveNodePrefix() {
  if (!targetPrefixNode.value) return
  if (!editingNodeName.value.trim()) {
    toast.warning('节点友好名称不能为空')
    return
  }

  savingPrefix.value = true
  try {
    await api.put(`/nodes/${targetPrefixNode.value.id}`, {
      name: editingNodeName.value.trim(),
      sub_prefix: editingNodePrefix.value.trim(),
    })
    toast.success('节点名称与订阅前缀已成功更新！')
    closePrefixModal()
    await fetchNodes()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  } finally {
    savingPrefix.value = false
  }
}

// ==================== 远程节点升级逻辑 ====================
const upgradeModalVisible = ref(false)
const targetUpgradeNode = ref<NodeItem | null>(null)
const upgradeStep = ref<'form' | 'terminal'>('form')
const isUpgrading = ref(false)
const upgradeTaskStatus = ref<'running' | 'success' | 'failed'>('running')
const upgradeLogs = ref<string[]>([])
const upgradeTerminalBodyRef = ref<HTMLElement | null>(null)
const availablePackages = ref<any[]>([])
let upgradeEventSource: EventSource | null = null

const upgradeReq = reactive({
  node_id: 0,
  mode: 'api' as 'api' | 'ssh',
  package_id: '',
  host: '',
  ssh_port: 22,
  ssh_user: 'root',
  auth_type: 'password' as 'password' | 'key',
  ssh_password: '',
  ssh_private_key: '',
  force: false,
})

async function openUpgradeModal(item: NodeItem) {
  targetUpgradeNode.value = item
  upgradeReq.node_id = item.id
  upgradeReq.mode = 'api'
  upgradeReq.package_id = ''
  upgradeReq.host = item.host || ''
  upgradeReq.ssh_port = item.ssh_port || 22
  upgradeReq.ssh_user = 'root'
  upgradeReq.auth_type = 'password'
  upgradeReq.ssh_password = ''
  upgradeReq.ssh_private_key = ''
  upgradeReq.force = false
  upgradeStep.value = 'form'
  upgradeLogs.value = []
  upgradeTaskStatus.value = 'running'
  upgradeModalVisible.value = true

  try {
    const res = await api.get('/system/upgrade/packages')
    availablePackages.value = res.data || []
  } catch {
    availablePackages.value = []
  }
}

function closeUpgradeModal() {
  if (isUpgrading.value && upgradeTaskStatus.value === 'running') {
    return
  }
  upgradeModalVisible.value = false
  targetUpgradeNode.value = null
  if (upgradeEventSource) {
    upgradeEventSource.close()
    upgradeEventSource = null
  }
  fetchNodes()
}

function scrollUpgradeToBottom() {
  nextTick(() => {
    if (upgradeTerminalBodyRef.value) {
      upgradeTerminalBodyRef.value.scrollTop = upgradeTerminalBodyRef.value.scrollHeight
    }
  })
}

async function startUpgrade() {
  if (upgradeReq.mode === 'ssh') {
    if (upgradeReq.auth_type === 'password' && !upgradeReq.ssh_password) {
      toast.warning('请输入 SSH 登录密码')
      return
    }
    if (upgradeReq.auth_type === 'key' && !upgradeReq.ssh_private_key) {
      toast.warning('请输入 SSH 私钥内容')
      return
    }
  }

  isUpgrading.value = true
  upgradeLogs.value = []
  upgradeTaskStatus.value = 'running'
  upgradeStep.value = 'terminal'

  try {
    const res = await api.post('/nodes/upgrade', upgradeReq)
    const taskId = res.data.task_id

    const secret = getSecretPath()
    const token = getToken() || ''
    const streamURL = `/${secret}/api/nodes/deploy-logs?task_id=${taskId}&token=${token}`

    if (upgradeEventSource) {
      upgradeEventSource.close()
    }
    upgradeEventSource = new EventSource(streamURL)

    upgradeEventSource.onmessage = (event) => {
      const line = event.data
      upgradeLogs.value.push(line)
      scrollUpgradeToBottom()

      if (line.includes('[SUCCESS]')) {
        upgradeTaskStatus.value = 'success'
        upgradeEventSource?.close()
        fetchNodes()
      } else if (line.includes('[ERROR]')) {
        upgradeTaskStatus.value = 'failed'
        upgradeEventSource?.close()
      }
    }

    upgradeEventSource.onerror = async () => {
      upgradeEventSource?.close()
      upgradeEventSource = null
      if (upgradeTaskStatus.value !== 'running') return

      // SSE 连接异常中断时，自动启动状态轮询兜底，杜绝假死与误报失败
      let pollDone = false
      for (let i = 0; i < 20; i++) {
        await new Promise((r) => setTimeout(r, 1500))
        if (upgradeTaskStatus.value !== 'running') return
        try {
          const statusRes = await api.get(`/nodes/deploy-status?task_id=${taskId}`)
          const data = statusRes.data
          if (data && Array.isArray(data.logs)) {
            if (data.logs.length > upgradeLogs.value.length) {
              upgradeLogs.value = data.logs
              scrollUpgradeToBottom()
            }
          }
          if (data.status === 'success') {
            upgradeTaskStatus.value = 'success'
            fetchNodes()
            pollDone = true
            return
          } else if (data.status === 'failed') {
            upgradeTaskStatus.value = 'failed'
            if (data.error && !upgradeLogs.value.some((l: string) => l.includes(data.error))) {
              upgradeLogs.value.push(`[ERROR] ${data.error}`)
              scrollUpgradeToBottom()
            }
            pollDone = true
            return
          }
        } catch {
          // 忽略单次网络抖动继续重试
        }
      }
      if (!pollDone && upgradeTaskStatus.value === 'running') {
        upgradeTaskStatus.value = 'failed'
        upgradeLogs.value.push('[ERROR] 日志流连接中断且状态轮询超时')
        scrollUpgradeToBottom()
      }
    }
  } catch (err: any) {
    upgradeTaskStatus.value = 'failed'
    upgradeLogs.value.push('[ERROR] 启动升级任务失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isUpgrading.value = false
  }
}

onUnmounted(() => {
  if (eventSource) eventSource.close()
  if (upgradeEventSource) upgradeEventSource.close()
  if (countdownTimer) clearInterval(countdownTimer)
})
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  backdrop-filter: blur(6px);
  z-index: 999;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
}

.cluster-modal {
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 1rem;
  width: 100%;
  max-width: 820px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid #1e293b;
  background: #1e293b;
}

.header-title-box {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.header-icon {
  font-size: 1.75rem;
}

.header-title {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 600;
  color: #f8fafc;
}

.header-desc {
  margin: 0.15rem 0 0 0;
  font-size: 0.75rem;
  color: #94a3b8;
}

.close-btn {
  background: transparent;
  border: none;
  color: #94a3b8;
  font-size: 1.25rem;
  cursor: pointer;
  padding: 0.25rem 0.5rem;
  border-radius: 0.35rem;
  transition: all 0.15s ease;
}

.close-btn:hover {
  background: #334155;
  color: #ffffff;
}

/* Tabs */
.cluster-tabs {
  display: flex;
  gap: 0.5rem;
  padding: 0.75rem 1.5rem;
  background: #0b1120;
  border-bottom: 1px solid #1e293b;
}

.tab-btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  background: transparent;
  border: none;
  color: #94a3b8;
  padding: 0.45rem 0.85rem;
  border-radius: 0.5rem;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tab-btn:hover {
  color: #f1f5f9;
  background: #1e293b;
}

.tab-btn-active {
  background: #1e293b;
  color: #818cf8;
  font-weight: 600;
}

.tab-badge {
  font-size: 0.7rem;
  background: rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
  padding: 0.05rem 0.35rem;
  border-radius: 9999px;
}

.tab-content {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
}

/* 节点卡片列表 */
.nodes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 1rem;
}

.node-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.75rem;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  transition: all 0.2s ease;
}

.node-card:hover {
  border-color: #475569;
}

.node-card-active {
  border-color: #6366f1;
  background: rgba(99, 102, 241, 0.08);
}

.node-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.node-title-group {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.node-badge-icon {
  font-size: 1.35rem;
}

.node-name-row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.card-name {
  font-size: 0.95rem;
  font-weight: 600;
  color: #f1f5f9;
}

.tag-master {
  font-size: 0.625rem;
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
  padding: 0.05rem 0.3rem;
  border-radius: 0.25rem;
}

.tag-current {
  font-size: 0.625rem;
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
  padding: 0.05rem 0.3rem;
  border-radius: 0.25rem;
}

.tag-tls {
  font-size: 0.625rem;
  background: rgba(168, 85, 247, 0.2);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.35);
  padding: 0.05rem 0.35rem;
  border-radius: 0.25rem;
  font-weight: 500;
}

.text-fingerprint {
  font-size: 0.6875rem;
  color: #a5b4fc;
}

.checkbox-container {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.checkbox-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: #f1f5f9;
}

.field-hint {
  font-size: 0.72rem;
  color: #94a3b8;
  line-height: 1.45;
  margin-top: 0.25rem;
}

.card-host {
  font-size: 0.75rem;
  color: #94a3b8;
  font-family: ui-monospace, monospace;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.7rem;
  padding: 0.15rem 0.5rem;
  border-radius: 9999px;
  font-family: ui-monospace, monospace;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-online {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
}
.status-online .status-dot {
  background: #10b981;
}

.status-offline {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}
.status-offline .status-dot {
  background: #ef4444;
}

.node-props {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  background: rgba(15, 23, 42, 0.6);
  padding: 0.5rem 0.75rem;
  border-radius: 0.45rem;
  font-size: 0.75rem;
}

.prop-item {
  display: flex;
  justify-content: space-between;
}

.prop-label {
  color: #64748b;
}

.prop-val {
  color: #cbd5e1;
}

.mono {
  font-family: ui-monospace, monospace;
}

.node-card-actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-top: auto;
}

.btn-action {
  padding: 0.35rem 0.65rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  border: 1px solid transparent;
}

.btn-switch {
  background: #4f46e5;
  color: #ffffff;
}
.btn-switch:hover {
  background: #4338ca;
}

.active-notice {
  font-size: 0.75rem;
  color: #10b981;
  font-weight: 500;
  flex: 1;
}

.btn-ping {
  background: #334155;
  color: #cbd5e1;
}
.btn-ping:hover {
  background: #475569;
}

.btn-terminal {
  background: rgba(56, 189, 248, 0.15);
  color: #38bdf8;
  border-color: rgba(56, 189, 248, 0.35);
}
.btn-terminal:hover {
  background: rgba(56, 189, 248, 0.28);
  color: #7dd3fc;
  border-color: #38bdf8;
}

.btn-prefix {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border-color: rgba(16, 185, 129, 0.3);
}
.btn-prefix:hover {
  background: rgba(16, 185, 129, 0.25);
}

.btn-reveal {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
  border-color: rgba(245, 158, 11, 0.3);
}
.btn-reveal:hover {
  background: rgba(245, 158, 11, 0.25);
}

.btn-upgrade {
  background: rgba(14, 165, 233, 0.15);
  color: #38bdf8;
  border-color: rgba(14, 165, 233, 0.35);
}
.btn-upgrade:hover {
  background: rgba(14, 165, 233, 0.28);
  border-color: #38bdf8;
  color: #e0f2fe;
}

.preset-pill-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.preset-pill-btn {
  background: rgba(51, 65, 85, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.2);
  color: #e2e8f0;
  border-radius: 9999px;
  padding: 0.2rem 0.55rem;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.preset-pill-btn:hover {
  background: rgba(16, 185, 129, 0.2);
  border-color: rgba(52, 211, 153, 0.5);
  color: #6ee7b7;
  transform: translateY(-1px);
}

.preview-box {
  background: rgba(15, 23, 42, 0.75);
  border: 1px solid rgba(51, 65, 85, 0.6);
  border-radius: 0.5rem;
  padding: 0.75rem;
}

.preview-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.78rem;
  margin-top: 0.25rem;
}

.preview-label {
  color: #94a3b8;
  font-size: 0.75rem;
  min-width: 5.5rem;
}

.preview-code {
  color: #38bdf8;
  background: rgba(0, 0, 0, 0.3);
  padding: 0.15rem 0.4rem;
  border-radius: 0.25rem;
  font-size: 0.75rem;
}

.btn-delete {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
  border-color: rgba(239, 68, 68, 0.3);
}
.btn-delete:hover {
  background: rgba(239, 68, 68, 0.25);
}

/* 部署表单样式 */
.deploy-form {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.form-alert-info {
  display: flex;
  gap: 0.75rem;
  background: rgba(59, 130, 246, 0.12);
  border: 1px solid rgba(59, 130, 246, 0.3);
  padding: 0.75rem 1rem;
  border-radius: 0.5rem;
  font-size: 0.8125rem;
  color: #93c5fd;
  line-height: 1.45;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.85rem;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.full-width {
  grid-column: 1 / -1;
}

.form-group label {
  font-size: 0.75rem;
  font-weight: 500;
  color: #cbd5e1;
}

.req {
  color: #f87171;
}

.form-group input,
.form-group textarea {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.55rem 0.75rem;
  color: #f8fafc;
  font-size: 0.8125rem;
  outline: none;
  transition: border-color 0.15s ease;
}

.form-group input:focus,
.form-group textarea:focus {
  border-color: #6366f1;
}

.radio-group {
  display: flex;
  gap: 1.5rem;
  padding: 0.35rem 0;
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.8125rem;
  color: #cbd5e1;
  cursor: pointer;
}

.form-divider {
  display: flex;
  align-items: center;
  margin: 0.5rem 0;
  color: #64748b;
  font-size: 0.75rem;
  font-weight: 600;
}
.form-divider::before,
.form-divider::after {
  content: '';
  flex: 1;
  border-bottom: 1px solid #1e293b;
}
.form-divider span {
  padding: 0 0.75rem;
}

.form-footer {
  display: flex;
  justify-content: flex-end;
  padding-top: 0.5rem;
}

.btn-primary {
  background: #4f46e5;
  color: #ffffff;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s ease;
}
.btn-primary:hover:not(:disabled) {
  background: #4338ca;
}
.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-secondary {
  background: #334155;
  color: #cbd5e1;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.15s ease;
}
.btn-secondary:hover {
  background: #475569;
}

/* 终端输出面板 */
.deploy-terminal-box {
  display: flex;
  flex-direction: column;
  background: #020617;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  overflow: hidden;
  height: 480px;
}

.terminal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.65rem 1rem;
  background: #0f172a;
  border-bottom: 1px solid #1e293b;
}

.terminal-status-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.pulse-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.pulse-active {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
  animation: pulseAnim 1.5s infinite;
}

@keyframes pulseAnim {
  0% { opacity: 0.4; }
  50% { opacity: 1; }
  100% { opacity: 0.4; }
}

.terminal-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: #f1f5f9;
}

.terminal-node-name {
  font-size: 0.75rem;
  color: #64748b;
  font-family: ui-monospace, monospace;
}

.terminal-body {
  flex: 1;
  padding: 0.85rem;
  overflow-y: auto;
  font-family: ui-monospace, monospace;
  font-size: 0.75rem;
  line-height: 1.6;
  color: #94a3b8;
}

.terminal-line {
  white-space: pre-wrap;
  word-break: break-all;
}

.line-success {
  color: #34d399;
  font-weight: 600;
}

.line-error {
  color: #f87171;
  font-weight: 600;
}

.line-highlight {
  color: #93c5fd;
}

.terminal-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  padding: 0.75rem 1rem;
  background: #0f172a;
  border-top: 1px solid #1e293b;
}

.deploy-progress-hint {
  font-size: 0.75rem;
  color: #64748b;
}

/* 二次鉴权模态框 (Reveal) */
.reveal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.8);
  backdrop-filter: blur(8px);
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
}

.reveal-card {
  background: #0f172a;
  border: 1px solid #475569;
  border-radius: 0.85rem;
  width: 100%;
  max-width: 520px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.8);
  overflow: hidden;
  animation: modalScale 0.15s ease-out;
}

@keyframes modalScale {
  from { transform: scale(0.96); opacity: 0; }
  to { transform: scale(1); opacity: 1; }
}

.reveal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  background: #1e293b;
  border-bottom: 1px solid #334155;
}

.reveal-title-box {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.reveal-icon {
  font-size: 1.5rem;
}

.reveal-title {
  margin: 0;
  font-size: 1rem;
  color: #f8fafc;
}

.reveal-desc {
  margin: 0.15rem 0 0 0;
  font-size: 0.7rem;
  color: #94a3b8;
}

.reveal-body {
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.reveal-prompt {
  font-size: 0.8125rem;
  color: #cbd5e1;
  line-height: 1.5;
}

.reveal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.credentials-box {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.countdown-bar {
  background: rgba(245, 158, 11, 0.15);
  border: 1px solid rgba(245, 158, 11, 0.3);
  padding: 0.45rem 0.75rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  color: #fbbf24;
  text-align: center;
}

.cred-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.8125rem;
}

.cred-label {
  color: #94a3b8;
  font-size: 0.75rem;
}

.cred-val {
  color: #f1f5f9;
  font-weight: 500;
}

.cred-val-copy {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  color: #f1f5f9;
}

.cred-link {
  color: #818cf8;
  text-decoration: underline;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.btn-mini-toggle {
  background: #334155;
  border: none;
  color: #cbd5e1;
  font-size: 0.675rem;
  padding: 0.15rem 0.4rem;
  border-radius: 0.25rem;
  cursor: pointer;
}

.btn-mini-copy {
  background: rgba(99, 102, 241, 0.2);
  border: none;
  color: #a5b4fc;
  font-size: 0.675rem;
  padding: 0.15rem 0.45rem;
  border-radius: 0.25rem;
  cursor: pointer;
}
.btn-mini-copy:hover {
  background: rgba(99, 102, 241, 0.35);
  color: #ffffff;
}

.tag-vultr {
  font-size: 0.625rem;
  background: rgba(14, 165, 233, 0.2);
  color: #38bdf8;
  border: 1px solid rgba(14, 165, 233, 0.35);
  padding: 0.05rem 0.35rem;
  border-radius: 0.25rem;
  font-weight: 500;
}

.btn-danger-strong {
  background: #dc2626;
  color: #ffffff;
  border-color: #ef4444;
}
.btn-danger-strong:hover:not(:disabled) {
  background: #b91c1c;
}
</style>

