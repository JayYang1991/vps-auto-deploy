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

      <div style="display: flex; justify-content: flex-end; margin-bottom: 1rem;">
        <button @click="modal.show = true" class="btn btn-primary">+ 添加成员</button>
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
        <button @click="modal.show = true" class="btn btn-primary">+ 添加首个成员</button>
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
              <label class="form-label">规则集与分流模板 (Ruleset / Config)</label>
              <select v-model="tester.template" class="input">
                <option v-for="t in templates" :key="t.name" :value="t.config_value">
                  {{ t.label }} {{ t.recommended ? '★ 推荐' : '' }}
                </option>
                <option value="custom">✏️ 自定义外部规则集 URL...</option>
              </select>
            </div>
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

          <!-- 在线转换预览输出结果 -->
          <div v-if="previewResult" style="margin-top: 1rem;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
              <span style="font-size: 0.8rem; color: #34d399; font-weight: 600;">
                ✓ 转换成功 (HTTP 200 &middot; {{ previewResult.length }} 字节)
              </span>
              <button @click="copyPreviewOutput" class="btn btn-secondary" style="font-size: 0.725rem; padding: 0.15rem 0.5rem;">
                复制内容
              </button>
            </div>
            <pre class="code-preview" style="max-height: 260px;">{{ previewResult }}</pre>
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

    <!-- 添加成员弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem;">添加团队成员</h3>

        <form @submit.prevent="createMember" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">成员名称 / 设备备注</label>
            <input v-model="form.name" class="input" required placeholder="如 Jason-iPhone 或 团队成员A" />
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">添加并分配专属订阅</button>
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
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import QrcodeVue from 'qrcode.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

const route = useRoute()
const { toast } = useToast()
const { showConfirm } = useConfirm()

const activeSubSection = ref<'members' | 'subconverter'>('members')
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
const previewResult = ref('')
const testingPreview = ref(false)

const modal = reactive({
  show: false,
})

const form = reactive({
  name: '',
})

const qrModal = reactive({
  show: false,
  title: '',
  url: '',
})

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
  copyToClipboard(previewResult.value, '转换结果配置')
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

async function createMember() {
  try {
    await api.post('/members', {
      name: form.name,
      enabled: true,
    })
    modal.show = false
    toast.success(`成员 ${form.name} 创建成功！`)
    form.name = ''
    await loadMembers()
  } catch (err: any) {
    toast.error('添加失败: ' + (err.response?.data?.error || err.message))
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

watch(
  () => route.query,
  (query) => {
    if (query.sub === 'subconverter' || query.tab === 'subconverter') {
      activeSubSection.value = 'subconverter'
    }
  },
  { immediate: true }
)

onMounted(async () => {
  await Promise.all([loadMembers(), fetchSubStatus(), fetchSubConfig(), fetchTemplates(), fetchWorkbenchPreferences()])
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
