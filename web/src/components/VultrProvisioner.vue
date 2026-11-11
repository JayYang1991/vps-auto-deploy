<template>
  <div class="vultr-provisioner">
    <!-- 1. Vultr Token 状态栏 / 首次绑定卡片 -->
    <div v-if="!vultrConfig.configured || showUpdateToken" class="token-setup-card">
      <div class="setup-header">
        <div class="setup-icon">🔑</div>
        <div>
          <h4 class="setup-title">{{ vultrConfig.configured ? '更换 Vultr API Token' : '绑定 Vultr API Token' }}</h4>
          <p class="setup-desc">
            请前往 Vultr 控制台 (Account ➔ API) 生成具备 <strong>All Access</strong> 权限的 Personal Access Token
          </p>
        </div>
      </div>

      <div class="token-input-row">
        <input
          v-model="tokenInput"
          :type="showTokenPlain ? 'text' : 'password'"
          placeholder="在此粘贴您的 Vultr API Key (例如：42CHARACTERSLONGVULTRAPITOKEN...)"
          class="token-input mono"
        />
        <button class="btn-toggle-eye" @click="showTokenPlain = !showTokenPlain" type="button">
          {{ showTokenPlain ? '🙈' : '👁️' }}
        </button>
        <button class="btn-primary" @click="saveToken" :disabled="savingToken || !tokenInput.trim()">
          <span>{{ savingToken ? '⏳ 验证中...' : '🔗 验证并加密绑定' }}</span>
        </button>
        <button v-if="vultrConfig.configured" class="btn-secondary" @click="showUpdateToken = false">
          取消
        </button>
      </div>

      <div v-if="tokenError" class="token-error-msg">
        ⚠️ {{ tokenError }}
      </div>
    </div>

    <!-- 已经绑定 Token 时的信息胶囊条 -->
    <div v-else class="token-status-bar">
      <div class="status-left">
        <span class="cloud-icon">☁️</span>
        <div class="status-meta">
          <div class="meta-row">
            <span class="provider-badge">Vultr API</span>
            <span class="masked-key mono">{{ vultrConfig.masked_key }}</span>
            <span class="active-dot" title="Token 正常可用"></span>
          </div>
          <div v-if="vultrConfig.account" class="balance-text">
            <span>账户: <strong>{{ vultrConfig.account.email }}</strong></span>
            <span class="dot-divider">·</span>
            <span>可用余额: <strong class="color-success">${{ Math.abs(vultrConfig.account.balance).toFixed(2) }}</strong></span>
            <span class="dot-divider">·</span>
            <span>本月已发生: ${{ vultrConfig.account.pending_charges.toFixed(2) }}</span>
          </div>
        </div>
      </div>

      <div class="status-right">
        <button class="btn-mini-refresh" @click="refreshCatalog" :disabled="loadingCatalog" title="重新从 Vultr 同步区域与机型库存">
          <span>{{ loadingCatalog ? '🔄 正在同步...' : '🔄 刷新库存' }}</span>
        </button>
        <button class="btn-mini-update" @click="showUpdateToken = true">
          ⚙️ 更换 Token
        </button>
      </div>
    </div>

    <!-- 模式切换胶囊 Tab -->
    <div v-if="vultrConfig.configured && !showUpdateToken && cloudStep === 'form'" class="mode-switch-wrapper">
      <div class="provision-mode-bar">
        <button
          class="mode-pill-btn"
          :class="{ 'mode-pill-active': provisionMode === 'create' }"
          @click="provisionMode = 'create'"
          type="button"
        >
          <span>🚀 购买创建全新 VPS</span>
        </button>

        <button
          class="mode-pill-btn"
          :class="{ 'mode-pill-active': provisionMode === 'existing' }"
          @click="switchToExistingMode"
          type="button"
        >
          <span>📦 纳管已有 Vultr 实例</span>
          <span v-if="vultrInstances.length > 0 && unmanagedInstancesCount > 0" class="mode-badge">
            {{ unmanagedInstancesCount }} 待纳管
          </span>
        </button>
      </div>
    </div>

    <!-- 2. 主流程：创建表单 / 已有实例纳管 vs 实时终端 -->
    <div v-if="cloudStep === 'form'" class="form-wrapper">
      <!-- 模式 A：购买创建全新 VPS -->
      <div v-if="provisionMode === 'create'" class="provision-flow">
      <!-- 步骤 1：选择区域 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">1</div>
          <div>
            <h4 class="section-title">选择数据中心区域 (Region)</h4>
            <span class="section-subtitle">已支持亚太高速、北美核心与欧洲数据中心，带自动国旗匹配</span>
          </div>
        </div>

        <!-- 洲际筛选 Tab -->
        <div class="continent-tabs">
          <button
            v-for="c in continents"
            :key="c.key"
            class="continent-btn"
            :class="{ 'continent-btn-active': selectedContinent === c.key }"
            @click="selectedContinent = c.key"
          >
            {{ c.label }}
          </button>
        </div>

        <!-- 区域卡片网格 -->
        <div v-if="loadingCatalog" class="loading-state">
          <span>⏳ 正在从 Vultr 同步全球数据中心与库存...</span>
        </div>
        <div v-else class="regions-grid">
          <div
            v-for="r in filteredRegions"
            :key="r.id"
            class="region-card"
            :class="{ 'region-card-selected': selectedRegion === r.id }"
            @click="selectRegion(r)"
          >
            <div class="region-flag">{{ r.flag_emoji }}</div>
            <div class="region-info">
              <div class="region-city">{{ r.city_zh }}</div>
              <div class="region-meta">
                <span class="region-id mono">{{ r.id }}</span>
                <span class="region-continent">{{ r.continent }}</span>
              </div>
            </div>
            <div v-if="selectedRegion === r.id" class="check-badge">✓</div>
          </div>
        </div>
      </section>

      <!-- 步骤 2：选择机型规格 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">2</div>
          <div>
            <h4 class="section-title">选择实例规格套餐 (Plan)</h4>
            <span class="section-subtitle">
              已自动根据选中的 <strong>[{{ currentRegionName }}]</strong> 过滤实时可用库存
            </span>
          </div>
        </div>

        <!-- 机型分类 Tab -->
        <div class="plan-type-tabs">
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'all' }"
            @click="planTypeFilter = 'all'"
          >
            全部规格
          </button>
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'vc2' }"
            @click="planTypeFilter = 'vc2'"
          >
            💻 标准计算 (vc2)
          </button>
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'vhf' }"
            @click="planTypeFilter = 'vhf'"
          >
            ⚡ 高频计算 3GHz+ (vhf)
          </button>
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'vhp' }"
            @click="planTypeFilter = 'vhp'"
          >
            🚀 AMD/Intel 高性能 (vhp)
          </button>
        </div>

        <!-- 机型规格网格 -->
        <div class="plans-grid">
          <div
            v-for="p in filteredPlans"
            :key="p.id"
            class="plan-card"
            :class="{
              'plan-card-selected': selectedPlan === p.id,
              'plan-card-disabled': !isPlanInRegion(p)
            }"
            @click="isPlanInRegion(p) && (selectedPlan = p.id)"
          >
            <div class="plan-card-header">
              <span class="plan-id mono">{{ p.id }}</span>
              <span class="plan-price">
                <strong>${{ p.monthly_cost.toFixed(2) }}</strong> <small>/ 月</small>
              </span>
            </div>

            <div class="plan-specs">
              <div class="spec-row">
                <span class="spec-label">核心与内存:</span>
                <span class="spec-val"><strong>{{ p.vcpu_count }} vCPU</strong> · {{ p.ram }} MB</span>
              </div>
              <div class="spec-row">
                <span class="spec-label">高速磁盘:</span>
                <span class="spec-val">{{ p.disk }} GB SSD</span>
              </div>
              <div class="spec-row">
                <span class="spec-label">公网流量:</span>
                <span class="spec-val">{{ (p.bandwidth / 1024).toFixed(1) }} TB</span>
              </div>
            </div>

            <div class="plan-card-footer">
              <span v-if="!isPlanInRegion(p)" class="stock-badge stock-empty">
                该区域无库存
              </span>
              <span v-else-if="selectedPlan === p.id" class="stock-badge stock-selected">
                ✓ 已选择此规格
              </span>
              <span v-else class="stock-badge stock-available">
                有库存
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- 步骤 3：选择操作系统镜像 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">3</div>
          <div>
            <h4 class="section-title">选择操作系统镜像 (OS)</h4>
            <span class="section-subtitle">已自动筛选 64 位原生支持 Systemd 与 BBR 的主流 Linux 镜像</span>
          </div>
        </div>

        <div class="os-family-tabs">
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'debian' }"
            @click="selectOSFamily('debian')"
          >
            <span>🌀 Debian</span>
            <small>推荐 · 低内存</small>
          </button>
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'ubuntu' }"
            @click="selectOSFamily('ubuntu')"
          >
            <span>🟠 Ubuntu LTS</span>
            <small>热门 · 生态好</small>
          </button>
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'rhel' }"
            @click="selectOSFamily('rhel')"
          >
            <span>🟢 Rocky / Alma</span>
            <small>企业级 RHEL</small>
          </button>
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'other' }"
            @click="selectOSFamily('other')"
          >
            <span>🔵 Fedora / Arch</span>
            <small>新内核</small>
          </button>
        </div>

        <div class="os-version-selector">
          <label>具体系统版本：</label>
          <select v-model="selectedOSID" class="os-select mono">
            <option v-for="o in filteredOSList" :key="o.id" :value="o.id">
              {{ o.name }} {{ o.recommended ? '— ⭐ 官方首选推荐' : '' }}
            </option>
          </select>
        </div>
      </section>

      <!-- 步骤 4：节点参数与安全选项 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">4</div>
          <div>
            <h4 class="section-title">节点名称与安全增强设置</h4>
            <span class="section-subtitle">配置面板初始参数与防中间人攻击保护</span>
          </div>
        </div>

        <div class="options-grid">
          <div class="form-group">
            <label>节点友好显示名 <span class="req">*</span></label>
            <input v-model="formReq.name" type="text" placeholder="例如：东京 Vultr-01" />
          </div>

          <div class="form-group">
            <label>面板监听端口</label>
            <input v-model.number="formReq.panel_port" type="number" placeholder="20888" />
          </div>

          <div class="form-group full-width">
            <label class="checkbox-container">
              <input type="checkbox" v-model="formReq.enable_ipv6" />
              <span class="checkbox-title">分配公网 IPv6 地址 (免费开启双栈网络)</span>
            </label>
          </div>

          <div class="form-group full-width" style="background: rgba(99, 102, 241, 0.08); padding: 0.75rem; border-radius: 0.5rem; border: 1px dashed rgba(99, 102, 241, 0.3);">
            <label class="checkbox-container">
              <input type="checkbox" v-model="formReq.enable_tls" />
              <span class="checkbox-title">🛡️ 启用自签 TLS 证书与指纹强绑定 (防中间人劫持 · 强烈推荐)</span>
            </label>
            <div class="field-hint">
              新主机启动后将自动生成 ECDSA TLS 证书并在主控端绑定 SHA-256 唯一指纹，杜绝任何公网路由劫持与流量窃听。
            </div>
          </div>
        </div>
      </section>

      <!-- 底部总结栏与开通按钮 -->
      <div class="bottom-action-bar">
        <div class="summary-box">
          <span class="summary-label">配置汇总:</span>
          <span class="summary-value">
            {{ currentRegionFlag }} {{ currentRegionName }} · {{ currentPlanText }} · {{ currentOSText }}
          </span>
          <span class="summary-price">
            预估费用: <strong>${{ currentPlanCost.toFixed(2) }}</strong> / 月
          </span>
        </div>

        <button class="btn-create-submit" @click="startProvision" :disabled="isSubmitting || !canSubmit">
          <span>🚀 立即开通云主机并自动纳管</span>
        </button>
      </div>
    </div>

    <!-- 模式 B：纳管已有 Vultr 实例 -->
    <div v-else class="existing-flow">
      <!-- 头部工具栏 -->
      <div class="existing-toolbar">
        <div class="search-input-box">
          <span class="search-icon">🔍</span>
          <input
            v-model="existingSearchQuery"
            type="text"
            placeholder="按实例名称、IP 地址、区域或操作系统搜索..."
            class="existing-search-input"
          />
        </div>
        <button
          class="btn-refresh-instances"
          @click="loadVultrInstances(true)"
          :disabled="loadingInstances"
          type="button"
        >
          <span>{{ loadingInstances ? '🔄 同步中...' : '🔄 刷新实例列表' }}</span>
        </button>
      </div>

      <!-- 实例列表加载状态 -->
      <div v-if="loadingInstances" class="loading-state">
        <span>⏳ 正在查询您 Vultr 账户下的全部云主机...</span>
      </div>

      <!-- 无实例空状态 -->
      <div v-else-if="filteredInstances.length === 0" class="empty-instances-state">
        <div class="empty-icon">📦</div>
        <div class="empty-text">未检索到匹配的 Vultr 实例</div>
        <div class="empty-sub">您可以切换至「购买创建全新 VPS」开通一台新主机</div>
      </div>

      <!-- 实例卡片列表 -->
      <div v-else class="instances-grid">
        <div
          v-for="inst in filteredInstances"
          :key="inst.id"
          class="instance-card"
          :class="{
            'instance-card-selected': selectedInstance?.id === inst.id,
            'instance-card-managed': inst.is_managed
          }"
          @click="selectExistingInstance(inst)"
        >
          <div class="instance-card-top">
            <div class="inst-region-box">
              <span class="inst-flag">{{ inst.flag_emoji || '🌐' }}</span>
              <span class="inst-city">{{ inst.city_zh || inst.region }}</span>
            </div>
            <span
              v-if="inst.is_managed"
              class="tag-managed"
              title="该实例已被纳管入本集群"
            >
              ✓ 已在集群中
            </span>
            <span
              v-else
              class="tag-unmanaged"
            >
              待纳管
            </span>
          </div>

          <div class="inst-name-row">
            <span class="inst-label" :title="inst.label || inst.id">{{ inst.label || inst.id }}</span>
            <span class="inst-power" :class="inst.power_status === 'running' ? 'power-running' : 'power-stopped'">
              ● {{ inst.power_status === 'running' ? '开机' : '关机' }}
            </span>
          </div>

          <div class="inst-props-list">
            <div class="inst-prop">
              <span class="prop-label">公网 IP:</span>
              <span class="prop-val mono font-highlight">{{ inst.main_ip || '未分配' }}</span>
            </div>
            <div class="inst-prop">
              <span class="prop-label">规格配置:</span>
              <span class="prop-val">{{ inst.vcpu_count }}核 · {{ inst.ram }}MB · {{ inst.plan }}</span>
            </div>
            <div class="inst-prop">
              <span class="prop-label">系统环境:</span>
              <span class="prop-val text-truncate" :title="inst.os">{{ inst.os }}</span>
            </div>
          </div>

          <div class="instance-card-bottom">
            <div class="inst-actions-row">
              <button
                v-if="!inst.is_managed"
                class="btn-select-inst"
                :class="{ 'btn-selected-active': selectedInstance?.id === inst.id }"
                type="button"
              >
                {{ selectedInstance?.id === inst.id ? '✓ 当前已选中' : '选择纳管 ➔' }}
              </button>
              <span v-else class="managed-note">关联节点：{{ inst.node_name }}</span>

              <button
                class="btn-card-destroy"
                @click.stop="openDestroyModal(inst)"
                type="button"
                title="彻底销毁该 Vultr 云主机并终止扣费"
              >
                🗑️
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 选中实例后的纳管配置面板 -->
      <div v-if="selectedInstance && !selectedInstance.is_managed" class="existing-config-card">
        <div class="config-card-header">
          <span class="config-header-icon">🛠️</span>
          <div>
            <h4 class="config-header-title">配置纳管：{{ selectedInstance.label || selectedInstance.id }}</h4>
            <span class="config-header-desc">
              公网 IP: <strong>{{ selectedInstance.main_ip }}</strong> · {{ selectedInstance.city_zh }} · {{ selectedInstance.os }}
            </span>
          </div>
        </div>

        <div class="config-form-grid">
          <div class="form-group">
            <label>节点显示名称 <span class="req">*</span></label>
            <input v-model="existingForm.name" type="text" placeholder="例如：东京 Vultr-01" />
          </div>

          <div class="form-group">
            <label>面板监听端口</label>
            <input v-model.number="existingForm.panel_port" type="number" placeholder="20888" />
          </div>

          <!-- SSH 认证方式选择 -->
          <div class="form-group full-width">
            <label>SSH 接入认证方式</label>
            <div class="radio-group">
              <label class="radio-label">
                <input type="radio" value="master_key" v-model="existingForm.auth_type" />
                <span>🔑 主控托管部署密钥 (推荐，若创建时已绑定)</span>
              </label>
              <label class="radio-label">
                <input type="radio" value="password" v-model="existingForm.auth_type" />
                <span>🔒 Root 密码认证</span>
              </label>
              <label class="radio-label">
                <input type="radio" value="key" v-model="existingForm.auth_type" />
                <span>📝 自定义 SSH 私钥</span>
              </label>
            </div>
          </div>

          <div v-if="existingForm.auth_type === 'password'" class="form-group full-width">
            <div class="label-with-action">
              <label>Root 密码 <span class="req">*</span></label>
              <button
                v-if="selectedInstance.default_password && existingForm.ssh_password !== selectedInstance.default_password"
                class="btn-fill-default-pwd"
                @click="existingForm.ssh_password = selectedInstance.default_password"
                type="button"
              >
                ⚡ 填入 Vultr 初始 root 密码
              </button>
            </div>
            <input
              v-model="existingForm.ssh_password"
              type="password"
              placeholder="请输入目标主机的 root 密码"
            />
          </div>

          <div v-else-if="existingForm.auth_type === 'key'" class="form-group full-width">
            <label>SSH 私钥内容 (OpenSSH PEM) <span class="req">*</span></label>
            <textarea
              v-model="existingForm.ssh_private_key"
              rows="4"
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
              class="mono"
            ></textarea>
          </div>

          <div class="form-group full-width" style="margin-top: 0.5rem; background: rgba(99, 102, 241, 0.08); padding: 0.75rem; border-radius: 0.5rem; border: 1px dashed rgba(99, 102, 241, 0.3);">
            <label class="checkbox-container">
              <input type="checkbox" v-model="existingForm.enable_tls" />
              <span class="checkbox-title">🛡️ 启用自签名 TLS 证书与指纹强绑定 (强烈推荐 · 防中间人劫持)</span>
            </label>
            <div class="field-hint">
              主控将通过 SSH 信道在目标服务器部署轻量控制面板，自动签发 ECDSA 证书并锁定指纹，杜绝公网流量窃听与劫持。
            </div>
          </div>
        </div>

        <div class="config-submit-bar">
          <button
            class="btn-create-submit"
            @click="startProvisionExisting(selectedInstance)"
            :disabled="isSubmitting || !existingForm.name.trim() || (existingForm.auth_type === 'password' && !existingForm.ssh_password) || (existingForm.auth_type === 'key' && !existingForm.ssh_private_key)"
          >
            <span>🚀 立即通过 SSH 部署并纳管此实例</span>
          </button>
        </div>
      </div>
    </div>
  </div>

    <!-- 3. 部署中与实时日志流终端 -->
    <div v-else class="cloud-terminal-box">
      <!-- 6 阶段可视化步骤条 -->
      <div class="stepper-bar">
        <div
          v-for="s in stages"
          :key="s.num"
          class="stepper-item"
          :class="{
            'step-done': currentStage > s.num || (currentStage === 6 && taskStatus === 'success'),
            'step-current': currentStage === s.num && taskStatus === 'running',
            'step-failed': currentStage === s.num && taskStatus === 'failed'
          }"
        >
          <div class="step-circle">{{ s.num }}</div>
          <span class="step-label">{{ s.title }}</span>
        </div>
      </div>

      <!-- 终端主体 -->
      <div class="terminal-container">
        <div class="terminal-header">
          <div class="terminal-status-info">
            <span class="pulse-indicator" :class="{ 'pulse-active': taskStatus === 'running' }"></span>
            <span class="terminal-title">
              {{ taskStatus === 'running' ? 'Vultr 云端弹性开机与集群纳管进行中...' : (taskStatus === 'success' ? '🎉 云主机开通并纳管成功！' : '❌ 创建过程发生异常') }}
            </span>
          </div>
          <span class="terminal-node-name mono">{{ formReq.name }}</span>
        </div>

        <div class="terminal-body" ref="terminalBodyRef">
          <div v-for="(line, idx) in taskLogs" :key="idx" class="terminal-line" :class="getLineClass(line)">
            {{ line }}
          </div>
        </div>

        <div class="terminal-footer">
          <template v-if="taskStatus === 'success'">
            <button
              class="btn-primary"
              @click="finishAndSwitch"
              type="button"
            >
              <span>✅ 完成并切换至新节点管理视角</span>
            </button>
            <button
              class="btn-secondary"
              @click="resetProvisionState"
              type="button"
              style="margin-left: 0.5rem;"
            >
              <span>➕ 继续开通下一台 VPS</span>
            </button>
          </template>
          <div v-else-if="taskStatus === 'failed'" class="terminal-failed-actions">
            <button
              v-if="createdInstanceId"
              class="btn-primary btn-retry-highlight"
              @click="retryExistingInstanceProvision"
              :disabled="isSubmitting"
            >
              <span>🔄 立即重试纳管此实例 (无需重新购机)</span>
            </button>
            <button
              v-else
              class="btn-primary"
              @click="startProvision"
              :disabled="isSubmitting"
            >
              <span>🔄 重新尝试</span>
            </button>
            <button
              class="btn-secondary"
              @click="cloudStep = 'form'"
            >
              <span>↩️ 返回配置</span>
            </button>
            <button
              v-if="createdInstanceId"
              class="btn-secondary"
              @click="goToExistingInstances"
            >
              <span>📦 前往已有实例列表</span>
            </button>
            <button
              v-if="createdInstanceId"
              class="btn-danger-action"
              @click="openDestroyModalForCurrent"
              :disabled="isSubmitting"
              type="button"
            >
              <span>💥 销毁该实例 (停止扣费)</span>
            </button>
          </div>
          <span v-else class="deploy-progress-hint">
            正在等待网络路由贯通与 SSH 握手（包含 cloud-init 密钥写入），全流程通常需 45~90 秒，请保持页面开启...
          </span>
        </div>
      </div>
    </div>

    <!-- 彻底销毁 Vultr 实例二次安全验证模态框 -->
    <div v-if="destroyModalVisible" class="destroy-overlay">
      <div class="destroy-card">
        <div class="destroy-header">
          <div class="destroy-title-box">
            <span class="destroy-icon">💥</span>
            <div>
              <h4 class="destroy-title">彻底销毁 Vultr 云主机确认</h4>
              <p class="destroy-desc">调用 Vultr 开放接口删除服务器并停止扣费</p>
            </div>
          </div>
          <button class="close-btn" @click="closeDestroyModal" type="button">✕</button>
        </div>

        <div class="destroy-body">
          <div class="destroy-instance-info">
            <div class="info-row">
              <span class="info-label">实例名称:</span>
              <span class="info-val font-bold">{{ targetDestroyInstance?.label || targetDestroyInstance?.id }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">公网 IP:</span>
              <span class="info-val mono font-highlight">{{ targetDestroyInstance?.main_ip || '未分配' }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">所属区域:</span>
              <span class="info-val">{{ targetDestroyInstance?.city_zh || targetDestroyInstance?.region }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">实例规格:</span>
              <span class="info-val">{{ targetDestroyInstance?.plan }} ({{ targetDestroyInstance?.vcpu_count }}核 / {{ targetDestroyInstance?.ram }}MB)</span>
            </div>
          </div>

          <div class="destroy-warning-box">
            <span class="warn-icon">⚠️</span>
            <div>
              <strong>高危操作警告：</strong>
              该操作将直接调用 Vultr 开放平台接口，<strong>彻底删除该云端 VPS 主机及磁盘数据，并立即终止计费！此操作不可逆。</strong>
              <span v-if="targetDestroyInstance?.is_managed" style="display: block; margin-top: 0.25rem; color: #fca5a5;">
                * 该实例当前已纳管在集群中，销毁后将同步从本地集群中移出。
              </span>
            </div>
          </div>

          <div class="form-group" style="margin-top: 1rem;">
            <label>主控管理员密码 <span class="req">*</span></label>
            <input
              v-model="destroyMasterPassword"
              type="password"
              placeholder="请输入当前主控端管理员密码以授权销毁"
              @keyup.enter="submitDestroyInstance"
            />
          </div>

          <div class="destroy-footer">
            <button class="btn-secondary" @click="closeDestroyModal" type="button">取消</button>
            <button
              class="btn-danger-strong"
              @click="submitDestroyInstance"
              :disabled="destroyingInstance || !destroyMasterPassword"
              type="button"
            >
              <span>{{ destroyingInstance ? '⏳ 正在调用 Vultr 销毁...' : '💥 确认立即销毁云主机' }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import api, { getSecretPath, getStreamTicket } from '../api'
import { useToast } from '../composables/useToast'
import { useNodeContext } from '../composables/useNodeContext'
import { getToken } from '../utils/auth'

const emit = defineEmits<{
  (e: 'deployed', nodeId: number): void
  (e: 'close'): void
  (e: 'task-change', info: { running: boolean; name: string; tab: 'cloud' }): void
}>()

const { toast } = useToast()
const { fetchNodes, switchNode } = useNodeContext()

// Vultr 配置状态
const vultrConfig = reactive({
  configured: false,
  masked_key: '',
  account: null as any,
  error: '',
})

const showUpdateToken = ref(false)
const tokenInput = ref('')
const showTokenPlain = ref(false)
const savingToken = ref(false)
const tokenError = ref('')

// Catalog 目录树
const loadingCatalog = ref(false)
const catalog = reactive<{
  regions: any[]
  plans: any[]
  os_list: any[]
}>({
  regions: [],
  plans: [],
  os_list: [],
})

// 筛选与选择状态
const continents = [
  { key: 'all', label: '全部区域' },
  { key: 'Asia', label: '🌏 亚太地区' },
  { key: 'North America', label: '🌎 美洲地区' },
  { key: 'Europe', label: '🌍 欧洲地区' },
  { key: 'Oceania', label: '🦘 大洋洲' },
]
const selectedContinent = ref('all')
const selectedRegion = ref('nrt')

const planTypeFilter = ref<'all' | 'vc2' | 'vhf' | 'vhp'>('all')
const selectedPlan = ref('vc2-1c-1gb')

const selectedOSFamily = ref<'debian' | 'ubuntu' | 'rhel' | 'other'>('debian')
const selectedOSID = ref<number>(2136)

const formReq = reactive({
  name: '东京 Vultr-01',
  panel_port: 20888,
  enable_ipv6: true,
  enable_tls: true,
})

// 阶段与终端
const cloudStep = ref<'form' | 'terminal'>('form')
const currentStage = ref(1)
const taskStatus = ref<'running' | 'success' | 'failed'>('running')
const taskLogs = ref<string[]>([])
const terminalBodyRef = ref<HTMLElement | null>(null)
let eventSource: EventSource | null = null
let deployedNodeId = 0
const isSubmitting = ref(false)
const createdInstanceId = ref('')
const createdInstanceIP = ref('')

// 任务进行中状态同步，便于父级组件感知并防误触退出
watch(
  [cloudStep, taskStatus, isSubmitting],
  () => {
    const isRunning = isSubmitting.value || (cloudStep.value === 'terminal' && taskStatus.value === 'running')
    emit('task-change', {
      running: isRunning,
      name: (provisionMode.value === 'existing' ? existingForm.name : formReq.name) || 'Vultr 云主机',
      tab: 'cloud',
    })
  },
  { immediate: true }
)

// 纳管已有实例状态
const provisionMode = ref<'create' | 'existing'>('create')
const vultrInstances = ref<any[]>([])
const loadingInstances = ref(false)
const existingSearchQuery = ref('')
const selectedInstance = ref<any | null>(null)

const existingForm = reactive({
  name: '',
  auth_type: 'master_key' as 'master_key' | 'password' | 'key',
  ssh_password: '',
  ssh_private_key: '',
  panel_port: 20888,
  admin_username: 'admin',
  admin_password: '',
  secret_path: '',
  enable_tls: true,
})

const stages = [
  { num: 1, title: '绑定密钥' },
  { num: 2, title: '创建实例' },
  { num: 3, title: '分配 IP' },
  { num: 4, title: 'SSH 就绪' },
  { num: 5, title: '极速部署' },
  { num: 6, title: '握手纳管' },
]

// 联动计算属性
const filteredRegions = computed(() => {
  if (selectedContinent.value === 'all') return catalog.regions
  return catalog.regions.filter((r) => r.continent === selectedContinent.value)
})

const filteredPlans = computed(() => {
  let list = catalog.plans
  if (planTypeFilter.value !== 'all') {
    list = list.filter((p) => p.type === planTypeFilter.value)
  }
  return list.slice(0, 16)
})

const filteredOSList = computed(() => {
  return catalog.os_list.filter((o) => {
    const fam = o.family?.toLowerCase() || ''
    if (selectedOSFamily.value === 'debian') return fam === 'debian'
    if (selectedOSFamily.value === 'ubuntu') return fam === 'ubuntu'
    if (selectedOSFamily.value === 'rhel') return fam === 'rocky linux' || fam === 'almalinux' || fam === 'centos'
    return fam === 'fedora' || fam === 'arch linux'
  })
})

const currentRegionName = computed(() => {
  const r = catalog.regions.find((x) => x.id === selectedRegion.value)
  return r ? r.city_zh : selectedRegion.value
})

const currentRegionFlag = computed(() => {
  const r = catalog.regions.find((x) => x.id === selectedRegion.value)
  return r ? r.flag_emoji : '🌐'
})

const currentPlanText = computed(() => {
  const p = catalog.plans.find((x) => x.id === selectedPlan.value)
  return p ? `${p.vcpu_count} vCPU ${p.ram}MB` : selectedPlan.value
})

const currentPlanCost = computed(() => {
  const p = catalog.plans.find((x) => x.id === selectedPlan.value)
  return p ? p.monthly_cost : 5.0
})

const currentOSText = computed(() => {
  const o = catalog.os_list.find((x) => x.id === selectedOSID.value)
  return o ? o.name : 'Debian'
})

const canSubmit = computed(() => {
  return selectedRegion.value && selectedPlan.value && selectedOSID.value && formReq.name.trim()
})

const unmanagedInstancesCount = computed(() => {
  return vultrInstances.value.filter((i) => !i.is_managed).length
})

const filteredInstances = computed(() => {
  let list = vultrInstances.value
  if (existingSearchQuery.value.trim()) {
    const q = existingSearchQuery.value.toLowerCase().trim()
    list = list.filter((i) =>
      (i.label || '').toLowerCase().includes(q) ||
      (i.main_ip || '').toLowerCase().includes(q) ||
      (i.region || '').toLowerCase().includes(q) ||
      (i.os || '').toLowerCase().includes(q) ||
      (i.city_zh || '').toLowerCase().includes(q)
    )
  }
  return list
})

async function switchToExistingMode() {
  provisionMode.value = 'existing'
  if (vultrInstances.value.length === 0) {
    await loadVultrInstances(false)
  }
}

async function loadVultrInstances(refresh = false) {
  loadingInstances.value = true
  try {
    const res = await api.get('/nodes/cloud/vultr/instances')
    vultrInstances.value = res.data || []
  } catch (err: any) {
    toast.error('获取 Vultr 实例列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loadingInstances.value = false
  }
}

function selectExistingInstance(inst: any) {
  if (inst.is_managed) return
  selectedInstance.value = inst
  existingForm.name = inst.label || `Vultr-${inst.region.toUpperCase()}`
  existingForm.ssh_password = inst.default_password || ''
  if (inst.default_password) {
    existingForm.auth_type = 'password'
  } else {
    existingForm.auth_type = 'master_key'
  }
}

function goToExistingInstances() {
  cloudStep.value = 'form'
  switchToExistingMode()
}

function isPlanInRegion(plan: any): boolean {
  if (!plan.locations || !Array.isArray(plan.locations)) return true
  return plan.locations.includes(selectedRegion.value)
}

function selectRegion(r: any) {
  selectedRegion.value = r.id
  formReq.name = `${r.city} Vultr-01`
}

function selectOSFamily(fam: 'debian' | 'ubuntu' | 'rhel' | 'other') {
  selectedOSFamily.value = fam
  const match = filteredOSList.value[0]
  if (match) {
    selectedOSID.value = match.id
  }
}

// 加载配置
async function loadVultrConfig() {
  try {
    const res = await api.get('/nodes/cloud/vultr/config')
    vultrConfig.configured = res.data.configured
    vultrConfig.masked_key = res.data.masked_key
    vultrConfig.account = res.data.account
    vultrConfig.error = res.data.error || ''

    if (vultrConfig.configured && !vultrConfig.error) {
      await loadCatalog(false)
    }
  } catch (err: any) {
    console.error('加载 Vultr 配置失败', err)
  }
}

// 保存 Token
async function saveToken() {
  if (!tokenInput.value.trim()) return
  savingToken.value = true
  tokenError.value = ''
  try {
    const res = await api.post('/nodes/cloud/vultr/config', {
      api_key: tokenInput.value.trim(),
    })
    vultrConfig.configured = true
    vultrConfig.masked_key = res.data.masked_key
    vultrConfig.account = res.data.account
    vultrConfig.error = ''
    showUpdateToken.value = false
    tokenInput.value = ''
    toast.success('Vultr Token 验证通过并已绑定！')
    await loadCatalog(true)
  } catch (err: any) {
    tokenError.value = err.response?.data?.error || err.message || 'Token 验证失败'
    toast.error(tokenError.value)
  } finally {
    savingToken.value = false
  }
}

// 加载 Catalog
async function loadCatalog(refresh: boolean) {
  loadingCatalog.value = true
  try {
    const res = await api.get(`/nodes/cloud/vultr/catalog?refresh=${refresh ? 'true' : 'false'}`)
    catalog.regions = res.data.regions || []
    catalog.plans = res.data.plans || []
    catalog.os_list = res.data.os_list || []

    // 默认选中东京或第一个亚太区域
    if (!catalog.regions.some((r) => r.id === selectedRegion.value)) {
      const defaultR = catalog.regions.find((r) => r.id === 'nrt') || catalog.regions[0]
      if (defaultR) selectRegion(defaultR)
    }

    // 默认选中 Debian 12
    const recOS = catalog.os_list.find((o) => o.recommended) || catalog.os_list[0]
    if (recOS) selectedOSID.value = recOS.id
  } catch (err: any) {
    toast.error('加载 Vultr 产品目录失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loadingCatalog.value = false
  }
}

function refreshCatalog() {
  loadCatalog(true)
}

function scrollToBottom() {
  nextTick(() => {
    if (terminalBodyRef.value) {
      terminalBodyRef.value.scrollTop = terminalBodyRef.value.scrollHeight
    }
  })
}

function getLineClass(line: string) {
  if (line.includes('[SUCCESS]')) return 'line-success'
  if (line.includes('[ERROR]')) return 'line-error'
  if (line.includes('【阶段') || line.includes('IP:')) return 'line-highlight'
  return ''
}

// 订阅部署日志流 (优先使用脱敏 Stream Ticket)
async function subscribeDeployLogs(taskId: string) {
  const secret = getSecretPath()
  const ticket = await getStreamTicket()
  const authParam = ticket ? `ticket=${ticket}` : `token=${getToken() || ''}`
  const streamURL = `/${secret}/api/nodes/deploy-logs?task_id=${taskId}&${authParam}`

  if (eventSource) eventSource.close()
  eventSource = new EventSource(streamURL)

  eventSource.onmessage = (event) => {
    const line = event.data
    taskLogs.value.push(line)
    scrollToBottom()

    // 实例信息提取（用于在失败时重试）
    if (line.includes('实例 UUID: ')) {
      const match = line.match(/实例 UUID:\s*([a-zA-Z0-9-]+)/)
      if (match) createdInstanceId.value = match[1]
    }
    if (line.includes('分配公网 IP: ')) {
      const ipMatch = line.match(/分配公网 IP:\s*([0-9.]+)/)
      if (ipMatch) createdInstanceIP.value = ipMatch[1]
    }

    // 阶段推进识别
    if (line.includes('【阶段 1/6】')) currentStage.value = 1
    else if (line.includes('【阶段 2/6】')) currentStage.value = 2
    else if (line.includes('【阶段 3/6】')) currentStage.value = 3
    else if (line.includes('【阶段 4/6】')) currentStage.value = 4
    else if (line.includes('【阶段 5/6】')) currentStage.value = 5
    else if (line.includes('【阶段 6/6】')) currentStage.value = 6

    if (line.includes('[SUCCESS]')) {
      taskStatus.value = 'success'
      currentStage.value = 6
      eventSource?.close()
      fetchNodes().then(() => {
        const match = useNodeContext().nodes.value.find((n) =>
          (createdInstanceId.value && n.cloud_instance_id === createdInstanceId.value) ||
          (createdInstanceIP.value && n.host === createdInstanceIP.value) ||
          n.name === formReq.name ||
          n.name === existingForm.name
        )
        if (match) deployedNodeId = match.id
      })
    } else if (line.includes('[ERROR]')) {
      taskStatus.value = 'failed'
      eventSource?.close()
    }
  }

  eventSource.onerror = async () => {
    eventSource?.close()
    eventSource = null
    if (taskStatus.value !== 'running') return

    // SSE 连接中断时，自动启动状态轮询兜底
    let pollDone = false
    for (let i = 0; i < 20; i++) {
      await new Promise((r) => setTimeout(r, 1500))
      if (taskStatus.value !== 'running') return
      try {
        const statusRes = await api.get(`/nodes/deploy-status?task_id=${taskId}`)
        const data = statusRes.data
        if (data && Array.isArray(data.logs)) {
          if (data.logs.length > taskLogs.value.length) {
            taskLogs.value = data.logs
            scrollToBottom()
          }
        }
        if (data.status === 'success') {
          taskStatus.value = 'success'
          currentStage.value = 6
          fetchNodes().then(() => {
            const match = useNodeContext().nodes.value.find((n) =>
              (createdInstanceId.value && n.cloud_instance_id === createdInstanceId.value) ||
              (createdInstanceIP.value && n.host === createdInstanceIP.value) ||
              n.name === formReq.name ||
              n.name === existingForm.name
            )
            if (match) deployedNodeId = match.id
          })
          pollDone = true
          return
        } else if (data.status === 'failed') {
          taskStatus.value = 'failed'
          if (data.error && !taskLogs.value.some((l: string) => l.includes(data.error))) {
            taskLogs.value.push(`[ERROR] ${data.error}`)
            scrollToBottom()
          }
          pollDone = true
          return
        }
      } catch {
        // 忽略单次网络抖动继续重试
      }
    }
    if (!pollDone && taskStatus.value === 'running') {
      taskStatus.value = 'failed'
      taskLogs.value.push('[ERROR] 日志流连接中断且状态轮询超时')
      scrollToBottom()
    }
  }
}

// 启动弹性开机流水线
async function startProvision() {
  if (!canSubmit.value) return

  isSubmitting.value = true
  cloudStep.value = 'terminal'
  currentStage.value = 1
  taskStatus.value = 'running'
  taskLogs.value = []
  createdInstanceId.value = ''
  createdInstanceIP.value = ''

  try {
    const res = await api.post('/nodes/cloud/vultr/provision', {
      name: formReq.name.trim(),
      region: selectedRegion.value,
      plan: selectedPlan.value,
      os_id: selectedOSID.value,
      enable_ipv6: formReq.enable_ipv6,
      panel_port: formReq.panel_port || 20888,
      enable_tls: formReq.enable_tls,
    })

    const taskId = res.data.task_id
    subscribeDeployLogs(taskId)
  } catch (err: any) {
    taskStatus.value = 'failed'
    taskLogs.value.push('[ERROR] 启动任务失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isSubmitting.value = false
  }
}

// 启动已有实例纳管流水线
async function startProvisionExisting(inst: any) {
  if (!inst) return

  isSubmitting.value = true
  cloudStep.value = 'terminal'
  currentStage.value = 1
  taskStatus.value = 'running'
  taskLogs.value = []
  createdInstanceId.value = inst.id
  createdInstanceIP.value = inst.main_ip

  try {
    const res = await api.post('/nodes/cloud/vultr/provision', {
      instance_id: inst.id,
      name: existingForm.name.trim() || inst.label,
      region: inst.region,
      plan: inst.plan,
      auth_type: existingForm.auth_type,
      ssh_password: existingForm.auth_type === 'password' ? existingForm.ssh_password : '',
      ssh_private_key: existingForm.auth_type === 'key' ? existingForm.ssh_private_key : '',
      panel_port: existingForm.panel_port || 20888,
      enable_tls: existingForm.enable_tls,
    })

    const taskId = res.data.task_id
    subscribeDeployLogs(taskId)
  } catch (err: any) {
    taskStatus.value = 'failed'
    taskLogs.value.push('[ERROR] 启动纳管任务失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isSubmitting.value = false
  }
}

// 失败重试已有实例纳管（无需重新购买 VPS）
async function retryExistingInstanceProvision() {
  if (!createdInstanceId.value) return

  isSubmitting.value = true
  currentStage.value = 1
  taskStatus.value = 'running'
  taskLogs.value = []

  try {
    const res = await api.post('/nodes/cloud/vultr/provision', {
      instance_id: createdInstanceId.value,
      name: formReq.name.trim() || existingForm.name.trim(),
      region: selectedRegion.value,
      plan: selectedPlan.value,
      panel_port: formReq.panel_port || existingForm.panel_port || 20888,
      enable_tls: formReq.enable_tls,
    })

    const taskId = res.data.task_id
    subscribeDeployLogs(taskId)
  } catch (err: any) {
    taskStatus.value = 'failed'
    taskLogs.value.push('[ERROR] 重试纳管失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isSubmitting.value = false
  }
}

// 彻底销毁 Vultr 云主机
const destroyModalVisible = ref(false)
const targetDestroyInstance = ref<any | null>(null)
const destroyMasterPassword = ref('')
const destroyingInstance = ref(false)

function openDestroyModal(inst: any) {
  targetDestroyInstance.value = inst
  destroyMasterPassword.value = ''
  destroyModalVisible.value = true
}

function openDestroyModalForCurrent() {
  if (!createdInstanceId.value) return
  let match = vultrInstances.value.find((i) => i.id === createdInstanceId.value)
  if (!match) {
    match = {
      id: createdInstanceId.value,
      label: formReq.name,
      main_ip: createdInstanceIP.value,
      region: selectedRegion.value,
      city_zh: currentRegionName.value,
      plan: selectedPlan.value,
      vcpu_count: 1,
      ram: 1024,
      is_managed: false,
    }
  }
  openDestroyModal(match)
}

function closeDestroyModal() {
  destroyModalVisible.value = false
  targetDestroyInstance.value = null
  destroyMasterPassword.value = ''
}

async function submitDestroyInstance() {
  if (!targetDestroyInstance.value || !destroyMasterPassword.value) {
    toast.warning('请输入主控管理员密码')
    return
  }

  destroyingInstance.value = true
  try {
    const instId = targetDestroyInstance.value.id
    const res = await api.delete(`/nodes/cloud/vultr/instances/${instId}`, {
      data: {
        master_password: destroyMasterPassword.value,
      },
    })

    toast.success(res.data.message || 'Vultr 云主机已成功销毁并终止计费')

    // 如果销毁的是当前刚创建并失败的实例，清理状态并回到表单
    if (createdInstanceId.value === instId) {
      createdInstanceId.value = ''
      createdInstanceIP.value = ''
      cloudStep.value = 'form'
    }

    if (selectedInstance.value?.id === instId) {
      selectedInstance.value = null
    }

    closeDestroyModal()
    // 刷新实例列表与集群节点
    await loadVultrInstances(true)
    await fetchNodes()
  } catch (err: any) {
    toast.error('销毁云主机失败: ' + (err.response?.data?.error || err.message))
  } finally {
    destroyingInstance.value = false
  }
}

function resetProvisionState() {
  cloudStep.value = 'form'
  currentStage.value = 1
  taskStatus.value = 'running'
  taskLogs.value = []
  createdInstanceId.value = ''
  createdInstanceIP.value = ''
  deployedNodeId = 0
  formReq.name = ''
  formReq.sub_prefix = ''
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
}

function finishAndSwitch() {
  const targetId = deployedNodeId
  resetProvisionState()
  if (targetId > 0) {
    switchNode(targetId)
    toast.success('已切换至新开通的云主机视角！')
  } else {
    toast.success('开机纳管完成！')
  }
  emit('close')
}

onMounted(() => {
  loadVultrConfig()
})

onUnmounted(() => {
  if (eventSource) eventSource.close()
})
</script>

<style scoped>
.vultr-provisioner {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

/* 顶部状态条 */
.token-setup-card {
  background: rgba(99, 102, 241, 0.08);
  border: 1px solid rgba(99, 102, 241, 0.3);
  padding: 1rem 1.25rem;
  border-radius: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.setup-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.setup-icon {
  font-size: 1.5rem;
}

.setup-title {
  margin: 0;
  font-size: 0.95rem;
  color: #f1f5f9;
}

.setup-desc {
  margin: 0.15rem 0 0 0;
  font-size: 0.75rem;
  color: #94a3b8;
}

.token-input-row {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.token-input {
  flex: 1;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.55rem 0.75rem;
  color: #f8fafc;
  font-size: 0.8125rem;
  outline: none;
}

.token-input:focus {
  border-color: #6366f1;
}

.btn-toggle-eye {
  background: #334155;
  border: none;
  padding: 0.55rem 0.65rem;
  border-radius: 0.45rem;
  cursor: pointer;
}

.token-error-msg {
  font-size: 0.75rem;
  color: #f87171;
}

.token-status-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(15, 23, 42, 0.8);
  border: 1px solid #334155;
  border-radius: 0.65rem;
  padding: 0.65rem 1rem;
}

.status-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.cloud-icon {
  font-size: 1.4rem;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.provider-badge {
  font-size: 0.675rem;
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
  padding: 0.05rem 0.35rem;
  border-radius: 0.25rem;
  font-weight: 600;
}

.masked-key {
  font-size: 0.75rem;
  color: #cbd5e1;
}

.active-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #10b981;
}

.balance-text {
  font-size: 0.72rem;
  color: #94a3b8;
  margin-top: 0.2rem;
}

.color-success {
  color: #34d399;
}

.dot-divider {
  margin: 0 0.35rem;
  color: #475569;
}

.status-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.btn-mini-refresh,
.btn-mini-update {
  background: #1e293b;
  border: 1px solid #334155;
  color: #94a3b8;
  font-size: 0.72rem;
  padding: 0.3rem 0.6rem;
  border-radius: 0.35rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-mini-refresh:hover,
.btn-mini-update:hover {
  background: #334155;
  color: #ffffff;
}

/* 步骤块 */
.provision-flow {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.section-card {
  background: #0b1120;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  padding: 1rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.section-title-row {
  display: flex;
  align-items: center;
  gap: 0.65rem;
}

.step-num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #4f46e5;
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.75rem;
  font-weight: 700;
}

.section-title {
  margin: 0;
  font-size: 0.875rem;
  font-weight: 600;
  color: #f8fafc;
}

.section-subtitle {
  font-size: 0.7rem;
  color: #64748b;
}

/* 区域 */
.continent-tabs {
  display: flex;
  gap: 0.4rem;
}

.continent-btn {
  background: transparent;
  border: 1px solid #334155;
  color: #94a3b8;
  padding: 0.25rem 0.65rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  cursor: pointer;
}

.continent-btn-active {
  background: rgba(99, 102, 241, 0.2);
  border-color: #6366f1;
  color: #a5b4fc;
  font-weight: 600;
}

.regions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 0.6rem;
  max-height: 240px;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.region-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.65rem;
  display: flex;
  align-items: center;
  gap: 0.6rem;
  cursor: pointer;
  position: relative;
  transition: all 0.15s ease;
}

.region-card:hover {
  border-color: #475569;
}

.region-card-selected {
  background: rgba(99, 102, 241, 0.15);
  border-color: #6366f1;
}

.region-flag {
  font-size: 1.5rem;
}

.region-city {
  font-size: 0.775rem;
  font-weight: 500;
  color: #f1f5f9;
}

.region-meta {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin-top: 0.15rem;
}

.region-id {
  font-size: 0.675rem;
  color: #818cf8;
}

.region-continent {
  font-size: 0.65rem;
  color: #64748b;
}

.check-badge {
  position: absolute;
  top: 0.4rem;
  right: 0.4rem;
  font-size: 0.75rem;
  color: #818cf8;
  font-weight: bold;
}

/* 机型 */
.plan-type-tabs {
  display: flex;
  gap: 0.4rem;
}

.plan-tab-btn {
  background: transparent;
  border: 1px solid #334155;
  color: #94a3b8;
  padding: 0.25rem 0.65rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  cursor: pointer;
}

.plan-tab-active {
  background: rgba(99, 102, 241, 0.2);
  border-color: #6366f1;
  color: #a5b4fc;
  font-weight: 600;
}

.plans-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 0.65rem;
  max-height: 260px;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.plan-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.plan-card:hover:not(.plan-card-disabled) {
  border-color: #475569;
}

.plan-card-selected {
  background: rgba(99, 102, 241, 0.15);
  border-color: #6366f1;
}

.plan-card-disabled {
  opacity: 0.4;
  cursor: not-allowed;
  filter: grayscale(0.8);
}

.plan-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.plan-id {
  font-size: 0.75rem;
  font-weight: 600;
  color: #f1f5f9;
}

.plan-price {
  font-size: 0.85rem;
  color: #34d399;
}

.plan-specs {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.7rem;
}

.spec-row {
  display: flex;
  justify-content: space-between;
}

.spec-label {
  color: #64748b;
}

.spec-val {
  color: #cbd5e1;
}

.stock-badge {
  font-size: 0.65rem;
  padding: 0.1rem 0.35rem;
  border-radius: 0.25rem;
}

.stock-available {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
}

.stock-selected {
  background: rgba(99, 102, 241, 0.25);
  color: #a5b4fc;
  font-weight: 600;
}

.stock-empty {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

/* 系统选择 */
.os-family-tabs {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.5rem;
}

.os-tab-btn {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.15rem;
  color: #cbd5e1;
  font-size: 0.775rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.os-tab-btn small {
  font-size: 0.65rem;
  color: #64748b;
}

.os-tab-active {
  background: rgba(99, 102, 241, 0.2);
  border-color: #6366f1;
  color: #f8fafc;
}

.os-tab-active small {
  color: #a5b4fc;
}

.os-version-selector {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  font-size: 0.775rem;
  color: #94a3b8;
}

.os-select {
  flex: 1;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.5rem 0.65rem;
  color: #f1f5f9;
  font-size: 0.775rem;
  outline: none;
}

/* 选项表单 */
.options-grid {
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

.form-group input {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.55rem 0.75rem;
  color: #f8fafc;
  font-size: 0.8125rem;
  outline: none;
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

/* 底部操作条 */
.bottom-action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 0.75rem;
  padding: 0.85rem 1.25rem;
  position: sticky;
  bottom: 0;
  z-index: 10;
  box-shadow: 0 -10px 25px -5px rgba(0, 0, 0, 0.4);
}

.summary-box {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.summary-label {
  font-size: 0.7rem;
  color: #64748b;
}

.summary-value {
  font-size: 0.8125rem;
  font-weight: 500;
  color: #f1f5f9;
}

.summary-price {
  font-size: 0.75rem;
  color: #94a3b8;
}

.summary-price strong {
  color: #34d399;
}

.btn-create-submit {
  background: linear-gradient(135deg, #4f46e5 0%, #7c3aed 100%);
  color: #ffffff;
  border: none;
  padding: 0.7rem 1.5rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
  box-shadow: 0 4px 12px rgba(79, 70, 229, 0.35);
}

.btn-create-submit:hover:not(:disabled) {
  opacity: 0.95;
  transform: translateY(-1px);
}

.btn-create-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 终端与步骤条 */
.cloud-terminal-box {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.stepper-bar {
  display: flex;
  justify-content: space-between;
  background: #0b1120;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  padding: 0.75rem 1.25rem;
}

.stepper-item {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  opacity: 0.4;
  transition: all 0.2s ease;
}

.step-circle {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #334155;
  color: #cbd5e1;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.7rem;
  font-weight: bold;
}

.step-label {
  font-size: 0.75rem;
  color: #94a3b8;
}

.step-current {
  opacity: 1;
}

.step-current .step-circle {
  background: #4f46e5;
  color: #ffffff;
  box-shadow: 0 0 10px rgba(99, 102, 241, 0.6);
}

.step-current .step-label {
  color: #818cf8;
  font-weight: 600;
}

.step-done {
  opacity: 0.9;
}

.step-done .step-circle {
  background: #10b981;
  color: #ffffff;
}

.step-done .step-label {
  color: #34d399;
}

.step-failed {
  opacity: 1;
}

.step-failed .step-circle {
  background: #ef4444;
  color: #ffffff;
}

.step-failed .step-label {
  color: #f87171;
}

.terminal-container {
  display: flex;
  flex-direction: column;
  background: #020617;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  overflow: hidden;
  height: 440px;
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

.btn-primary {
  background: #4f46e5;
  color: #ffffff;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-secondary {
  background: #334155;
  color: #cbd5e1;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  cursor: pointer;
}

.terminal-failed-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.btn-retry-highlight {
  background: #0284c7;
  color: #ffffff;
}
.btn-retry-highlight:hover:not(:disabled) {
  background: #0369a1;
}

/* 模式切换 Tab 样式 */
.mode-switch-wrapper {
  display: flex;
  margin-bottom: 0.25rem;
}

.provision-mode-bar {
  display: flex;
  background: #0f172a;
  padding: 0.25rem;
  border-radius: 0.6rem;
  border: 1px solid #1e293b;
  gap: 0.35rem;
}

.mode-pill-btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 1rem;
  border-radius: 0.45rem;
  border: none;
  background: transparent;
  color: #94a3b8;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-pill-btn:hover {
  color: #f1f5f9;
}

.mode-pill-active {
  background: #1e293b;
  color: #38bdf8;
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.mode-badge {
  font-size: 0.675rem;
  background: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
  padding: 0.05rem 0.35rem;
  border-radius: 9999px;
  font-weight: 600;
}

/* 纳管已有实例工作区样式 */
.existing-flow {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.existing-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
}

.search-input-box {
  flex: 1;
  display: flex;
  align-items: center;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.45rem 0.75rem;
  gap: 0.5rem;
}

.search-icon {
  font-size: 0.85rem;
  color: #64748b;
}

.existing-search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: #f8fafc;
  font-size: 0.8125rem;
}

.btn-refresh-instances {
  background: #1e293b;
  border: 1px solid #334155;
  color: #94a3b8;
  padding: 0.45rem 0.85rem;
  border-radius: 0.5rem;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.btn-refresh-instances:hover:not(:disabled) {
  color: #ffffff;
  border-color: #64748b;
}

.empty-instances-state {
  background: #1e293b;
  border: 1px dashed #334155;
  border-radius: 0.75rem;
  padding: 2.5rem 1rem;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}

.empty-icon {
  font-size: 2.2rem;
}

.empty-text {
  font-size: 0.95rem;
  font-weight: 600;
  color: #cbd5e1;
}

.empty-sub {
  font-size: 0.775rem;
  color: #64748b;
}

.instances-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 0.85rem;
}

.instance-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.65rem;
  padding: 0.85rem;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  cursor: pointer;
  transition: all 0.15s ease;
}
.instance-card:hover {
  border-color: #475569;
}

.instance-card-selected {
  border-color: #0ea5e9;
  background: rgba(14, 165, 233, 0.08);
}

.instance-card-managed {
  opacity: 0.75;
  cursor: default;
  border-style: dashed;
}

.instance-card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.inst-region-box {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.inst-flag {
  font-size: 1.1rem;
}

.inst-city {
  font-size: 0.8rem;
  font-weight: 500;
  color: #cbd5e1;
}

.tag-managed {
  font-size: 0.65rem;
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  padding: 0.1rem 0.4rem;
  border-radius: 0.25rem;
  font-weight: 500;
}

.tag-unmanaged {
  font-size: 0.65rem;
  background: rgba(14, 165, 233, 0.15);
  color: #38bdf8;
  padding: 0.1rem 0.4rem;
  border-radius: 0.25rem;
  font-weight: 500;
}

.inst-name-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.inst-label {
  font-size: 0.875rem;
  font-weight: 600;
  color: #f1f5f9;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 180px;
}

.inst-power {
  font-size: 0.675rem;
  font-family: ui-monospace, monospace;
}

.power-running {
  color: #34d399;
}

.power-stopped {
  color: #94a3b8;
}

.inst-props-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  background: #0f172a;
  padding: 0.5rem 0.65rem;
  border-radius: 0.45rem;
  font-size: 0.725rem;
}

.inst-prop {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.font-highlight {
  color: #38bdf8;
  font-weight: 600;
}

.text-truncate {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 170px;
}

.instance-card-bottom {
  margin-top: auto;
  padding-top: 0.35rem;
}

.btn-select-inst {
  width: 100%;
  padding: 0.35rem;
  background: rgba(14, 165, 233, 0.12);
  border: 1px solid rgba(14, 165, 233, 0.35);
  color: #38bdf8;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-select-inst:hover {
  background: rgba(14, 165, 233, 0.25);
  color: #ffffff;
}

.btn-selected-active {
  background: #0ea5e9;
  color: #ffffff;
  border-color: #0ea5e9;
}

.managed-note {
  font-size: 0.7rem;
  color: #64748b;
  display: block;
  text-align: center;
}

/* 纳管配置卡片样式 */
.existing-config-card {
  background: #1e293b;
  border: 1px solid #0ea5e9;
  border-radius: 0.75rem;
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  box-shadow: 0 10px 25px -5px rgba(14, 165, 233, 0.15);
  animation: slideUp 0.2s ease-out;
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

.config-card-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  border-bottom: 1px solid #334155;
  padding-bottom: 0.85rem;
}

.config-header-icon {
  font-size: 1.6rem;
}

.config-header-title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: #f1f5f9;
}

.config-header-desc {
  font-size: 0.75rem;
  color: #94a3b8;
}

.config-form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.85rem;
}

.label-with-action {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.btn-fill-default-pwd {
  background: rgba(245, 158, 11, 0.15);
  border: 1px solid rgba(245, 158, 11, 0.35);
  color: #fbbf24;
  font-size: 0.675rem;
  padding: 0.1rem 0.45rem;
  border-radius: 0.3rem;
  cursor: pointer;
}
.btn-fill-default-pwd:hover {
  background: rgba(245, 158, 11, 0.25);
}

.config-submit-bar {
  display: flex;
  justify-content: flex-end;
  border-top: 1px solid #334155;
  padding-top: 0.85rem;
}

.inst-actions-row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.btn-card-destroy {
  background: rgba(239, 68, 68, 0.12);
  border: 1px solid rgba(239, 68, 68, 0.3);
  color: #f87171;
  padding: 0.35rem 0.55rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-card-destroy:hover {
  background: rgba(239, 68, 68, 0.25);
  border-color: #ef4444;
}

.btn-danger-action {
  background: rgba(239, 68, 68, 0.15);
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #fca5a5;
  padding: 0.6rem 1rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-danger-action:hover:not(:disabled) {
  background: rgba(239, 68, 68, 0.28);
  color: #ffffff;
  border-color: #ef4444;
}

/* 销毁模态框样式 */
.destroy-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.82);
  backdrop-filter: blur(8px);
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
}

.destroy-card {
  background: #0f172a;
  border: 1px solid #475569;
  border-radius: 0.85rem;
  width: 100%;
  max-width: 500px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.85);
  overflow: hidden;
  animation: modalScale 0.15s ease-out;
}

.destroy-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.15rem 1.35rem;
  border-bottom: 1px solid #1e293b;
  background: #1e293b;
}

.destroy-title-box {
  display: flex;
  align-items: center;
  gap: 0.65rem;
}

.destroy-icon {
  font-size: 1.6rem;
}

.destroy-title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: #f8fafc;
}

.destroy-desc {
  margin: 0.15rem 0 0 0;
  font-size: 0.725rem;
  color: #94a3b8;
}

.destroy-body {
  padding: 1.25rem 1.35rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.destroy-instance-info {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.75rem 0.95rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.775rem;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label {
  color: #94a3b8;
}

.info-val {
  color: #f1f5f9;
}

.font-bold {
  font-weight: 600;
}

.destroy-warning-box {
  display: flex;
  align-items: flex-start;
  gap: 0.65rem;
  background: rgba(239, 68, 68, 0.1);
  border: 1px dashed rgba(239, 68, 68, 0.4);
  padding: 0.85rem 1rem;
  border-radius: 0.5rem;
  font-size: 0.775rem;
  color: #fca5a5;
  line-height: 1.45;
}

.warn-icon {
  font-size: 1.25rem;
  flex-shrink: 0;
}

.destroy-footer {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.btn-danger-strong {
  background: #dc2626;
  color: #ffffff;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-danger-strong:hover:not(:disabled) {
  background: #b91c1c;
}
.btn-danger-strong:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.mono {
  font-family: ui-monospace, monospace;
}
</style>

