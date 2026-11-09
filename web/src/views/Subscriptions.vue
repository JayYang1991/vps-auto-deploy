<template>
  <div>
    <!-- 头部说明与 Tab 切换 -->
    <div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">👥</span>
          <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">团队成员与订阅分发中心</h1>
          <span
            v-if="subStatus.installed && subStatus.active"
            class="badge badge-success"
            style="font-size: 0.725rem;"
            title="Subconverter 订阅转换引擎正在运行"
          >
            ⚡ Subconverter 引擎已就绪 (v{{ subStatus.installed_version }})
          </span>
          <span
            v-else-if="subStatus.installed"
            class="badge badge-warning"
            style="font-size: 0.725rem;"
          >
            Subconverter 已停止
          </span>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          管理团队成员独立免登录订阅，并配置 Subconverter 高级跨客户端订阅转换与 ACL4SSR 远程分流规则集。
        </p>
      </div>

      <!-- 子业务分类切换器 -->
      <div style="display: flex; gap: 0.5rem; background: #0f172a; padding: 0.25rem; border-radius: 0.5rem; border: 1px solid #334155;">
        <button
          @click="activeSubSection = 'members'"
          :class="['btn', activeSubSection === 'members' ? 'btn-primary' : 'btn-secondary']"
          style="padding: 0.35rem 0.85rem; font-size: 0.8rem; border: none;"
          type="button"
        >
          <span>👥 成员与订阅链接</span>
          <span v-if="members.length" class="badge badge-secondary" style="margin-left: 0.35rem; font-size: 0.7rem;">
            {{ members.length }}
          </span>
        </button>

        <button
          @click="activeSubSection = 'subconverter'"
          :class="['btn', activeSubSection === 'subconverter' ? 'btn-primary' : 'btn-secondary']"
          style="padding: 0.35rem 0.85rem; font-size: 0.8rem; border: none;"
          type="button"
        >
          <span>🔄 Subconverter 转换引擎</span>
          <span
            v-if="!subStatus.installed"
            class="badge badge-warning"
            style="margin-left: 0.35rem; font-size: 0.65rem;"
          >
            未安装
          </span>
        </button>

        <button
          @click="activeSubSection = 'cf_preferred'"
          :class="['btn', activeSubSection === 'cf_preferred' ? 'btn-primary' : 'btn-secondary']"
          style="padding: 0.35rem 0.85rem; font-size: 0.8rem; border: none;"
          type="button"
        >
          <span>⚡ Cloudflare 优选 IP</span>
          <span v-if="cfPrefStatus.total_count" class="badge badge-secondary" style="margin-left: 0.35rem; font-size: 0.7rem;">
            {{ cfPrefStatus.total_count }}
          </span>
        </button>

        <button
          @click="activeSubSection = 'inventory'"
          :class="['btn', activeSubSection === 'inventory' ? 'btn-primary' : 'btn-secondary']"
          style="padding: 0.35rem 0.85rem; font-size: 0.8rem; border: none;"
          type="button"
        >
          <span>📋 节点清单</span>
          <span v-if="inventoryData?.total_inbounds" class="badge badge-secondary" style="margin-left: 0.35rem; font-size: 0.7rem;">
            {{ inventoryData.total_inbounds }}
          </span>
        </button>
      </div>
    </div>

    <!-- ==================== 版块一：成员与订阅链接 ==================== -->
    <div v-if="activeSubSection === 'members'">
      <!-- Subconverter 未安装时的渐进式引导小贴士 -->
      <div
        v-if="!subStatus.installed && !loading"
        style="background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 0.5rem; padding: 0.85rem 1.15rem; margin-bottom: 1.25rem; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.75rem;"
      >
        <div style="display: flex; align-items: center; gap: 0.6rem;">
          <span style="font-size: 1.25rem;">💡</span>
          <div style="font-size: 0.85rem; color: #fde68a;">
            <strong>解锁高级规则分流：</strong>
            当前使用的是内置 Go 订阅生成器。安装 <strong>Subconverter 订阅转换引擎</strong> 后，即可一键分发 ACL4SSR 策略组、广告拦截与数十种客户端配置。
          </div>
        </div>
        <router-link
          to="/components?install=subconverter"
          class="btn btn-primary"
          style="font-size: 0.775rem; padding: 0.35rem 0.75rem; text-decoration: none;"
        >
          🚀 前往「软件中心」一键安装
        </router-link>
      </div>

      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.75rem;">
        <div style="font-size: 0.85rem; color: #94a3b8; display: flex; align-items: center; gap: 0.5rem;">
          <span style="display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: #10b981;"></span>
          <span>主控统一凭据源 (SSOT) · 自动同步至全集群纳管节点</span>
        </div>
        <div style="display: flex; gap: 0.5rem;">
          <button 
            @click="broadcastSyncMembers" 
            :disabled="syncingMembers" 
            class="btn btn-secondary"
            style="font-size: 0.825rem; display: flex; align-items: center; gap: 0.35rem;"
            title="将当前成员 UUID 与凭据推送到所有被纳管 VPS 节点并热重载底层 sing-box"
          >
            {{ syncingMembers ? '⏳ 正在全网广播...' : '⚡ 同步全网凭据' }}
          </button>
          <button @click="openCreateMemberModal" class="btn btn-primary">+ 添加成员</button>
        </div>
      </div>

      <!-- 加载中指示器 -->
      <div v-if="loading" class="card loading-box">
        <div class="loading-spinner"></div>
        <p class="loading-text">正在加载团队成员与订阅配置...</p>
      </div>

      <!-- 空状态 -->
      <div v-else-if="members.length === 0" class="card" style="text-align: center; padding: 3rem;">
        <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">👥</div>
        <p style="color: #94a3b8; margin-bottom: 1.25rem;">暂无团队成员，点击下方按钮添加首个成员以生成客户端专属订阅。</p>
        <button @click="openCreateMemberModal" class="btn btn-primary">+ 添加首个成员</button>
      </div>

      <!-- 成员卡片列表 -->
      <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
        <div v-for="m in members" :key="m.id" class="card">
          <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem; margin-bottom: 1rem;">
            <div>
              <div style="display: flex; align-items: center; gap: 0.75rem;">
                <span style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">{{ m.name }}</span>
                <span :class="['badge', m.enabled ? 'badge-success' : 'badge-secondary']">
                  {{ m.enabled ? '正常' : '已禁用' }}
                </span>
              </div>
              <div style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.35rem; font-family: monospace;">
                UUID: {{ m.uuid }}
              </div>
            </div>

            <div style="display: flex; gap: 0.5rem;">
              <button @click="openEditMemberModal(m)" class="btn btn-secondary" style="font-size: 0.8rem;">
                ✏️ 编辑
              </button>
              <button @click="resetToken(m.id)" class="btn btn-secondary" style="font-size: 0.8rem;">
                🔄 重置订阅码
              </button>
              <button v-if="members.length > 1" @click="deleteMember(m.id)" class="btn btn-danger" style="font-size: 0.8rem;">
                删除
              </button>
            </div>
          </div>

          <!-- 订阅格式一键获取 -->
          <div style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <!-- Subconverter 高级订阅 (若已安装) -->
            <div
              v-if="subStatus.installed && subStatus.active"
              style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem; padding-bottom: 0.75rem; border-bottom: 1px dashed #334155;"
            >
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: #a855f7; font-weight: 600; font-size: 0.875rem;">⚡ Subconverter 高级规则订阅</span>
                <span class="badge badge-info" style="font-size: 0.65rem;">
                  {{ currentSavedTemplateLabel }}
                </span>
                <span style="color: #64748b; font-size: 0.75rem;">
                  ({{ savedPrefs.target.toUpperCase() }} &middot; Emoji: {{ savedPrefs.emoji ? '开' : '关' }} &middot; UDP: {{ savedPrefs.udp ? '开' : '关' }})
                </span>
              </div>
              <div style="display: flex; gap: 0.5rem;">
                <button
                  @click="copySavedAdvancedSubURL(m.sub_token)"
                  class="btn btn-primary"
                  style="font-size: 0.75rem; padding: 0.25rem 0.6rem; background: #9333ea; border-color: #9333ea;"
                >
                  📋 复制 {{ savedPrefs.target.toUpperCase() }} 分流订阅
                </button>
                <button
                  @click="openSavedAdvancedSubQR(m.name, m.sub_token)"
                  class="btn btn-secondary"
                  style="font-size: 0.75rem; padding: 0.25rem 0.6rem;"
                >
                  📱 扫码
                </button>
              </div>
            </div>

            <!-- Clash 基础订阅 -->
            <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: #38bdf8; font-weight: 600; font-size: 0.875rem;">Clash (Mihomo) 原生订阅</span>
                <span style="color: #64748b; font-size: 0.75rem;">(推荐：Clash Verge Rev / Flclash / Stash)</span>
              </div>
              <div style="display: flex; gap: 0.5rem;">
                <button @click="copySubURL(m.sub_token, 'clash')" class="btn btn-primary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                  📋 复制 Clash 链接
                </button>
                <button @click="openSubQR(m.name, m.sub_token, 'clash')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                  📱 扫码
                </button>
              </div>
            </div>

            <!-- Sing-box 官方客户端订阅 -->
            <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: #34d399; font-weight: 600; font-size: 0.875rem;">Sing-box 官方客户端订阅</span>
                <span style="color: #64748b; font-size: 0.75rem;">(SFI / SFA / SFM 客户端)</span>
              </div>
              <div style="display: flex; gap: 0.5rem;">
                <button @click="copySubURL(m.sub_token, 'singbox')" class="btn btn-primary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                  📋 复制 Sing-box 链接
                </button>
                <button @click="openSubQR(m.name, m.sub_token, 'singbox')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                  📱 扫码
                </button>
              </div>
            </div>

            <!-- 通用 Base64 订阅 -->
            <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: #fbbf24; font-weight: 600; font-size: 0.875rem;">通用 Base64 / URI 订阅</span>
                <span style="color: #64748b; font-size: 0.75rem;">(Shadowrocket / v2rayN)</span>
              </div>
              <div style="display: flex; gap: 0.5rem;">
                <button @click="copySubURL(m.sub_token, 'base64')" class="btn btn-primary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                  📋 复制通用链接
                </button>
                <button @click="openSubQR(m.name, m.sub_token, 'base64')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                  📱 扫码
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ==================== 版块二：Subconverter 转换引擎与规则 ==================== -->
    <div v-else-if="activeSubSection === 'subconverter'">
      <!-- 未安装时的 Progressive Empty State 引导 -->
      <div v-if="!subStatus.installed" class="card" style="text-align: center; padding: 3rem 1.5rem;">
        <div style="font-size: 3rem; margin-bottom: 1rem;">🔄</div>
        <h2 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc; margin-bottom: 0.5rem;">
          Subconverter 订阅转换引擎尚未安装
        </h2>
        <p style="color: #94a3b8; font-size: 0.875rem; max-width: 640px; margin: 0 auto 1.75rem; line-height: 1.6;">
          Subconverter 能够将本面板管理的 Reality / Shadowsocks 等所有入站节点，自动转换为包含 ACL4SSR 策略组分流、自动测速、国内直连和广告反追踪的高级客户端配置，并兼容 Clash、Surge、Quantumult X、Loon 等数十种协议。
        </p>

        <div style="display: flex; justify-content: center; gap: 1rem; flex-wrap: wrap;">
          <router-link
            to="/components?install=subconverter"
            class="btn btn-primary"
            style="font-size: 0.9rem; padding: 0.6rem 1.4rem; text-decoration: none;"
          >
            🚀 前往「软件中心」一键安装
          </router-link>
          <button @click="fetchSubStatus" class="btn btn-secondary">
            🔄 刷新检测状态
          </button>
        </div>
      </div>

      <!-- 已安装时的完整配置中心 -->
      <div v-else style="display: flex; flex-direction: column; gap: 1.5rem;">
        <!-- 服务状态指示与控制条 -->
        <div class="card" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
          <div style="display: flex; align-items: center; gap: 1rem;">
            <span style="font-size: 2rem;">🔄</span>
            <div>
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <h3 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">Subconverter 订阅转换服务</h3>
                <span :class="['badge', subStatus.active ? 'badge-success' : 'badge-danger']">
                  {{ subStatus.active ? '🟢 运行中' : '🔴 已停止' }}
                </span>
                <span class="badge badge-secondary" style="font-size: 0.7rem; font-family: monospace;">
                  v{{ subStatus.installed_version || '0.9.0' }}
                </span>
              </div>
              <div style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.25rem;">
                本地监听：<span class="mono" style="color: #38bdf8;">127.0.0.1:{{ subStatus.port || 25500 }}</span>
                &middot; 运行资产目录：<span class="mono" style="color: #cbd5e1;">{{ subStatus.component_dir }}</span>
              </div>
            </div>
          </div>

          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap;">
            <button
              @click="toggleSubconverterService(subStatus.active ? 'stop' : 'start')"
              :class="['btn', subStatus.active ? 'btn-danger' : 'btn-success']"
              style="font-size: 0.825rem; padding: 0.4rem 0.8rem;"
            >
              {{ subStatus.active ? '⏹ 停止服务' : '▶ 启动服务' }}
            </button>
            <button
              @click="toggleSubconverterService('restart')"
              class="btn btn-secondary"
              style="font-size: 0.825rem; padding: 0.4rem 0.8rem;"
            >
              🔄 重启服务
            </button>
            <router-link
              to="/components"
              class="btn btn-secondary"
              style="font-size: 0.825rem; padding: 0.4rem 0.8rem; text-decoration: none;"
            >
              📜 查看终端日志
            </router-link>
          </div>
        </div>

        <!-- 在线转换生成器与实时调试工作台 -->
        <div class="card">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.75rem;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-size: 1.25rem;">⚡</span>
              <div>
                <h3 style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">在线订阅规则生成与测试工作台</h3>
                <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.15rem;">
                  选择目标客户端与远程分流规则集，即时生成并验证转换效果，支持保存为默认订阅偏好
                </p>
              </div>
            </div>

            <!-- 独立保存按钮 -->
            <button
              @click="saveWorkbenchPreferences"
              class="btn btn-primary"
              style="font-size: 0.825rem; padding: 0.45rem 0.9rem;"
              :disabled="savingWorkbench"
              type="button"
              title="将当前选择的客户端格式、规则模板与参数持久化保存为系统默认分流偏好"
            >
              <span>💾</span>
              <span>{{ savingWorkbench ? '正在保存...' : '保存为默认规则偏好' }}</span>
            </button>
          </div>

          <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 1rem; margin-bottom: 1.25rem;">
            <div>
              <label class="form-label">选择关联成员</label>
              <select v-model="tester.memberId" class="input">
                <option v-for="m in members" :key="m.id" :value="m.id">
                  {{ m.name }} ({{ m.sub_token.slice(0, 8) }}...)
                </option>
              </select>
            </div>

            <div>
              <label class="form-label">目标客户端格式 (Target)</label>
              <select v-model="tester.target" class="input">
                <option value="clash">Clash / Clash Verge / Mihomo (YAML)</option>
                <option value="singbox">Sing-box (JSON)</option>
                <option value="surge">Surge 4/5 (CONF)</option>
                <option value="quantumultx">Quantumult X (CONF)</option>
                <option value="loon">Loon (CONF)</option>
                <option value="shadowrocket">Shadowrocket 小火箭</option>
              </select>
            </div>

            <div>
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.35rem;">
                <label class="form-label" style="margin: 0;">规则集与分流模板 (Ruleset / Config)</label>
                <button
                  v-if="tester.template === 'config/custom_rules.ini'"
                  type="button"
                  @click="openCustomRulesModal"
                  class="btn btn-secondary"
                  style="font-size: 0.72rem; padding: 0.15rem 0.5rem; color: #38bdf8;"
                  title="查看与修改本地专属的 custom_rules.ini 规则文件"
                >
                  ✏️ 编辑本地规则内容
                </button>
              </div>
              <select v-model="tester.template" class="input">
                <option v-for="t in templates" :key="t.name" :value="t.config_value">
                  {{ t.label }} {{ t.recommended ? '★ 推荐' : '' }}
                </option>
                <option value="custom">🌐 外部在线规则集链接 (URL)...</option>
              </select>
            </div>
          </div>

          <!-- 本地专属规则选中时的轻量状态指示 (无侵入、不主动弹窗) -->
          <div
            v-if="tester.template === 'config/custom_rules.ini'"
            style="margin-bottom: 1.25rem; padding: 0.6rem 0.85rem; background: rgba(56, 189, 248, 0.08); border: 1px solid rgba(56, 189, 248, 0.25); border-radius: 0.375rem; display: flex; justify-content: space-between; align-items: center; font-size: 0.8rem; flex-wrap: wrap; gap: 0.5rem;"
          >
            <div style="display: flex; align-items: center; gap: 0.5rem; color: #bae6fd;">
              <span>✏️</span>
              <span>
                <strong>已选用本地私有分流规则</strong>：由 Subconverter 离线即时解析，无需依赖外部网络。
              </span>
            </div>
            <button
              type="button"
              @click="openCustomRulesModal"
              class="btn btn-secondary"
              style="font-size: 0.75rem; padding: 0.2rem 0.6rem; color: #38bdf8; border-color: rgba(56, 189, 248, 0.4);"
            >
              ✏️ 打开规则源码编辑器
            </button>
          </div>

          <!-- 自定义外部规则集 URL 输入框 -->
          <div v-if="tester.template === 'custom'" style="margin-bottom: 1.25rem;">
            <label class="form-label">自定义外部规则集链接 (URL)</label>
            <input
              v-model="tester.customTemplateUrl"
              type="text"
              class="input"
              placeholder="请输入规则集完整 URL，例如：https://raw.githubusercontent.com/.../ACL4SSR_Online.ini"
            />
          </div>

          <!-- 转换开关参数 -->
          <div style="display: flex; gap: 1.5rem; flex-wrap: wrap; margin-bottom: 1.25rem; font-size: 0.825rem; color: #cbd5e1;">
            <label style="display: inline-flex; align-items: center; gap: 0.4rem; cursor: pointer;">
              <input type="checkbox" v-model="tester.emoji" />
              <span>自动添加国家/地区 Emoji 旗帜</span>
            </label>
            <label style="display: inline-flex; align-items: center; gap: 0.4rem; cursor: pointer;">
              <input type="checkbox" v-model="tester.udp" />
              <span>强制开启 UDP 转发</span>
            </label>
            <label style="display: inline-flex; align-items: center; gap: 0.4rem; cursor: pointer;">
              <input type="checkbox" v-model="tester.scv" />
              <span>跳过证书校验 (Skip Cert Verify)</span>
            </label>
          </div>

          <!-- 生成的专属订阅 URL 与操作 -->
          <div style="background-color: #0b0f19; border: 1px solid #1e293b; padding: 0.85rem; border-radius: 0.5rem; margin-bottom: 1rem;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
              <span style="font-size: 0.775rem; color: #94a3b8; font-weight: 600;">生成的完整订阅链接 (经由主控安全代理分发)：</span>
              <div style="display: flex; gap: 0.5rem; flex-wrap: wrap;">
                <button
                  @click="saveWorkbenchPreferences"
                  class="btn btn-primary"
                  style="font-size: 0.75rem; padding: 0.2rem 0.6rem;"
                  :disabled="savingWorkbench"
                  title="将此套参数保存为默认"
                >
                  💾 保存为默认偏好
                </button>
                <button @click="copyTesterUrl" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.6rem;">
                  📋 复制链接
                </button>
                <button @click="openTesterQR" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.6rem;">
                  📱 扫码
                </button>
                <button @click="runLiveTest" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.6rem;" :disabled="testingPreview">
                  ⚡ {{ testingPreview ? '正在转换...' : '在线测试并预览' }}
                </button>
              </div>
            </div>
            <div class="mono" style="font-size: 0.8rem; color: #38bdf8; word-break: break-all;">
              {{ currentTesterSubURL }}
            </div>
          </div>

          <!-- 在线转换预览输出结果 (防卡顿安全窗口截断模式) -->
          <div v-if="previewResult" style="margin-top: 1rem;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; flex-wrap: wrap; gap: 0.5rem;">
              <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;">
                <span style="font-size: 0.8rem; color: #34d399; font-weight: 600;">
                  ✓ 转换成功 (HTTP 200 &middot; 共 {{ previewLines }} 行 &middot; {{ previewSizeKB }} KB)
                </span>
                <span v-if="isPreviewTruncated" class="badge badge-warning" style="font-size: 0.68rem; padding: 0.1rem 0.4rem;">
                  ⚡ 防卡顿安全模式：当前仅渲染前 {{ MAX_PREVIEW_LINES }} 行
                </span>
              </div>
              <div style="display: flex; gap: 0.4rem; flex-wrap: wrap;">
                <button
                  @click="downloadPreviewFile"
                  class="btn btn-secondary"
                  style="font-size: 0.725rem; padding: 0.15rem 0.5rem;"
                  title="直接下载为本地客户端配置文件"
                >
                  📥 下载配置
                </button>
                <button
                  @click="copyPreviewOutput"
                  class="btn btn-secondary"
                  style="font-size: 0.725rem; padding: 0.15rem 0.5rem;"
                  title="复制全量完整配置文本到剪贴板"
                >
                  📋 复制全量内容
                </button>
              </div>
            </div>

            <pre class="code-preview" style="max-height: 280px;">{{ displayedPreview }}</pre>

            <!-- 超大文件截断提示与展开按钮 -->
            <div
              v-if="isPreviewTruncated"
              style="margin-top: 0.4rem; padding: 0.5rem 0.75rem; background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.25); border-radius: 0.35rem; font-size: 0.75rem; color: #fbbf24; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;"
            >
              <span>
                ⚠️ 该订阅文件较大（共 {{ previewLines }} 行），已启用防卡顿截断预览。完整内容请点击上方「复制全量内容」或「下载配置」。
              </span>
              <button
                type="button"
                @click="showFullPreview = true"
                class="btn btn-secondary"
                style="font-size: 0.7rem; padding: 0.15rem 0.5rem; color: #f59e0b; border-color: rgba(245, 158, 11, 0.4);"
              >
                展开全部 {{ previewLines }} 行 (可能引起页面轻微卡顿)
              </button>
            </div>
            <div
              v-else-if="showFullPreview && previewLines > MAX_PREVIEW_LINES"
              style="margin-top: 0.4rem; text-align: right;"
            >
              <button
                type="button"
                @click="showFullPreview = false"
                class="btn btn-secondary"
                style="font-size: 0.7rem; padding: 0.15rem 0.5rem; color: #94a3b8;"
              >
                收起超长预览 (恢复防卡顿模式)
              </button>
            </div>
          </div>
        </div>

        <!-- 核心运行偏好与 pref.ini 在线编辑器 -->
        <div class="card">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.5rem;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-size: 1.25rem;">⚙️</span>
              <div>
                <h3 style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">Subconverter 核心偏好设置 (pref.ini)</h3>
                <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.15rem;">
                  修改后将保存至磁盘并自动重载守护进程生效
                </p>
              </div>
            </div>

            <button @click="savePreferences" class="btn btn-primary" :disabled="savingConfig">
              <span>💾</span>
              <span>{{ savingConfig ? '正在保存...' : '保存并重载配置' }}</span>
            </button>
          </div>

          <!-- 表单常用项 -->
          <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 1rem; margin-bottom: 1.25rem;">
            <div>
              <label class="form-label">绑定监听地址 (Listen)</label>
              <input v-model="subForm.listen" type="text" class="input" placeholder="127.0.0.1" />
              <p class="form-hint">推荐设置为 127.0.0.1，防止外部未授权盗用转换接口</p>
            </div>

            <div>
              <label class="form-label">监听端口 (Port)</label>
              <input v-model.number="subForm.port" type="number" class="input" placeholder="25500" />
            </div>

            <div>
              <label class="form-label">API 访问 Token (可选)</label>
              <input v-model="subForm.api_token" type="text" class="input" placeholder="留空则不开启密码鉴权" />
            </div>

            <div>
              <label class="form-label">排除节点过滤正则 (exclude_remarks)</label>
              <input v-model="subForm.exclude_remarks" type="text" class="input" placeholder="如 (剩余流量|过期时间)" />
            </div>
          </div>

          <!-- 高级 pref.ini 原始文本透视与编辑 -->
          <div style="margin-top: 1rem;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.4rem;">
              <label class="form-label" style="margin: 0;">完整 pref.ini 配置源码：</label>
              <button
                type="button"
                @click="showRawEditor = !showRawEditor"
                class="btn btn-secondary"
                style="font-size: 0.75rem; padding: 0.2rem 0.5rem;"
              >
                {{ showRawEditor ? '收起源码编辑器' : '展开源码编辑器' }}
              </button>
            </div>

            <div v-show="showRawEditor">
              <textarea
                v-model="subForm.raw_content"
                class="code-editor"
                rows="14"
                placeholder="[common]&#10;listen=127.0.0.1&#10;port=25500&#10;..."
              ></textarea>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ==================== 版块三：Cloudflare 优选 IP ==================== -->
    <div v-else-if="activeSubSection === 'cf_preferred'">
      <!-- 顶部统计概览 -->
      <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(210px, 1fr)); gap: 1rem; margin-bottom: 1.5rem;">
        <div class="card" style="display: flex; align-items: center; gap: 0.85rem; padding: 1rem;">
          <div style="font-size: 1.85rem;">⚡</div>
          <div>
            <div style="font-size: 0.775rem; color: #94a3b8; font-weight: 500;">优选 IP 节点数</div>
            <div style="font-size: 1.35rem; font-weight: 700; color: #f8fafc; margin-top: 0.2rem;">
              {{ cfPrefStatus.total_count }} <span style="font-size: 0.75rem; font-weight: 400; color: #94a3b8;">个可用</span>
            </div>
          </div>
        </div>

        <div class="card" style="display: flex; align-items: center; gap: 0.85rem; padding: 1rem;">
          <div style="font-size: 1.85rem;">⏱️</div>
          <div>
            <div style="font-size: 0.775rem; color: #94a3b8; font-weight: 500;">上次同步时间</div>
            <div style="font-size: 0.95rem; font-weight: 600; color: #f8fafc; margin-top: 0.35rem;">
              {{ formatDateTime(cfPrefStatus.last_fetch_at) || '尚未执行同步' }}
            </div>
          </div>
        </div>

        <div class="card" style="display: flex; align-items: center; gap: 0.85rem; padding: 1rem;">
          <div style="font-size: 1.85rem;">⏰</div>
          <div>
            <div style="font-size: 0.775rem; color: #94a3b8; font-weight: 500;">自动定时更新</div>
            <div style="font-size: 0.95rem; font-weight: 600; margin-top: 0.35rem;">
              <span v-if="cfPrefStatus.cron_enabled" style="color: #34d399;">
                ✓ 运行中 ({{ cfPrefStatus.cron_mode === 'daily' ? `每日 ${cfPrefStatus.cron_time}` : `每 ${cfPrefStatus.cron_interval}` }})
              </span>
              <span v-else style="color: #94a3b8;">未启用 (仅手动同步)</span>
            </div>
          </div>
        </div>

        <div class="card" style="display: flex; align-items: center; gap: 0.85rem; padding: 1rem;">
          <div style="font-size: 1.85rem;">📡</div>
          <div>
            <div style="font-size: 0.775rem; color: #94a3b8; font-weight: 500;">最近抓取状态</div>
            <div style="font-size: 0.875rem; font-weight: 600; margin-top: 0.35rem; word-break: break-all;">
              <span v-if="cfPrefStatus.last_fetch_status === 'success'" style="color: #34d399;">
                ✓ 成功获取 {{ cfPrefStatus.last_fetch_count }} 个节点
              </span>
              <span v-else-if="cfPrefStatus.last_fetch_status" style="color: #f87171;">
                {{ cfPrefStatus.last_fetch_status }}
              </span>
              <span v-else style="color: #94a3b8;">就绪</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 抓取源与定时更新调度配置 Card -->
      <div class="card" style="margin-bottom: 1.5rem;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.5rem;">
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span style="font-size: 1.25rem;">⚙️</span>
            <div>
              <h3 style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">优选 IP 抓取源与定时更新配置</h3>
              <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.15rem;">
                配置 EDGETUNNEL 优选订阅源与周期性自动同步，保持本地优选节点库最新
              </p>
            </div>
          </div>

          <button
            @click="saveCFPrefSettings"
            class="btn btn-primary"
            style="font-size: 0.825rem; padding: 0.45rem 0.9rem;"
            :disabled="savingCfSettings"
            type="button"
          >
            <span>💾</span>
            <span>{{ savingCfSettings ? '正在保存...' : '保存抓取与调度配置' }}</span>
          </button>
        </div>

        <div style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label class="form-label">
              优选订阅源 URL (EDGETUNNEL_SUB_URL)
            </label>
            <div style="display: flex; gap: 0.5rem;">
              <input
                v-model="cfPrefStatus.sub_url"
                type="text"
                class="input"
                placeholder="https://sub.19910417.xyz/sub?host={host}&uuid={uuid}"
              />
              <button
                @click="cfPrefStatus.sub_url = 'https://sub.19910417.xyz/sub?host={host}&uuid={uuid}'"
                class="btn btn-secondary"
                style="font-size: 0.75rem; white-space: nowrap;"
                type="button"
              >
                默认模板
              </button>
            </div>
            <p class="form-hint">
              优选订阅分发接口地址，支持直接填入完整订阅链接或带 <code>{host}</code>、<code>{uuid}</code> 占位符的模板
            </p>
          </div>

          <!-- 定时自动同步配置 -->
          <div style="background: rgba(15, 23, 42, 0.6); border: 1px solid #1e293b; border-radius: 0.5rem; padding: 1rem;">
            <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.75rem; margin-bottom: 0.75rem;">
              <label style="display: flex; align-items: center; gap: 0.6rem; cursor: pointer;">
                <input type="checkbox" v-model="cfPrefStatus.cron_enabled" />
                <span style="font-size: 0.9rem; font-weight: 600; color: #f8fafc;">
                  启用后台定时自动更新优选 IP
                </span>
              </label>

              <div v-if="cfPrefStatus.cron_enabled" style="display: flex; gap: 0.5rem; align-items: center;">
                <label style="display: flex; align-items: center; gap: 0.35rem; font-size: 0.8rem; color: #cbd5e1; cursor: pointer;">
                  <input type="radio" value="interval" v-model="cfPrefStatus.cron_mode" />
                  固定间隔模式
                </label>
                <label style="display: flex; align-items: center; gap: 0.35rem; font-size: 0.8rem; color: #cbd5e1; cursor: pointer;">
                  <input type="radio" value="daily" v-model="cfPrefStatus.cron_mode" />
                  每日固定时间点
                </label>
              </div>
            </div>

            <div v-if="cfPrefStatus.cron_enabled" style="display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; padding-top: 0.5rem; border-top: 1px dashed #334155;">
              <div v-if="cfPrefStatus.cron_mode === 'interval'" style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="font-size: 0.825rem; color: #94a3b8;">每隔多久自动拉取一次：</span>
                <select v-model="cfPrefStatus.cron_interval" class="input" style="width: 140px; padding: 0.3rem 0.6rem; font-size: 0.825rem;">
                  <option value="2h">每 2 小时</option>
                  <option value="6h">每 6 小时 (推荐)</option>
                  <option value="12h">每 12 小时</option>
                  <option value="24h">每 24 小时</option>
                </select>
              </div>

              <div v-else-if="cfPrefStatus.cron_mode === 'daily'" style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="font-size: 0.825rem; color: #94a3b8;">每日自动拉取时间点：</span>
                <input
                  v-model="cfPrefStatus.cron_time"
                  type="time"
                  class="input"
                  style="width: 130px; padding: 0.3rem 0.6rem; font-size: 0.825rem;"
                />
                <span style="font-size: 0.75rem; color: #64748b;">(推荐 04:00 凌晨低峰期)</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 本地持久化优选 IP 节点库 Card -->
      <div class="card">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.75rem;">
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span style="font-size: 1.25rem;">🌐</span>
            <div>
              <h3 style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">
                本地优选 IP 节点库
                <span style="font-size: 0.85rem; color: #94a3b8; font-weight: 400;">
                  (已持久化 {{ cfIps.length }} 个节点)
                </span>
              </h3>
            </div>
          </div>

          <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;">
            <button
              @click="manualFetchCFIps"
              class="btn btn-primary"
              style="font-size: 0.8rem; padding: 0.4rem 0.8rem;"
              :disabled="fetchingCf"
              type="button"
            >
              <span>⚡</span>
              <span>{{ fetchingCf ? '正在抓取中...' : '立即重新获取' }}</span>
            </button>

            <button
              @click="copyAllCFIpList"
              class="btn btn-secondary"
              style="font-size: 0.8rem; padding: 0.4rem 0.8rem;"
              :disabled="!cfIps.length"
              type="button"
              title="复制全部 IP:端口 列表"
            >
              📋 复制全部 IP
            </button>

            <button
              @click="copyAllCFVlessLinks"
              class="btn btn-secondary"
              style="font-size: 0.8rem; padding: 0.4rem 0.8rem;"
              :disabled="!cfIps.length"
              type="button"
              title="复制全部 VLESS 订阅链接"
            >
              🔗 复制订阅链接
            </button>

            <button
              @click="clearAllCFIps"
              class="btn btn-danger"
              style="font-size: 0.8rem; padding: 0.4rem 0.8rem;"
              :disabled="!cfIps.length"
              type="button"
            >
              🗑️ 清空列表
            </button>
          </div>
        </div>

        <!-- 搜索过滤 -->
        <div style="margin-bottom: 1rem;">
          <input
            v-model="cfSearchKeyword"
            @input="loadCFIps"
            type="text"
            class="input"
            placeholder="🔍 搜索优选 IP / 地区备注 (如 SG, JP, 443)..."
            style="font-size: 0.825rem;"
          />
        </div>

        <!-- 列表表格 -->
        <div v-if="loadingCfIps" style="text-align: center; padding: 2.5rem; color: #94a3b8;">
          正在加载优选 IP 节点列表...
        </div>

        <div v-else-if="cfIps.length === 0" style="text-align: center; padding: 3rem 1rem; color: #94a3b8; background: rgba(15, 23, 42, 0.4); border-radius: 0.5rem; border: 1px dashed #334155;">
          <div style="font-size: 2.2rem; margin-bottom: 0.5rem;">📡</div>
          <div style="font-size: 0.95rem; font-weight: 600; color: #cbd5e1; margin-bottom: 0.35rem;">
            本地暂无持久化的 Cloudflare 优选 IP
          </div>
          <p style="font-size: 0.8rem; max-width: 480px; margin: 0 auto 1.25rem; color: #64748b;">
            点击右上角「立即重新获取」按钮，系统将立即从 EDGETUNNEL_SUB_URL 获取最新优选节点并保存在本地数据库中
          </p>
          <button @click="manualFetchCFIps" class="btn btn-primary" :disabled="fetchingCf">
            ⚡ 立即获取优选 IP
          </button>
        </div>

        <div v-else style="overflow-x: auto;">
          <table class="table">
            <thead>
              <tr>
                <th style="width: 60px;">序号</th>
                <th>地区 / 节点名称</th>
                <th>优选 IP 地址</th>
                <th style="width: 80px;">端口</th>
                <th style="width: 160px;">更新时间</th>
                <th style="width: 160px; text-align: right;">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(item, idx) in cfIps" :key="item.id">
                <td style="color: #64748b; font-size: 0.8rem;">#{{ idx + 1 }}</td>
                <td>
                  <div style="display: flex; align-items: center; gap: 0.45rem;">
                    <span v-if="item.colo" class="badge badge-primary" style="font-size: 0.7rem; font-weight: 700;">
                      {{ item.colo }}
                    </span>
                    <span style="font-weight: 600; color: #f8fafc; font-size: 0.85rem;">
                      {{ item.name || item.tag }}
                    </span>
                  </div>
                </td>
                <td>
                  <code style="background: rgba(30, 41, 59, 0.7); padding: 0.2rem 0.45rem; border-radius: 0.3rem; font-size: 0.8rem; color: #38bdf8;">
                    {{ item.ip }}
                  </code>
                </td>
                <td style="color: #cbd5e1; font-size: 0.825rem;">{{ item.port }}</td>
                <td style="color: #94a3b8; font-size: 0.75rem;">{{ formatDateTime(item.updated_at) }}</td>
                <td style="text-align: right;">
                  <div style="display: flex; justify-content: flex-end; gap: 0.35rem;">
                    <button
                      @click="copySingleCFIp(item.ip, item.port)"
                      class="btn btn-secondary"
                      style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                      title="复制 IP:端口"
                    >
                      复制 IP
                    </button>
                    <button
                      v-if="item.raw_link"
                      @click="copySingleCFLink(item.raw_link)"
                      class="btn btn-secondary"
                      style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                      title="复制 VLESS 链接"
                    >
                      链接
                    </button>
                    <button
                      @click="deleteSingleCFIp(item.id)"
                      class="btn btn-danger"
                      style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                      title="删除单条"
                    >
                      ✕
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- ==================== 版块四：节点清单 (按节点主机分组) ==================== -->
    <div v-if="activeSubSection === 'inventory'">
      <!-- 控制与操作栏 -->
      <div class="card" style="margin-bottom: 1.25rem;">
        <div style="display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 1rem;">
          <!-- 左侧：成员凭据切换与搜索 -->
          <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; flex: 1;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-size: 0.85rem; color: #94a3b8; white-space: nowrap;">订阅凭据成员:</span>
              <select
                v-model="inventoryMemberId"
                @change="loadInventory"
                class="input"
                style="padding: 0.35rem 0.75rem; font-size: 0.85rem; min-width: 140px;"
              >
                <option v-for="m in members" :key="m.id" :value="m.id">
                  {{ m.name }}
                </option>
              </select>
            </div>

            <div style="position: relative; flex: 1; max-width: 320px;">
              <input
                v-model="inventorySearchKeyword"
                type="text"
                placeholder="搜索节点名 / 主机 / 标签 / 端口 / 协议..."
                class="input"
                style="padding: 0.35rem 0.75rem; font-size: 0.85rem; width: 100%;"
              />
            </div>

            <button
              @click="loadInventory"
              :disabled="loadingInventory"
              class="btn btn-secondary"
              style="font-size: 0.8rem; padding: 0.35rem 0.75rem;"
              title="重新抓取全集群最新节点状态与配置"
            >
              {{ loadingInventory ? '⏳ 刷新中...' : '🔄 刷新' }}
            </button>
          </div>

          <!-- 右侧：概览与全局批量操作 -->
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <div v-if="inventoryData" style="display: flex; gap: 0.5rem; font-size: 0.8rem; color: #94a3b8;">
              <span class="badge badge-secondary">🖥️ {{ inventoryData.total_nodes }} 台主机</span>
              <span class="badge badge-primary">🌐 {{ inventoryData.total_inbounds }} 个入站节点</span>
            </div>
            <button
              @click="copyAllInventoryLinks"
              class="btn btn-primary"
              style="font-size: 0.8rem; padding: 0.4rem 0.85rem;"
            >
              📋 复制全集群所有链接
            </button>
          </div>
        </div>
      </div>

      <!-- 加载中状态 -->
      <div v-if="loadingInventory && !inventoryData" class="card" style="text-align: center; padding: 3rem 1rem;">
        <div style="color: #94a3b8; font-size: 0.95rem;">正在聚合多节点入站配置与实时状态...</div>
      </div>

      <!-- 空状态 -->
      <div v-else-if="filteredGroups.length === 0" class="card" style="text-align: center; padding: 3rem 1rem;">
        <div style="font-size: 2.5rem; margin-bottom: 0.75rem;">📭</div>
        <div style="font-size: 1rem; font-weight: 600; color: #f1f5f9; margin-bottom: 0.35rem;">
          {{ inventorySearchKeyword ? '未找到匹配的节点或主机' : '全集群暂无可用入站节点' }}
        </div>
        <p style="color: #64748b; font-size: 0.85rem;">
          {{ inventorySearchKeyword ? '请尝试更换关键词搜索' : '请先在「业务配置 -> Sing-box 核心 -> 入站节点」中添加入站' }}
        </p>
      </div>

      <!-- 分组展示节点清单 -->
      <div v-else style="display: flex; flex-direction: column; gap: 1.25rem;">
        <div
          v-for="group in filteredGroups"
          :key="group.node_id"
          class="card"
          style="padding: 0; overflow: hidden; border: 1px solid #1e293b;"
        >
          <!-- 节点主机头部 -->
          <div
            style="display: flex; justify-content: space-between; align-items: center; padding: 0.85rem 1.25rem; background: #0f172a; border-bottom: 1px solid #1e293b; cursor: pointer;"
            @click="toggleGroupCollapse(group.node_id)"
          >
            <!-- 左侧：主机标识与属性 -->
            <div style="display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap;">
              <span style="font-size: 0.9rem; color: #64748b; transition: transform 0.2s;" :style="{ transform: collapsedGroups[group.node_id] ? 'rotate(-90deg)' : 'rotate(0deg)' }">
                ▼
              </span>
              <span style="font-size: 1.05rem; font-weight: 600; color: #f8fafc;">
                🖥️ {{ group.node_name }}
              </span>
              <span v-if="group.sub_prefix" class="badge" style="background: rgba(16, 185, 129, 0.15); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.3); font-size: 0.72rem;">
                🏷️ 前缀: {{ group.sub_prefix }}
              </span>

              <!-- 主控 / 被纳管节点标签 -->
              <span v-if="group.is_master" class="badge badge-primary" style="font-size: 0.7rem;">
                ⭐ 本机主控
              </span>
              <span v-else class="badge badge-secondary" style="font-size: 0.7rem;">
                🌐 纳管节点
              </span>

              <!-- 在线状态标签 -->
              <span
                v-if="group.status === 'online'"
                class="badge badge-success"
                style="font-size: 0.7rem;"
              >
                🟢 在线 <span v-if="group.latency_ms > 0">({{ group.latency_ms }}ms)</span>
              </span>
              <span
                v-else-if="group.status === 'offline'"
                class="badge badge-danger"
                style="font-size: 0.7rem;"
              >
                🔴 离线
              </span>
              <span
                v-else
                class="badge badge-warning"
                style="font-size: 0.7rem;"
              >
                ⚠️ 异常
              </span>

              <span v-if="group.region" class="badge badge-secondary" style="font-size: 0.7rem;">
                📍 {{ group.region }}
              </span>

              <span style="font-family: 'JetBrains Mono', monospace; font-size: 0.8rem; color: #64748b;">
                {{ group.host }}
              </span>
            </div>

            <!-- 右侧：该主机快捷批量操作 -->
            <div style="display: flex; align-items: center; gap: 0.5rem;" @click.stop>
              <span class="badge badge-secondary" style="font-size: 0.75rem;">
                {{ group.inbounds.length }} 个入站
              </span>
              <button
                v-if="group.inbounds.length > 0"
                @click="copyGroupLinks(group)"
                class="btn btn-secondary"
                style="font-size: 0.75rem; padding: 0.25rem 0.6rem;"
                title="复制该主机下的所有入站链接"
              >
                📋 复制该主机全部链接
              </button>
            </div>
          </div>

          <!-- 展开内容区域 -->
          <div v-show="!collapsedGroups[group.node_id]">
            <!-- 异常告警 -->
            <div
              v-if="group.error"
              style="padding: 0.75rem 1.25rem; background: rgba(239, 68, 68, 0.1); border-bottom: 1px solid rgba(239, 68, 68, 0.2); color: #fca5a5; font-size: 0.8rem;"
            >
              ⚠️ 连接该节点异常: {{ group.error }}
            </div>

            <!-- 主机内无入站 -->
            <div
              v-if="group.inbounds.length === 0"
              style="padding: 2rem; text-align: center; color: #64748b; font-size: 0.85rem;"
            >
              该主机暂未配置任何入站规则
            </div>

            <!-- 入站列表表格 -->
            <div v-else class="table-container" style="border: none; border-radius: 0;">
              <table>
                <thead>
                  <tr>
                    <th style="width: 70px;">状态</th>
                    <th>节点标识 (Tag)</th>
                    <th>协议类型</th>
                    <th>监听端口 / 伪装网络</th>
                    <th>SNI / 路径</th>
                    <th>客户端连接链接</th>
                    <th style="width: 220px; text-align: right;">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="item in group.inbounds" :key="item.inbound_id">
                    <td>
                      <span
                        :class="['badge', item.enabled ? 'badge-success' : 'badge-danger']"
                        style="font-size: 0.7rem;"
                      >
                        {{ item.enabled ? '已启用' : '已停用' }}
                      </span>
                    </td>
                    <td>
                      <span style="font-weight: 600; color: #f8fafc; font-size: 0.85rem;">
                        {{ item.tag }}
                      </span>
                    </td>
                    <td>
                      <span class="badge badge-primary" style="font-size: 0.7rem;">
                        {{ item.protocol_label || item.protocol }}
                      </span>
                    </td>
                    <td>
                      <div style="font-family: 'JetBrains Mono', monospace; font-size: 0.8rem;">
                        <span style="color: #38bdf8;">{{ item.port }}</span>
                        <span v-if="item.protocol === 'vless-ws'" style="color: #64748b; margin-left: 0.35rem; font-size: 0.725rem;">
                          (WS)
                        </span>
                        <span v-else-if="item.protocol === 'vless-reality'" style="color: #64748b; margin-left: 0.35rem; font-size: 0.725rem;">
                          (TCP Reality)
                        </span>
                        <span v-else-if="item.protocol === 'socks'" style="color: #64748b; margin-left: 0.35rem; font-size: 0.725rem;">
                          (SOCKS5)
                        </span>
                      </div>
                    </td>
                    <td>
                      <div v-if="item.server_name" style="font-family: 'JetBrains Mono', monospace; font-size: 0.75rem; color: #94a3b8;">
                        SNI: {{ item.server_name }}
                      </div>
                      <div v-if="item.path" style="font-family: 'JetBrains Mono', monospace; font-size: 0.725rem; color: #64748b;">
                        Path: {{ item.path }}
                      </div>
                      <div v-if="item.tunnel_domain && item.tunnel_domain !== item.server_name" style="font-family: 'JetBrains Mono', monospace; font-size: 0.725rem; color: #64748b;">
                        Host: {{ item.tunnel_domain }}
                      </div>
                      <span v-if="!item.server_name && !item.path && !item.tunnel_domain" style="color: #475569; font-size: 0.75rem;">-</span>
                    </td>
                    <td>
                      <div
                        v-if="item.link"
                        style="max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: 'JetBrains Mono', monospace; font-size: 0.75rem; color: #94a3b8; background: #0b0f19; padding: 0.2rem 0.4rem; border-radius: 0.25rem;"
                        :title="item.link"
                      >
                        {{ item.link }}
                      </div>
                      <span v-else style="color: #64748b; font-size: 0.75rem;">无连接链接</span>
                    </td>
                    <td>
                      <div style="display: flex; justify-content: flex-end; gap: 0.35rem; flex-wrap: wrap;">
                        <button
                          v-if="item.link"
                          @click="copyInboundLink(item.link, item.tag)"
                          class="btn btn-secondary"
                          style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                          title="复制客户端连接链接 (URL)"
                        >
                          📋 链接
                        </button>
                        <button
                          v-if="item.link"
                          @click="openInboundQR(item.link, item.tag)"
                          class="btn btn-secondary"
                          style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                          title="查看二维码扫码导入"
                        >
                          📱 扫码
                        </button>
                        <button
                          v-if="item.clash_snippet"
                          @click="copyInboundClashSnippet(item.clash_snippet, item.tag)"
                          class="btn btn-secondary"
                          style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                          title="复制该节点 Clash 代理节点 YAML 配置片段"
                        >
                          📄 Clash
                        </button>
                        <button
                          @click="copyToClipboard(`${item.server_host || group.host}:${item.port}`, '节点地址')"
                          class="btn btn-secondary"
                          style="font-size: 0.7rem; padding: 0.2rem 0.45rem;"
                          title="复制 主机:端口"
                        >
                          🌐 地址
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 成员添加与编辑弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem;">
          {{ modal.isEdit ? '编辑成员配置' : '添加团队成员' }}
        </h3>

        <form @submit.prevent="saveMember" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
              成员名称 / 用户标识 (Name)
            </label>
            <input v-model="form.name" class="input" required placeholder="如 Jason-iPhone 或 团队成员A" />
            <div style="font-size: 0.725rem; color: #64748b; margin-top: 0.25rem;">
              对应各入站节点（包括 VLESS-WS 与 Reality）配置中 <code style="color: #cbd5e1;">users[].name</code> 属性。
            </div>
          </div>

          <div v-if="modal.isEdit">
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
              专属连接凭据 (UUID)
            </label>
            <input v-model="form.uuid" class="input" required placeholder="标准 UUID 格式" />
            <div style="font-size: 0.725rem; color: #64748b; margin-top: 0.25rem;">
              修改后各入站中该成员的连接密码将同步更新，需重新分发或更新客户端。
            </div>
          </div>

          <div v-if="modal.isEdit">
            <label style="display: flex; align-items: center; gap: 0.5rem; font-size: 0.85rem; color: #cbd5e1; cursor: pointer;">
              <input type="checkbox" v-model="form.enabled" />
              <span>启用此成员 (停用后该成员无法通过各入站连接)</span>
            </label>
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">
              {{ modal.isEdit ? '保存并同步节点配置' : '添加并分配专属订阅' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- 订阅二维码弹窗 -->
    <div v-if="qrModal.show" class="modal-overlay" @click.self="qrModal.show = false">
      <div class="modal-content" style="text-align: center;">
        <h3 style="font-size: 1.125rem; font-weight: 600; margin-bottom: 1rem;">
          {{ qrModal.title }}
        </h3>

        <div style="background-color: white; padding: 1rem; border-radius: 0.5rem; display: inline-block; margin-bottom: 1rem;">
          <qrcode-vue :value="qrModal.url" :size="220" level="M" />
        </div>

        <p style="color: #94a3b8; font-size: 0.8rem; margin-bottom: 0.5rem;">打开客户端订阅中心扫码导入</p>

        <textarea
          :value="qrModal.url"
          readonly
          class="input"
          rows="2"
          style="font-size: 0.75rem; margin-bottom: 1rem; word-break: break-all;"
        ></textarea>

        <div style="display: flex; justify-content: center; gap: 0.75rem;">
          <button @click="copyModalUrl" class="btn btn-primary">复制订阅链接</button>
          <button @click="qrModal.show = false" class="btn btn-secondary">关闭</button>
        </div>
      </div>
    </div>

    <!-- 本地自定义规则模板 (custom_rules.ini) 编辑弹窗 -->
    <div v-if="customRulesModal.show" class="modal-overlay" @click.self="customRulesModal.show = false">
      <div class="modal-content" style="max-width: 880px; max-height: 90vh; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; flex-wrap: wrap; gap: 0.5rem;">
          <div>
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-size: 1.25rem;">✏️</span>
              <h3 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">本地自定义分流规则 (custom_rules.ini)</h3>
              <span v-if="customRulesModal.exists" class="badge badge-success" style="font-size: 0.7rem;">已落盘</span>
              <span v-else class="badge badge-warning" style="font-size: 0.7rem;">推荐范例 (未保存)</span>
            </div>
            <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem; font-family: monospace;">
              保存路径: {{ customRulesModal.path || 'components/subconverter/config/custom_rules.ini' }}
            </div>
          </div>

          <div style="display: flex; gap: 0.5rem; flex-wrap: wrap;">
            <button @click="resetToDefaultCustomRules" class="btn btn-secondary" style="font-size: 0.775rem; padding: 0.35rem 0.75rem;" type="button">
              🔄 载入官方范例
            </button>
            <button @click="copyCustomRules" class="btn btn-secondary" style="font-size: 0.775rem; padding: 0.35rem 0.75rem;" type="button">
              📋 复制内容
            </button>
            <button @click="customRulesModal.show = false" class="btn btn-secondary" style="font-size: 0.775rem; padding: 0.35rem 0.75rem;" type="button">
              ✕ 关闭
            </button>
          </div>
        </div>

        <p style="color: #94a3b8; font-size: 0.8rem; margin-bottom: 0.75rem; line-height: 1.4;">
          此模板由 Subconverter 直接离线解析，支持自由编写策略组（<code style="color: #38bdf8;">custom_group</code>）与规则集分流（<code style="color: #38bdf8;">ruleset</code>）。保存后订阅转换秒级生效，无需借助任何第三方外部网络。
        </p>

        <div style="flex: 1; display: flex; flex-direction: column; min-height: 380px;">
          <textarea
            v-model="customRulesModal.content"
            class="code-editor"
            style="flex: 1; min-height: 380px; font-family: 'JetBrains Mono', monospace; font-size: 0.825rem; line-height: 1.45; resize: vertical;"
            placeholder="[custom]&#10;custom_group=🚀 节点选择`select`.*&#10;ruleset=..."
          ></textarea>
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
          <button type="button" @click="customRulesModal.show = false" class="btn btn-secondary">取消</button>
          <button
            type="button"
            @click="saveCustomRules"
            class="btn btn-primary"
            :disabled="customRulesModal.saving"
          >
            <span>💾</span>
            <span>{{ customRulesModal.saving ? '正在保存...' : '保存并设为当前模板' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, reactive, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import QrcodeVue from 'qrcode.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

const route = useRoute()
const { toast } = useToast()
const { showConfirm } = useConfirm()

const activeSubSection = ref<'members' | 'subconverter' | 'cf_preferred' | 'inventory'>('members')
const members = ref<any[]>([])
const loading = ref(true)

// Subconverter 状态与配置
const subStatus = reactive({
  installed: false,
  installed_version: '',
  active: false,
  running_status: '',
  port: 25500,
  component_dir: '',
  config_path: '',
})

const subForm = reactive({
  listen: '127.0.0.1',
  port: 25500,
  api_token: '',
  exclude_remarks: '',
  include_remarks: '',
  default_external_config: '',
  raw_content: '',
})

const showRawEditor = ref(false)
const savingConfig = ref(false)
const savingWorkbench = ref(false)
const templates = ref<any[]>([])

// 保存的全局规则与订阅偏好
const savedPrefs = reactive({
  target: 'clash',
  template: 'config/ACL4SSR_Online.ini',
  custom_template_url: '',
  emoji: true,
  udp: true,
  scv: true,
})

const currentSavedTemplateLabel = computed(() => {
  if (savedPrefs.template === 'custom') {
    return '自定义规则集'
  }
  const match = templates.value.find((t: any) => t.config_value === savedPrefs.template)
  return match ? match.label : 'ACL4SSR 经典在线分流版'
})

// 实时测试调试器
const tester = reactive({
  memberId: 0,
  target: 'clash',
  template: 'config/ACL4SSR_Online.ini',
  customTemplateUrl: '',
  emoji: true,
  udp: true,
  scv: true,
})

// 防卡顿超大文本安全渲染机制
const previewResult = shallowRef('')
const showFullPreview = ref(false)
const MAX_PREVIEW_LINES = 600

const previewLines = computed(() => {
  if (!previewResult.value) return 0
  let count = 1
  const len = previewResult.value.length
  for (let i = 0; i < len; i++) {
    if (previewResult.value.charCodeAt(i) === 10) count++
  }
  return count
})

const previewSizeKB = computed(() => {
  if (!previewResult.value) return '0.0'
  return (previewResult.value.length / 1024).toFixed(1)
})

const isPreviewTruncated = computed(() => {
  return !showFullPreview.value && previewLines.value > MAX_PREVIEW_LINES
})

const displayedPreview = computed(() => {
  if (!previewResult.value) return ''
  if (!isPreviewTruncated.value) return previewResult.value

  let lineCount = 0
  let cutIndex = 0
  const len = previewResult.value.length
  for (let i = 0; i < len; i++) {
    if (previewResult.value.charCodeAt(i) === 10) {
      lineCount++
      if (lineCount >= MAX_PREVIEW_LINES) {
        cutIndex = i
        break
      }
    }
  }
  if (cutIndex > 0) {
    return previewResult.value.slice(0, cutIndex)
  }
  return previewResult.value
})

const testingPreview = ref(false)

const modal = reactive({
  show: false,
  isEdit: false,
  id: 0,
})

const form = reactive({
  name: '',
  uuid: '',
  enabled: true,
})

function openCreateMemberModal() {
  modal.isEdit = false
  modal.id = 0
  form.name = ''
  form.uuid = ''
  form.enabled = true
  modal.show = true
}

function openEditMemberModal(m: any) {
  modal.isEdit = true
  modal.id = m.id
  form.name = m.name
  form.uuid = m.uuid
  form.enabled = !!m.enabled
  modal.show = true
}

const qrModal = reactive({
  show: false,
  title: '',
  url: '',
})

const customRulesModal = reactive({
  show: false,
  saving: false,
  path: '',
  exists: false,
  content: '',
})

async function openCustomRulesModal() {
  try {
    const res = await api.get('/subconverter/custom-rules')
    customRulesModal.path = res.data.path || ''
    customRulesModal.exists = !!res.data.exists
    customRulesModal.content = res.data.content || ''
    customRulesModal.show = true
  } catch (err: any) {
    toast.error('读取自定义规则模板失败: ' + (err.response?.data?.error || err.message))
  }
}

async function saveCustomRules() {
  customRulesModal.saving = true
  try {
    const res = await api.post('/subconverter/custom-rules', {
      content: customRulesModal.content,
    })
    toast.success(res.data.message || '本地自定义规则模板已成功保存！')
    customRulesModal.exists = true
    customRulesModal.show = false

    await fetchTemplates()
    tester.template = 'config/custom_rules.ini'
  } catch (err: any) {
    toast.error('保存自定义规则模板失败: ' + (err.response?.data?.error || err.message))
  } finally {
    customRulesModal.saving = false
  }
}

async function resetToDefaultCustomRules() {
  try {
    const res = await api.get('/subconverter/custom-rules')
    if (res.data.content) {
      customRulesModal.content = res.data.content
      toast.success('已载入推荐分流规则范例')
    }
  } catch {
    // ignore
  }
}

async function copyCustomRules() {
  await copyToClipboard(customRulesModal.content, '自定义规则内容')
}

// 计算当前选中的测试订阅 URL
const currentTesterSubURL = computed(() => {
  const m = members.value.find((item) => item.id === tester.memberId)
  const token = m ? m.sub_token : (members.value[0]?.sub_token || 'TOKEN')
  const host = window.location.origin

  const params = new URLSearchParams()
  params.set('engine', 'subconverter')
  params.set('target', tester.target)

  const configVal = tester.template === 'custom' ? tester.customTemplateUrl.trim() : tester.template
  if (configVal) {
    params.set('config', configVal)
  }
  if (tester.emoji) params.set('emoji', 'true')
  if (tester.udp) params.set('udp', 'true')
  if (tester.scv) params.set('scv', 'true')

  return `${host}/sub/${token}?${params.toString()}`
})

async function fetchSubStatus() {
  try {
    const res = await api.get('/subconverter/status')
    if (res.data) {
      subStatus.installed = res.data.installed
      subStatus.installed_version = res.data.installed_version
      subStatus.active = res.data.active
      subStatus.running_status = res.data.running_status
      subStatus.port = res.data.port
      subStatus.component_dir = res.data.component_dir
      subStatus.config_path = res.data.config_path
    }
  } catch {
    // 忽略未安装或请求失败
  }
}

async function fetchSubConfig() {
  try {
    const res = await api.get('/subconverter/config')
    if (res.data) {
      subForm.listen = res.data.listen || '127.0.0.1'
      subForm.port = res.data.port || 25500
      subForm.api_token = res.data.api_token || ''
      subForm.exclude_remarks = res.data.exclude_remarks || ''
      subForm.include_remarks = res.data.include_remarks || ''
      subForm.default_external_config = res.data.default_external_config || ''
      subForm.raw_content = res.data.raw_content || ''
    }
  } catch {
    // ignore
  }
}

async function fetchTemplates() {
  try {
    const res = await api.get('/subconverter/templates')
    templates.value = res.data || []
    if (templates.value.length > 0 && (!tester.template || tester.template === 'config/ACL4SSR_Online.ini')) {
      const defaultTpl = templates.value.find((t: any) => t.recommended) || templates.value[0]
      tester.template = defaultTpl.config_value || defaultTpl.ConfigValue || 'config/ACL4SSR_Online.ini'
    }
  } catch {
    // ignore
  }
}

async function savePreferences() {
  savingConfig.value = true
  try {
    const res = await api.post('/subconverter/config', subForm)
    toast.success(res.data.message || '配置已成功保存！')
    await fetchSubStatus()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
    await fetchSubConfig()
    await fetchSubStatus()
  } finally {
    savingConfig.value = false
  }
}

async function toggleSubconverterService(action: string) {
  try {
    await api.post(`/components/subconverter/action`, { action })
    const actionLabel = action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'
    toast.success(`已下发 ${actionLabel} Subconverter 指令`)
    await fetchSubStatus()
  } catch (err: any) {
    toast.error('操作失败: ' + (err.response?.data?.error || err.message))
  }
}

async function runLiveTest() {
  testingPreview.value = true
  previewResult.value = ''
  showFullPreview.value = false
  try {
    const res = await fetch(currentTesterSubURL.value)
    if (!res.ok) {
      const errText = await res.text()
      throw new Error(`HTTP ${res.status}: ${errText}`)
    }
    const text = await res.text()
    previewResult.value = text
    toast.success('在线订阅转换测试成功！')
  } catch (err: any) {
    toast.error('转换失败: ' + err.message)
  } finally {
    testingPreview.value = false
  }
}

function copyTesterUrl() {
  copyToClipboard(currentTesterSubURL.value, 'Subconverter 订阅链接')
}

function openTesterQR() {
  qrModal.title = `Subconverter - ${tester.target.toUpperCase()} 订阅二维码`
  qrModal.url = currentTesterSubURL.value
  qrModal.show = true
}

function copyPreviewOutput() {
  copyToClipboard(previewResult.value, '转换结果完整配置')
}

function downloadPreviewFile() {
  if (!previewResult.value) {
    toast.warning('暂无预览内容可供下载')
    return
  }
  const extMap: Record<string, string> = {
    clash: 'yaml',
    singbox: 'json',
    surge: 'conf',
    quantumultx: 'conf',
    loon: 'conf',
    shadowrocket: 'txt',
  }
  const ext = extMap[tester.target] || 'txt'
  const blob = new Blob([previewResult.value], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `subscription-${tester.target}-${new Date().toISOString().slice(0, 10)}.${ext}`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  toast.success(`已开始下载 ${a.download}`)
}

async function loadMembers() {
  loading.value = true
  try {
    const res = await api.get('/members')
    members.value = res.data || []
    if (members.value.length > 0 && tester.memberId === 0) {
      tester.memberId = members.value[0].id
    }
  } catch (err: any) {
    toast.error('加载成员列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

function getSubFullURL(subToken: string, target: string): string {
  const host = window.location.origin
  return `${host}/sub/${subToken}?target=${target}`
}

function copySubURL(subToken: string, target: string) {
  const url = getSubFullURL(subToken, target)
  copyToClipboard(url, `${target.toUpperCase()} 订阅链接`)
}

function copySavedAdvancedSubURL(subToken: string) {
  const host = window.location.origin
  const tpl = savedPrefs.template === 'custom' ? savedPrefs.custom_template_url : savedPrefs.template
  const params = new URLSearchParams()
  params.set('engine', 'subconverter')
  params.set('target', savedPrefs.target || 'clash')
  if (tpl) params.set('config', tpl)
  if (savedPrefs.emoji) params.set('emoji', 'true')
  if (savedPrefs.udp) params.set('udp', 'true')
  if (savedPrefs.scv) params.set('scv', 'true')

  const url = `${host}/sub/${subToken}?${params.toString()}`
  copyToClipboard(url, `Subconverter ${savedPrefs.target.toUpperCase()} 分流订阅链接`)
}

function openSavedAdvancedSubQR(memberName: string, subToken: string) {
  const host = window.location.origin
  const tpl = savedPrefs.template === 'custom' ? savedPrefs.custom_template_url : savedPrefs.template
  const params = new URLSearchParams()
  params.set('engine', 'subconverter')
  params.set('target', savedPrefs.target || 'clash')
  if (tpl) params.set('config', tpl)
  if (savedPrefs.emoji) params.set('emoji', 'true')
  if (savedPrefs.udp) params.set('udp', 'true')
  if (savedPrefs.scv) params.set('scv', 'true')

  qrModal.title = `${memberName} - Subconverter ${savedPrefs.target.toUpperCase()} 分流订阅`
  qrModal.url = `${host}/sub/${subToken}?${params.toString()}`
  qrModal.show = true
}

async function fetchWorkbenchPreferences() {
  try {
    const res = await api.get('/subconverter/preferences')
    if (res.data) {
      savedPrefs.target = res.data.target || 'clash'
      savedPrefs.template = res.data.template || 'config/ACL4SSR_Online.ini'
      savedPrefs.custom_template_url = res.data.custom_template_url || ''
      savedPrefs.emoji = res.data.emoji !== false
      savedPrefs.udp = res.data.udp !== false
      savedPrefs.scv = res.data.scv !== false

      // 同步给测试器初始化
      tester.target = savedPrefs.target
      tester.template = savedPrefs.template
      tester.customTemplateUrl = savedPrefs.custom_template_url
      tester.emoji = savedPrefs.emoji
      tester.udp = savedPrefs.udp
      tester.scv = savedPrefs.scv
    }
  } catch {
    // ignore
  }
}

async function saveWorkbenchPreferences() {
  savingWorkbench.value = true
  try {
    const payload = {
      target: tester.target,
      template: tester.template,
      custom_template_url: tester.customTemplateUrl,
      emoji: tester.emoji,
      udp: tester.udp,
      scv: tester.scv,
    }
    const res = await api.post('/subconverter/preferences', payload)
    savedPrefs.target = payload.target
    savedPrefs.template = payload.template
    savedPrefs.custom_template_url = payload.custom_template_url
    savedPrefs.emoji = payload.emoji
    savedPrefs.udp = payload.udp
    savedPrefs.scv = payload.scv

    toast.success(res.data.message || '已成功将当前规则集与参数保存为默认偏好！')
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  } finally {
    savingWorkbench.value = false
  }
}

function copyAdvancedSubURL(subToken: string, target: string, config: string) {
  const host = window.location.origin
  const url = `${host}/sub/${subToken}?engine=subconverter&target=${target}&config=${encodeURIComponent(config)}&emoji=true&udp=true`
  copyToClipboard(url, `Subconverter ACL4SSR 订阅链接`)
}

function openAdvancedSubQR(memberName: string, subToken: string, target: string, config: string) {
  const host = window.location.origin
  qrModal.title = `${memberName} - Subconverter ACL4SSR 分流订阅`
  qrModal.url = `${host}/sub/${subToken}?engine=subconverter&target=${target}&config=${encodeURIComponent(config)}&emoji=true&udp=true`
  qrModal.show = true
}

function copyModalUrl() {
  copyToClipboard(qrModal.url, '订阅链接')
}

function openSubQR(memberName: string, subToken: string, target: string) {
  qrModal.title = `${memberName} - ${target.toUpperCase()} 订阅二维码`
  qrModal.url = getSubFullURL(subToken, target)
  qrModal.show = true
}

const syncingMembers = ref(false)

async function broadcastSyncMembers() {
  if (syncingMembers.value) return
  syncingMembers.value = true
  try {
    const res = await api.post('/members/broadcast-sync')
    const { total, success, failed, errors } = res.data
    if (total === 0) {
      toast.success('当前仅有主控单机，无需额外同步')
    } else if (failed === 0) {
      toast.success(`全网同步完成！已将凭据推送到全部 ${success} 个受管节点并触发热重载`)
    } else {
      const detail = errors && errors.length ? `(${errors.join('; ')})` : ''
      toast.warning(`同步完成：成功 ${success} 个节点，失败 ${failed} 个节点 ${detail}`)
    }
  } catch (err: any) {
    toast.error('广播同步失败: ' + (err.response?.data?.error || err.message))
  } finally {
    syncingMembers.value = false
  }
}

async function saveMember() {
  try {
    if (modal.isEdit) {
      await api.put(`/members/${modal.id}`, {
        name: form.name,
        uuid: form.uuid,
        enabled: form.enabled,
      })
      toast.success(`成员 ${form.name} 更新成功，全网受管节点已广播同步`)
    } else {
      await api.post('/members', {
        name: form.name,
        enabled: true,
      })
      toast.success(`成员 ${form.name} 创建成功，全网受管节点已广播同步`)
    }
    modal.show = false
    await loadMembers()
  } catch (err: any) {
    toast.error((modal.isEdit ? '更新失败: ' : '添加失败: ') + (err.response?.data?.error || err.message))
  }
}

async function resetToken(id: number) {
  const confirmed = await showConfirm({
    title: '重置订阅码',
    message: '确定要重置该成员的订阅码吗？重置后原有的订阅链接将立即失效！',
    confirmText: '确认重置',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.post(`/members/${id}/reset-token`)
    toast.success('订阅码已成功重置！原链接已失效。')
    await loadMembers()
  } catch (err: any) {
    toast.error('重置失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteMember(id: number) {
  const confirmed = await showConfirm({
    title: '删除成员',
    message: '确定要删除该成员吗？删除后该成员名下的所有客户端连接配置将无法再同步节点。',
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.delete(`/members/${id}`)
    toast.success('成员已删除！')
    await loadMembers()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

// Cloudflare 优选 IP 状态与数据
const cfPrefStatus = reactive({
  sub_url: '',
  host: '',
  uuid: '',
  path: '',
  cron_enabled: false,
  cron_mode: 'interval',
  cron_interval: '6h',
  cron_time: '04:00',
  last_fetch_at: '',
  last_fetch_status: '',
  last_fetch_count: '',
  total_count: 0,
})

const cfIps = ref<any[]>([])
const loadingCfIps = ref(false)
const fetchingCf = ref(false)
const savingCfSettings = ref(false)
const cfSearchKeyword = ref('')

function formatDateTime(str?: string): string {
  if (!str) return ''
  const d = new Date(str)
  if (isNaN(d.getTime())) return str
  return d.toLocaleString('zh-CN', { hour12: false })
}

async function loadCFPrefStatus() {
  try {
    const res = await api.get('/cf-preferred/status')
    if (res.data) {
      Object.assign(cfPrefStatus, res.data)
    }
  } catch (err: any) {
    console.error('加载优选 IP 配置失败:', err)
  }
}

async function loadCFIps() {
  loadingCfIps.value = true
  try {
    const params = cfSearchKeyword.value.trim() ? { keyword: cfSearchKeyword.value.trim() } : {}
    const res = await api.get('/cf-preferred/ips', { params })
    cfIps.value = res.data || []
    cfPrefStatus.total_count = cfIps.value.length
  } catch (err: any) {
    toast.error('加载优选 IP 列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loadingCfIps.value = false
  }
}

async function saveCFPrefSettings() {
  savingCfSettings.value = true
  try {
    await api.post('/cf-preferred/settings', {
      sub_url: cfPrefStatus.sub_url,
      cron_enabled: cfPrefStatus.cron_enabled,
      cron_mode: cfPrefStatus.cron_mode,
      cron_interval: cfPrefStatus.cron_interval,
      cron_time: cfPrefStatus.cron_time,
    })
    toast.success('优选 IP 抓取与调度配置保存成功！')
    await loadCFPrefStatus()
  } catch (err: any) {
    toast.error('保存配置失败: ' + (err.response?.data?.error || err.message))
  } finally {
    savingCfSettings.value = false
  }
}

async function manualFetchCFIps() {
  fetchingCf.value = true
  try {
    const res = await api.post('/cf-preferred/fetch')
    const count = res.data?.data?.count || 0
    toast.success(`优选 IP 同步成功！已成功持久化 ${count} 个节点`)
    await loadCFPrefStatus()
    await loadCFIps()
  } catch (err: any) {
    toast.error('同步优选 IP 失败: ' + (err.response?.data?.error || err.message))
    await loadCFPrefStatus()
  } finally {
    fetchingCf.value = false
  }
}

async function clearAllCFIps() {
  const ok = await showConfirm({
    title: '确认清空本地优选 IP 库',
    message: '清空后将移除本地数据库保存的所有 Cloudflare 优选节点，是否继续？',
    confirmText: '确认清空',
    danger: true,
  })
  if (!ok) return

  try {
    await api.delete('/cf-preferred/ips')
    toast.success('本地优选 IP 库已成功清空')
    cfIps.value = []
    cfPrefStatus.total_count = 0
    await loadCFPrefStatus()
  } catch (err: any) {
    toast.error('清空失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteSingleCFIp(id: number) {
  try {
    await api.delete(`/cf-preferred/ips/${id}`)
    toast.success('已删除该优选 IP')
    cfIps.value = cfIps.value.filter((item) => item.id !== id)
    cfPrefStatus.total_count = cfIps.value.length
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

function copyAllCFIpList() {
  if (!cfIps.value.length) return
  const text = cfIps.value.map((item) => `${item.ip}:${item.port}`).join('\n')
  copyToClipboard(text, `${cfIps.value.length} 个优选 IP:端口`)
}

function copyAllCFVlessLinks() {
  if (!cfIps.value.length) return
  const text = cfIps.value.map((item) => item.raw_link).filter(Boolean).join('\n')
  copyToClipboard(text, `${cfIps.value.length} 条 VLESS 节点链接`)
}

function copySingleCFIp(ip: string, port: number) {
  copyToClipboard(`${ip}:${port}`, `${ip}:${port}`)
}

function copySingleCFLink(link: string) {
  copyToClipboard(link, 'VLESS 优选节点链接')
}

// ================= 节点清单 (按照节点分组) =================
interface NodeInventoryItem {
  inbound_id: number
  tag: string
  protocol: string
  protocol_label: string
  port: number
  listen: string
  enabled: boolean
  server_host: string
  server_name?: string
  path?: string
  tunnel_domain?: string
  flow?: string
  fingerprint?: string
  link: string
  clash_snippet: string
}

interface NodeInventoryGroup {
  node_id: number
  node_name: string
  sub_prefix?: string
  host: string
  is_master: boolean
  status: string
  region: string
  cloud_provider: string
  latency_ms: number
  inbounds: NodeInventoryItem[]
  inbound_count: number
  error?: string
}

interface NodeInventoryData {
  member: any
  groups: NodeInventoryGroup[]
  total_nodes: number
  total_inbounds: number
  preferred_ip_count: number
}

const inventoryData = ref<NodeInventoryData | null>(null)
const inventoryMemberId = ref<number | ''>('')
const loadingInventory = ref(false)
const inventorySearchKeyword = ref('')
const collapsedGroups = ref<Record<number, boolean>>({})

async function loadInventory() {
  loadingInventory.value = true
  try {
    const params: Record<string, any> = {}
    if (inventoryMemberId.value) {
      params.member_id = inventoryMemberId.value
    }
    const res = await api.get('/subscription/inventory', { params })
    inventoryData.value = res.data
    if (!inventoryMemberId.value && res.data.member?.id) {
      inventoryMemberId.value = res.data.member.id
    }
  } catch (err: any) {
    toast.error('获取节点清单失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loadingInventory.value = false
  }
}

const filteredGroups = computed(() => {
  if (!inventoryData.value?.groups) return []
  const kw = inventorySearchKeyword.value.trim().toLowerCase()
  if (!kw) return inventoryData.value.groups

  return inventoryData.value.groups
    .map((group) => {
      const groupMatches =
        group.node_name.toLowerCase().includes(kw) ||
        group.host.toLowerCase().includes(kw) ||
        (group.region && group.region.toLowerCase().includes(kw))

      if (groupMatches) return group

      const matchedInbounds = (group.inbounds || []).filter(
        (item) =>
          item.tag.toLowerCase().includes(kw) ||
          item.protocol.toLowerCase().includes(kw) ||
          (item.protocol_label && item.protocol_label.toLowerCase().includes(kw)) ||
          String(item.port).includes(kw) ||
          (item.server_name && item.server_name.toLowerCase().includes(kw)) ||
          (item.path && item.path.toLowerCase().includes(kw)) ||
          (item.tunnel_domain && item.tunnel_domain.toLowerCase().includes(kw))
      )
      return {
        ...group,
        inbounds: matchedInbounds,
      }
    })
    .filter(
      (group) =>
        group.inbounds.length > 0 ||
        group.node_name.toLowerCase().includes(kw) ||
        group.host.toLowerCase().includes(kw)
    )
})

function toggleGroupCollapse(nodeId: number) {
  collapsedGroups.value[nodeId] = !collapsedGroups.value[nodeId]
}

function copyAllInventoryLinks() {
  if (!inventoryData.value?.groups) return
  const links: string[] = []
  for (const group of inventoryData.value.groups) {
    for (const item of group.inbounds || []) {
      if (item.enabled && item.link) {
        links.push(item.link)
      }
    }
  }
  if (links.length === 0) {
    toast.warning('当前无可用节点链接')
    return
  }
  copyToClipboard(links.join('\n'), `全集群 ${links.length} 个节点链接`)
}

function copyGroupLinks(group: NodeInventoryGroup) {
  const links = (group.inbounds || [])
    .filter((item) => item.enabled && item.link)
    .map((item) => item.link)
  if (links.length === 0) {
    toast.warning(`节点 [${group.node_name}] 无可用连接`)
    return
  }
  copyToClipboard(links.join('\n'), `[${group.node_name}] ${links.length} 个节点链接`)
}

function copyInboundLink(link: string, tag: string) {
  if (!link) {
    toast.warning('该节点暂无有效连接链接')
    return
  }
  copyToClipboard(link, `节点 ${tag} 链接`)
}

function copyInboundClashSnippet(snippet: string, tag: string) {
  if (!snippet) {
    toast.warning('该节点暂无 Clash 配置片段')
    return
  }
  copyToClipboard(snippet, `节点 ${tag} Clash 片段`)
}

function openInboundQR(link: string, tag: string) {
  if (!link) {
    toast.warning('该节点暂无有效连接链接')
    return
  }
  qrModal.title = `节点连接二维码 - ${tag}`
  qrModal.url = link
  qrModal.show = true
}

watch(
  () => route.query,
  (query) => {
    if (query.sub === 'subconverter' || query.tab === 'subconverter') {
      activeSubSection.value = 'subconverter'
    } else if (query.sub === 'cf_preferred' || query.tab === 'cf_preferred' || query.sub === 'cfip') {
      activeSubSection.value = 'cf_preferred'
      loadCFIps()
    } else if (query.sub === 'inventory' || query.tab === 'inventory' || query.sub === 'nodelist') {
      activeSubSection.value = 'inventory'
      loadInventory()
    }
  },
  { immediate: true }
)

watch(activeSubSection, (sec) => {
  if (sec === 'cf_preferred') {
    loadCFPrefStatus()
    loadCFIps()
  } else if (sec === 'inventory') {
    loadInventory()
  }
})

onMounted(async () => {
  await Promise.all([
    loadMembers(),
    fetchSubStatus(),
    fetchSubConfig(),
    fetchTemplates(),
    fetchWorkbenchPreferences(),
    loadCFPrefStatus(),
  ])
  if (activeSubSection.value === 'cf_preferred') {
    await loadCFIps()
  } else if (activeSubSection.value === 'inventory') {
    await loadInventory()
  }
  window.addEventListener('node-context-changed', loadMembers)
})

onUnmounted(() => {
  window.removeEventListener('node-context-changed', loadMembers)
})
</script>

<style scoped>
.form-label {
  display: block;
  font-size: 0.85rem;
  color: #cbd5e1;
  margin-bottom: 0.35rem;
  font-weight: 500;
}

.form-hint {
  color: #64748b;
  font-size: 0.725rem;
  margin-top: 0.25rem;
}

.code-editor {
  width: 100%;
  background-color: #0b0f19;
  border: 1px solid #1e293b;
  border-radius: 0.5rem;
  padding: 0.85rem;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.8rem;
  line-height: 1.45;
  color: #e2e8f0;
  resize: vertical;
}

.code-preview {
  background-color: #0b0f19;
  border: 1px solid #1e293b;
  border-radius: 0.5rem;
  padding: 0.85rem;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.78rem;
  line-height: 1.45;
  color: #cbd5e1;
  overflow: auto;
}
</style>
