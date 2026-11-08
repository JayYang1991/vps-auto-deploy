<template>
  <div>
    <!-- 头部操作与说明 -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">📤</span>
          <h2 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">Sing-box 出站规则配置 (Outbounds)</h2>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          配置服务端网络出口流向，支持本机直连 (Direct)、广告拦截 (Block)、SOCKS5/WARP 中继或链式多跳转发。
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem;">
        <button @click="openCreateModal" class="btn btn-primary">+ 新增出站规则</button>
      </div>
    </div>

    <!-- 未安装 sing-box 渐进式空状态引导卡片 (仅在加载完成后判定) -->
    <div v-if="!loading && !singboxInstalled" class="card" style="margin-bottom: 1.5rem; background: rgba(245, 158, 11, 0.08); border-color: rgba(245, 158, 11, 0.35);">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div style="display: flex; align-items: center; gap: 0.75rem;">
          <span style="font-size: 1.6rem;">📦</span>
          <div>
            <div style="font-weight: 600; color: #fbbf24; font-size: 0.95rem;">未检测到已安装的 sing-box 代理核心</div>
            <div style="color: #94a3b8; font-size: 0.825rem; margin-top: 0.25rem;">
              出站规则由底层的 sing-box 核心驱动。您可先预先配置规则，或前往软件中心一键安装。
            </div>
          </div>
        </div>
        <router-link to="/components?install=sing-box" class="btn btn-primary" style="font-size: 0.825rem; padding: 0.4rem 0.85rem;">
          🚀 前往「软件中心」一键安装
        </router-link>
      </div>
    </div>

    <!-- 加载中指示器 -->
    <div v-if="loading" class="card loading-box">
      <div class="loading-spinner"></div>
      <p class="loading-text">正在加载出站规则配置...</p>
    </div>

    <!-- 出站规则列表空状态 -->
    <div v-else-if="outbounds.length === 0" class="card" style="text-align: center; padding: 3rem;">
      <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">📤</div>
      <p style="color: #94a3b8; margin-bottom: 1.25rem;">暂无已配置的出站规则，点击下方按钮添加出站规则。</p>
      <button @click="openCreateModal" class="btn btn-primary">+ 添加出站规则</button>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div
        v-for="ob in outbounds"
        :key="ob.id"
        class="card"
        style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;"
      >
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">{{ ob.tag }}</span>
            <span :class="['badge', getTypeBadge(ob.type)]">
              {{ ob.type }}
            </span>
            <span v-for="feat in getOutboundFeatures(ob)" :key="feat" class="badge" style="font-size: 0.68rem; background: rgba(56, 189, 248, 0.12); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.25);">
              {{ feat }}
            </span>
            <span v-if="ob.is_system" class="badge badge-secondary" style="font-size: 0.68rem;">
              系统预置
            </span>
            <span :class="['badge', ob.enabled ? 'badge-success' : 'badge-secondary']">
              {{ ob.enabled ? '已启用' : '已停用' }}
            </span>
            <span style="color: #64748b; font-size: 0.75rem;">
              优先级: {{ ob.priority }}
            </span>
          </div>

          <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; display: flex; gap: 1.5rem; flex-wrap: wrap;">
            <span v-if="ob.server">
              上游地址: <b style="color: #38bdf8;">{{ ob.server }}:{{ ob.server_port }}</b>
            </span>
            <span v-else-if="ob.type === 'direct'">
              出口目标: <b style="color: #34d399;">本机公网网卡直接出站</b>
            </span>
            <span v-else-if="ob.type === 'block'">
              出口动作: <b style="color: #f87171;">静默丢弃 / 拦截</b>
            </span>
            <span v-else>
              出口参数: <b style="color: #f1f5f9;">自定义配置</b>
            </span>
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 0.6rem;">
          <!-- 快捷启停开关 -->
          <button
            @click="toggleOutbound(ob.id)"
            class="btn btn-secondary"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
            :title="ob.enabled ? '点击停用该出站' : '点击启用该出站'"
          >
            {{ ob.enabled ? '⏸ 停用' : '▶ 启用' }}
          </button>

          <!-- 编辑按钮 -->
          <button
            @click="openEditModal(ob)"
            class="btn btn-secondary"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
          >
            ✏️ 编辑
          </button>

          <!-- 删除按钮 (系统预置 direct/block 禁止删除) -->
          <button
            v-if="!ob.is_system"
            @click="deleteOutbound(ob)"
            class="btn btn-danger"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
          >
            🗑️ 删除
          </button>
        </div>
      </div>
    </div>

    <!-- 新建/编辑出站规则弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content" style="max-width: 680px; max-height: 90vh; overflow-y: auto;">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem; color: #f8fafc;">
          {{ modal.isEdit ? '编辑出站规则' : '新建出站规则' }}
        </h3>

        <!-- 模式切换：仅在新建出站时提供一键中转选项 -->
        <div v-if="!modal.isEdit" style="display: flex; gap: 0.5rem; margin-bottom: 1.25rem; background: #0f172a; padding: 0.25rem; border-radius: 0.5rem;">
          <button
            type="button"
            :class="['btn', createMode === 'relay' ? 'btn-primary' : 'btn-secondary']"
            style="flex: 1; font-size: 0.825rem; padding: 0.45rem; justify-content: center;"
            @click="switchCreateMode('relay')"
          >
            ⚡ 从集群节点快速创建中转 (推荐)
          </button>
          <button
            type="button"
            :class="['btn', createMode === 'manual' ? 'btn-primary' : 'btn-secondary']"
            style="flex: 1; font-size: 0.825rem; padding: 0.45rem; justify-content: center;"
            @click="switchCreateMode('manual')"
          >
            🛠️ 手动输入模式
          </button>
        </div>

        <!-- 节点中转级联选择器工作区 -->
        <div v-if="!modal.isEdit && createMode === 'relay'" style="background: rgba(30, 41, 59, 0.7); border: 1px solid #334155; border-radius: 0.5rem; padding: 1rem; margin-bottom: 1rem; display: flex; flex-direction: column; gap: 0.85rem;">
          <div style="font-size: 0.85rem; font-weight: 600; color: #38bdf8; display: flex; align-items: center; gap: 0.4rem;">
            <span>🌐</span> 目标落地节点与入站协议选择 (自动排除本节点防环路)
          </div>

          <div v-if="loadingRelayCandidates" style="font-size: 0.8rem; color: #94a3b8; padding: 0.5rem 0;">
            ⏳ 正在探测全集群可用节点入站...
          </div>
          <div v-else-if="relayCandidates.length === 0" style="font-size: 0.8rem; color: #f59e0b; background: rgba(245, 158, 11, 0.1); padding: 0.75rem; border-radius: 0.35rem;">
            ⚠️ 当前集群中没有其他在线节点或未配置启用中的入站协议，请先在其他节点添加入站或切换至手动输入模式。
          </div>
          <div v-else style="display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">1. 目标落地节点</label>
                <select v-model="selectedRelayNodeId" class="input" @change="onRelayNodeChange">
                  <option v-for="node in relayCandidates" :key="node.node_id" :value="node.node_id">
                    {{ node.node_name }} ({{ node.host }})
                  </option>
                </select>
              </div>

              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">2. 目标入站协议</label>
                <select v-model="selectedRelayInboundId" class="input" @change="fetchAndApplyRelayOutbound">
                  <option v-for="ib in currentRelayInbounds" :key="ib.id" :value="ib.id">
                    {{ ib.summary }}
                  </option>
                </select>
              </div>
            </div>

            <!-- 地址偏好：若关联了 Cloudflare 穿透隧道，允许二选一 -->
            <div v-if="selectedInboundHasTunnel" style="font-size: 0.8rem; color: #cbd5e1; display: flex; gap: 1.25rem; align-items: center; margin-top: 0.25rem;">
              <span style="color: #94a3b8;">连接偏好:</span>
              <label style="display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                <input type="radio" v-model="selectedRelayAddrType" value="ip" @change="fetchAndApplyRelayOutbound" />
                公网直连 IP ({{ currentRelayNode?.host }})
              </label>
              <label style="display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                <input type="radio" v-model="selectedRelayAddrType" value="tunnel" @change="fetchAndApplyRelayOutbound" />
                🚇 Cloudflare 穿透域名 ({{ selectedInbound?.tunnel_domain }})
              </label>
            </div>

            <div v-if="relayGeneratedSummary" style="background: rgba(16, 185, 129, 0.12); border: 1px solid rgba(16, 185, 129, 0.35); border-radius: 0.35rem; padding: 0.6rem 0.85rem; font-size: 0.8rem; color: #34d399;">
              ✅ {{ relayGeneratedSummary }} (已自动计算并注入加密密钥，可在下方查看参数或微调后保存)
            </div>
          </div>
        </div>

        <form @submit.prevent="submitOutbound" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
              出站标识 Tag (必须唯一) <span style="color:#ef4444;">*</span>
            </label>
            <input
              v-model="form.tag"
              class="input"
              required
              :disabled="modal.isEdit && modal.isSystem"
              placeholder="如 direct, warp-out, vless-jp, hy2-relay"
            />
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">出站协议类型</label>
              <select
                v-model="form.type"
                class="input"
                :disabled="modal.isEdit && modal.isSystem"
                @change="onTypeChange"
              >
                <option value="direct">Direct (本机直连出站)</option>
                <option value="block">Block (拦截阻断)</option>
                <option value="vless">VLESS (上游中继 / Reality / WS)</option>
                <option value="trojan">Trojan (上游 TLS 代理)</option>
                <option value="hysteria2">Hysteria 2 (上游 UDP 极速节点)</option>
                <option value="vmess">VMess (上游 VMess 节点)</option>
                <option value="tuic">TUIC (上游 TUIC v5 极速节点)</option>
                <option value="shadowsocks">Shadowsocks (节点转发)</option>
                <option value="socks">SOCKS5 (上游前置代理 / WARP)</option>
                <option value="http">HTTP Proxy (上游 HTTP/HTTPS 代理)</option>
                <option value="custom">Custom (高级自定义 JSON 片段)</option>
              </select>
            </div>

            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
                优先级权重 (Priority)
              </label>
              <input
                v-model.number="form.priority"
                type="number"
                class="input"
                min="1"
                max="999"
                required
                placeholder="1 (越小越优先)"
              />
              <span style="font-size: 0.72rem; color: #64748b; margin-top: 0.2rem; display: block;">
                排序在最前方的规则将作为默认缺省出站
              </span>
            </div>
          </div>

          <!-- SOCKS5 / HTTP 参数 -->
          <div
            v-if="form.type === 'socks' || form.type === 'http'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游服务器地址 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="127.0.0.1 或 域名" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" :placeholder="form.type === 'socks' ? '1080' : '8080'" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证用户名 (可选)</label>
                <input v-model="proxySpec.username" class="input" placeholder="留空则无认证" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证密码 (可选)</label>
                <input v-model="proxySpec.password" type="password" class="input" placeholder="留空则无密码" />
              </div>
            </div>

            <!-- HTTP 专属 HTTPS/TLS 支持 -->
            <div v-if="form.type === 'http'" style="background: rgba(30, 41, 59, 0.5); padding: 0.75rem; border-radius: 0.375rem; border: 1px solid #334155; display: flex; flex-direction: column; gap: 0.6rem;">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <label style="font-size: 0.75rem; color: #cbd5e1; display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                  <input type="checkbox" v-model="proxySpec.tls_enabled" />
                  <span>🛡️ 上游代理使用 HTTPS / TLS 加密</span>
                </label>
                <label v-if="proxySpec.tls_enabled" style="font-size: 0.72rem; color: #94a3b8; display: flex; align-items: center; gap: 0.3rem; cursor: pointer;">
                  <input type="checkbox" v-model="proxySpec.tls_insecure" />
                  <span>跳过证书校验 (insecure)</span>
                </label>
              </div>
              <div v-if="proxySpec.tls_enabled">
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS SNI 伪装域名</label>
                <input v-model="proxySpec.tls_server_name" class="input" placeholder="留空默认同服务器地址" />
              </div>
            </div>
          </div>

          <!-- Shadowsocks 参数 -->
          <div
            v-else-if="form.type === 'shadowsocks'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 SS 服务器 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="ss.example.com" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="8388" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">加密方法 (Method)</label>
                <select v-model="ssSpec.method" class="input">
                  <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
                  <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm</option>
                  <option value="aes-128-gcm">aes-128-gcm</option>
                  <option value="aes-256-gcm">aes-256-gcm</option>
                  <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
                  <option value="none">none (无加密)</option>
                </select>
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">密码 (Password) <span style="color:#ef4444;">*</span></label>
                <input v-model="ssSpec.password" class="input" required placeholder="SS 密码" />
              </div>
            </div>

            <!-- SS 插件扩展 -->
            <div style="display: grid; grid-template-columns: 1fr 2fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">插件 (Plugin，可选)</label>
                <input v-model="ssSpec.plugin" class="input" placeholder="如 v2ray-plugin" />
              </div>
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">插件参数 (Plugin Opts)</label>
                <input v-model="ssSpec.plugin_opts" class="input" placeholder="如 server;tls;host=domain.com" />
              </div>
            </div>
          </div>

          <!-- VLESS 出站参数 -->
          <div
            v-else-if="form.type === 'vless'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 VLESS 服务器 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="vless.example.com 或 IP" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="443" required />
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">用户 UUID <span style="color:#ef4444;">*</span></label>
              <input v-model="vlessSpec.uuid" class="input" required placeholder="用户 UUID (例如 c3a6027a-...)" />
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">流控特性 (Flow)</label>
                <select v-model="vlessSpec.flow" class="input">
                  <option value="xtls-rprx-vision">xtls-rprx-vision (Reality / TCP 强推)</option>
                  <option value="">无 (None / WS / gRPC 适用)</option>
                </select>
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">数据包编码 (Packet Encoding)</label>
                <select v-model="vlessSpec.packet_encoding" class="input">
                  <option value="xudp">xudp (标准高效 UDP 包装)</option>
                  <option value="">无 (None)</option>
                </select>
              </div>
            </div>

            <!-- 传输层协议 (Transport) -->
            <div style="background: rgba(30, 41, 59, 0.5); padding: 0.75rem; border-radius: 0.375rem; border: 1px solid #334155;">
              <label style="display: block; font-size: 0.75rem; font-weight: 600; color: #cbd5e1; margin-bottom: 0.5rem;">
                🌐 传输层协议 (Transport)
              </label>
              <div style="display: grid; grid-template-columns: 1fr 2fr; gap: 0.75rem;">
                <div>
                  <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">协议类型</label>
                  <select v-model="vlessSpec.transport_type" class="input">
                    <option value="tcp">TCP (标准直连)</option>
                    <option value="ws">WebSocket (CDN 反代中转)</option>
                    <option value="grpc">gRPC</option>
                  </select>
                </div>
                <div v-if="vlessSpec.transport_type === 'ws'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem;">
                  <div>
                    <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">WS 路径 (Path)</label>
                    <input v-model="vlessSpec.ws_path" class="input" placeholder="/ws" />
                  </div>
                  <div>
                    <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">Host 伪装标头</label>
                    <input v-model="vlessSpec.ws_host" class="input" placeholder="留空同服务器地址" />
                  </div>
                </div>
                <div v-else-if="vlessSpec.transport_type === 'grpc'">
                  <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">gRPC Service Name</label>
                  <input v-model="vlessSpec.grpc_service_name" class="input" placeholder="如 vless-grpc" />
                </div>
              </div>
            </div>

            <!-- 传输层安全 (TLS / Reality) -->
            <div style="background: rgba(30, 41, 59, 0.5); padding: 0.75rem; border-radius: 0.375rem; border: 1px solid #334155; display: flex; flex-direction: column; gap: 0.6rem;">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <label style="font-size: 0.75rem; font-weight: 600; color: #cbd5e1; display: flex; align-items: center; gap: 0.4rem; cursor: pointer;">
                  <input type="checkbox" v-model="vlessSpec.tls_enabled" />
                  <span>🛡️ 启用 TLS 传输层加密</span>
                </label>
                <label v-if="vlessSpec.tls_enabled" style="font-size: 0.72rem; color: #94a3b8; display: flex; align-items: center; gap: 0.3rem; cursor: pointer;">
                  <input type="checkbox" v-model="vlessSpec.tls_insecure" />
                  <span>跳过证书校验 (insecure)</span>
                </label>
              </div>

              <div v-if="vlessSpec.tls_enabled" style="display: flex; flex-direction: column; gap: 0.6rem; margin-top: 0.25rem;">
                <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
                  <div>
                    <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS 伪装域名 (SNI)</label>
                    <input v-model="vlessSpec.tls_server_name" class="input" placeholder="留空默认同服务器地址" />
                  </div>
                  <div>
                    <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">客户端指纹 (uTLS)</label>
                    <select v-model="vlessSpec.tls_utls_fingerprint" class="input">
                      <option value="chrome">chrome (推荐)</option>
                      <option value="firefox">firefox</option>
                      <option value="safari">safari</option>
                      <option value="ios">ios</option>
                      <option value="edge">edge</option>
                      <option value="random">random</option>
                    </select>
                  </div>
                </div>

                <!-- Reality 伪装区块 -->
                <div style="background: rgba(15, 23, 42, 0.6); padding: 0.6rem 0.75rem; border-radius: 0.375rem; border: 1px dashed #475569;">
                  <label style="font-size: 0.75rem; font-weight: 600; color: #38bdf8; display: flex; align-items: center; gap: 0.4rem; cursor: pointer;">
                    <input type="checkbox" v-model="vlessSpec.reality_enabled" />
                    <span>✨ 开启 Reality 真实伪装 (VLESS-Reality)</span>
                  </label>
                  
                  <div v-if="vlessSpec.reality_enabled" style="display: flex; flex-direction: column; gap: 0.5rem; margin-top: 0.5rem;">
                    <div>
                      <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">Reality Public Key (服务端公钥) <span style="color:#ef4444;">*</span></label>
                      <input v-model="vlessSpec.reality_public_key" class="input mono" placeholder="43 字符公钥 (如 -uM4_z...)" required />
                    </div>
                    <div>
                      <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">Short ID (简短 ID，可选)</label>
                      <input v-model="vlessSpec.reality_short_id" class="input mono" placeholder="如 1688，留空使用空 short_id" />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Trojan 出站参数 -->
          <div
            v-else-if="form.type === 'trojan'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 Trojan 服务器 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="trojan.example.com 或 IP" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="443" required />
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证密码 <span style="color:#ef4444;">*</span></label>
              <input v-model="trojanSpec.password" class="input" required placeholder="节点认证密码" />
            </div>

            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS SNI 伪装域名</label>
                <input v-model="trojanSpec.server_name" class="input" placeholder="留空默认同服务器地址" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">ALPN 协商</label>
                <input v-model="trojanSpec.alpn" class="input" placeholder="h2,http/1.1" />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 2fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">传输层协议</label>
                <select v-model="trojanSpec.transport_type" class="input">
                  <option value="tcp">TCP (标准)</option>
                  <option value="ws">WebSocket (CDN 反代)</option>
                </select>
              </div>
              <div v-if="trojanSpec.transport_type === 'ws'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem;">
                <div>
                  <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">WS 路径</label>
                  <input v-model="trojanSpec.ws_path" class="input" placeholder="/ws" />
                </div>
                <div>
                  <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">Host 标头</label>
                  <input v-model="trojanSpec.ws_host" class="input" placeholder="留空同服务器地址" />
                </div>
              </div>
            </div>

            <label style="font-size: 0.75rem; color: #94a3b8; display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
              <input type="checkbox" v-model="trojanSpec.insecure" />
              <span>跳过证书校验 (insecure，允许自签证书)</span>
            </label>
          </div>

          <!-- Hysteria 2 出站参数 -->
          <div
            v-else-if="form.type === 'hysteria2'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 Hysteria 2 服务器 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="hy2.example.com 或 IP" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="443" required />
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证密码 <span style="color:#ef4444;">*</span></label>
              <input v-model="hysteria2Spec.password" class="input" required placeholder="节点认证密码" />
            </div>

            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS SNI 域名</label>
                <input v-model="hysteria2Spec.server_name" class="input" placeholder="留空默认同服务器地址" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">ALPN 协商</label>
                <input v-model="hysteria2Spec.alpn" class="input" placeholder="h3" />
              </div>
            </div>

            <!-- Salamander 混淆 -->
            <div style="display: grid; grid-template-columns: 1fr 2fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">混淆类型 (Obfs)</label>
                <select v-model="hysteria2Spec.obfs_type" class="input">
                  <option value="none">无混淆 (None)</option>
                  <option value="salamander">Salamander</option>
                </select>
              </div>
              <div v-if="hysteria2Spec.obfs_type === 'salamander'">
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">混淆密码 (Password)</label>
                <input v-model="hysteria2Spec.obfs_password" class="input" placeholder="混淆密钥" />
              </div>
            </div>

            <!-- 带宽设置 (可选) -->
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">上行带宽 Mbps (可选)</label>
                <input v-model.number="hysteria2Spec.up_mbps" type="number" class="input" placeholder="例如：100" />
              </div>
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">下行带宽 Mbps (可选)</label>
                <input v-model.number="hysteria2Spec.down_mbps" type="number" class="input" placeholder="例如：500" />
              </div>
            </div>

            <label style="font-size: 0.75rem; color: #94a3b8; display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
              <input type="checkbox" v-model="hysteria2Spec.insecure" />
              <span>跳过证书校验 (insecure，支持自签证书)</span>
            </label>
          </div>

          <!-- VMess 出站参数 -->
          <div
            v-else-if="form.type === 'vmess'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 VMess 服务器 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="vmess.example.com" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="443" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">用户 UUID <span style="color:#ef4444;">*</span></label>
                <input v-model="vmessSpec.uuid" class="input" required placeholder="用户 UUID" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">加密方式</label>
                <select v-model="vmessSpec.security" class="input">
                  <option value="auto">auto (自动)</option>
                  <option value="aes-128-gcm">aes-128-gcm</option>
                  <option value="chacha20-poly1305">chacha20-poly1305</option>
                  <option value="zero">zero (无加密)</option>
                </select>
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 2fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">传输层协议</label>
                <select v-model="vmessSpec.transport_type" class="input">
                  <option value="tcp">TCP (标准)</option>
                  <option value="ws">WebSocket</option>
                </select>
              </div>
              <div v-if="vmessSpec.transport_type === 'ws'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem;">
                <div>
                  <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">WS 路径</label>
                  <input v-model="vmessSpec.ws_path" class="input" placeholder="/ws" />
                </div>
                <div>
                  <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">Host 标头</label>
                  <input v-model="vmessSpec.ws_host" class="input" placeholder="留空同服务器地址" />
                </div>
              </div>
            </div>

            <div style="display: flex; align-items: center; gap: 1rem;">
              <label style="font-size: 0.75rem; color: #cbd5e1; display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                <input type="checkbox" v-model="vmessSpec.tls_enabled" />
                <span>开启 TLS 加密</span>
              </label>
              <label v-if="vmessSpec.tls_enabled" style="font-size: 0.75rem; color: #94a3b8; display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                <input type="checkbox" v-model="vmessSpec.tls_insecure" />
                <span>跳过证书校验 (insecure)</span>
              </label>
            </div>
            <div v-if="vmessSpec.tls_enabled">
              <label style="display: block; font-size: 0.72rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS SNI 域名</label>
              <input v-model="vmessSpec.tls_server_name" class="input" placeholder="留空默认同服务器地址" />
            </div>
          </div>

          <!-- TUIC 出站参数 -->
          <div
            v-else-if="form.type === 'tuic'"
            style="background-color: #0f172a; padding: 1.15rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.85rem; border: 1px solid #1e293b;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 TUIC 服务器 <span style="color:#ef4444;">*</span></label>
                <input v-model="form.server" class="input" placeholder="tuic.example.com 或 IP" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口 <span style="color:#ef4444;">*</span></label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="8443" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">用户 UUID <span style="color:#ef4444;">*</span></label>
                <input v-model="tuicSpec.uuid" class="input" required placeholder="用户 UUID" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证密码 <span style="color:#ef4444;">*</span></label>
                <input v-model="tuicSpec.password" class="input" required placeholder="节点密码" />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">拥塞控制算法</label>
                <select v-model="tuicSpec.congestion_control" class="input">
                  <option value="bbr">bbr (推荐)</option>
                  <option value="cubic">cubic</option>
                  <option value="new_reno">new_reno</option>
                </select>
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS SNI 域名</label>
                <input v-model="tuicSpec.server_name" class="input" placeholder="留空同服务器地址" />
              </div>
            </div>

            <label style="font-size: 0.75rem; color: #94a3b8; display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
              <input type="checkbox" v-model="tuicSpec.insecure" />
              <span>跳过证书校验 (insecure，允许自签证书)</span>
            </label>
          </div>

          <!-- Custom 自定义 JSON 参数 -->
          <div
            v-else-if="form.type === 'custom'"
            style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.5rem;"
          >
            <label style="display: block; font-size: 0.75rem; color: #94a3b8;">自定义出站 JSON 对象 (例如 WireGuard/WARP 配置)</label>
            <textarea
              v-model="customSpecJSON"
              class="input"
              rows="6"
              style="font-family: monospace; font-size: 0.75rem;"
              placeholder='{\n  "type": "wireguard",\n  "server": "162.159.192.1",\n  "server_port": 2408\n}'
            ></textarea>
          </div>

          <div style="display: flex; align-items: center; gap: 0.5rem; margin-top: 0.5rem;">
            <input type="checkbox" id="ob-enabled" v-model="form.enabled" />
            <label for="ob-enabled" style="font-size: 0.85rem; color: #cbd5e1; cursor: pointer;">
              立即启用该出站
            </label>
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">保存出站规则</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import api from '../api'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

interface OutboundItem {
  id: number
  tag: string
  type: string
  server: string
  server_port: number
  spec_json: string
  priority: number
  enabled: boolean
  is_system: boolean
}

interface RelayInbound {
  id: number
  tag: string
  protocol: string
  port: number
  spec_json: string
  tunnel_domain?: string
  summary: string
}

interface RelayNode {
  node_id: number
  node_name: string
  host: string
  inbounds: RelayInbound[]
}

const { toast } = useToast()
const { showConfirm } = useConfirm()

const outbounds = ref<OutboundItem[]>([])
const loading = ref(true)
const singboxInstalled = ref(true)

const createMode = ref<'relay' | 'manual'>('relay')
const relayCandidates = ref<RelayNode[]>([])
const loadingRelayCandidates = ref(false)
const selectedRelayNodeId = ref<number | null>(null)
const selectedRelayInboundId = ref<number | null>(null)
const selectedRelayAddrType = ref<'ip' | 'tunnel'>('ip')
const relayGeneratedSummary = ref('')
const relayGeneratedSpec = ref<any>(null)

const currentRelayNode = computed(() => {
  return relayCandidates.value.find(n => n.node_id === selectedRelayNodeId.value)
})

const currentRelayInbounds = computed(() => {
  return currentRelayNode.value ? currentRelayNode.value.inbounds : []
})

const selectedInbound = computed(() => {
  return currentRelayInbounds.value.find(ib => ib.id === selectedRelayInboundId.value)
})

const selectedInboundHasTunnel = computed(() => {
  return !!(selectedInbound.value && selectedInbound.value.tunnel_domain)
})

const modal = reactive({
  show: false,
  isEdit: false,
  isSystem: false,
  id: 0,
})

const form = reactive({
  tag: '',
  type: 'direct',
  server: '',
  server_port: 0,
  priority: 10,
  enabled: true,
})

const proxySpec = reactive({
  username: '',
  password: '',
  tls_enabled: false,
  tls_server_name: '',
  tls_insecure: false,
})

const ssSpec = reactive({
  method: '2022-blake3-aes-128-gcm',
  password: '',
  plugin: '',
  plugin_opts: '',
})

const vlessSpec = reactive({
  uuid: '',
  flow: 'xtls-rprx-vision',
  packet_encoding: 'xudp',
  tls_enabled: true,
  tls_server_name: '',
  tls_insecure: false,
  tls_utls_fingerprint: 'chrome',
  reality_enabled: false,
  reality_public_key: '',
  reality_short_id: '',
  transport_type: 'tcp' as 'tcp' | 'ws' | 'grpc',
  ws_path: '/ws',
  ws_host: '',
  grpc_service_name: '',
})

const trojanSpec = reactive({
  password: '',
  server_name: '',
  insecure: false,
  alpn: 'h2,http/1.1',
  transport_type: 'tcp' as 'tcp' | 'ws',
  ws_path: '/ws',
  ws_host: '',
})

const hysteria2Spec = reactive({
  password: '',
  server_name: '',
  insecure: true,
  alpn: 'h3',
  obfs_type: 'none' as 'none' | 'salamander',
  obfs_password: '',
  up_mbps: undefined as number | undefined,
  down_mbps: undefined as number | undefined,
})

const vmessSpec = reactive({
  uuid: '',
  security: 'auto',
  alter_id: 0,
  tls_enabled: false,
  tls_server_name: '',
  tls_insecure: false,
  transport_type: 'tcp' as 'tcp' | 'ws',
  ws_path: '/ws',
  ws_host: '',
})

const tuicSpec = reactive({
  uuid: '',
  password: '',
  congestion_control: 'bbr',
  server_name: '',
  insecure: true,
  alpn: 'h3',
})

const customSpecJSON = ref('')

function resetSpecs() {
  proxySpec.username = ''
  proxySpec.password = ''
  proxySpec.tls_enabled = false
  proxySpec.tls_server_name = ''
  proxySpec.tls_insecure = false

  ssSpec.method = '2022-blake3-aes-128-gcm'
  ssSpec.password = ''
  ssSpec.plugin = ''
  ssSpec.plugin_opts = ''

  vlessSpec.uuid = ''
  vlessSpec.flow = 'xtls-rprx-vision'
  vlessSpec.packet_encoding = 'xudp'
  vlessSpec.tls_enabled = true
  vlessSpec.tls_server_name = ''
  vlessSpec.tls_insecure = false
  vlessSpec.tls_utls_fingerprint = 'chrome'
  vlessSpec.reality_enabled = false
  vlessSpec.reality_public_key = ''
  vlessSpec.reality_short_id = ''
  vlessSpec.transport_type = 'tcp'
  vlessSpec.ws_path = '/ws'
  vlessSpec.ws_host = ''
  vlessSpec.grpc_service_name = ''

  trojanSpec.password = ''
  trojanSpec.server_name = ''
  trojanSpec.insecure = false
  trojanSpec.alpn = 'h2,http/1.1'
  trojanSpec.transport_type = 'tcp'
  trojanSpec.ws_path = '/ws'
  trojanSpec.ws_host = ''

  hysteria2Spec.password = ''
  hysteria2Spec.server_name = ''
  hysteria2Spec.insecure = true
  hysteria2Spec.alpn = 'h3'
  hysteria2Spec.obfs_type = 'none'
  hysteria2Spec.obfs_password = ''
  hysteria2Spec.up_mbps = undefined
  hysteria2Spec.down_mbps = undefined

  vmessSpec.uuid = ''
  vmessSpec.security = 'auto'
  vmessSpec.alter_id = 0
  vmessSpec.tls_enabled = false
  vmessSpec.tls_server_name = ''
  vmessSpec.tls_insecure = false
  vmessSpec.transport_type = 'tcp'
  vmessSpec.ws_path = '/ws'
  vmessSpec.ws_host = ''

  tuicSpec.uuid = ''
  tuicSpec.password = ''
  tuicSpec.congestion_control = 'bbr'
  tuicSpec.server_name = ''
  tuicSpec.insecure = true
  tuicSpec.alpn = 'h3'

  customSpecJSON.value = ''
}

function populateFromSpec(type: string, spec: any) {
  if (!spec || typeof spec !== 'object') return

  if (type === 'vless') {
    vlessSpec.uuid = spec.uuid || ''
    vlessSpec.flow = spec.flow || ''
    vlessSpec.packet_encoding = spec.packet_encoding || 'xudp'

    const tls = spec.tls || {}
    vlessSpec.tls_enabled = !!tls.enabled
    vlessSpec.tls_server_name = tls.server_name || ''
    vlessSpec.tls_insecure = !!tls.insecure
    if (tls.utls?.fingerprint) {
      vlessSpec.tls_utls_fingerprint = tls.utls.fingerprint
    }
    const reality = tls.reality || {}
    vlessSpec.reality_enabled = !!reality.enabled
    vlessSpec.reality_public_key = reality.public_key || ''
    vlessSpec.reality_short_id = reality.short_id || ''

    const tr = spec.transport || {}
    vlessSpec.transport_type = tr.type || 'tcp'
    vlessSpec.ws_path = tr.path || '/ws'
    vlessSpec.ws_host = tr.headers?.Host || ''
    vlessSpec.grpc_service_name = tr.service_name || ''
  } else if (type === 'trojan') {
    trojanSpec.password = spec.password || ''
    const tls = spec.tls || {}
    trojanSpec.server_name = tls.server_name || spec.server_name || ''
    trojanSpec.insecure = !!tls.insecure
    trojanSpec.alpn = Array.isArray(tls.alpn) ? tls.alpn.join(',') : (tls.alpn || '')

    const tr = spec.transport || {}
    trojanSpec.transport_type = tr.type || 'tcp'
    trojanSpec.ws_path = tr.path || '/ws'
    trojanSpec.ws_host = tr.headers?.Host || ''
  } else if (type === 'hysteria2') {
    hysteria2Spec.password = spec.password || ''
    const tls = spec.tls || {}
    hysteria2Spec.server_name = tls.server_name || spec.server_name || ''
    hysteria2Spec.insecure = tls.insecure !== undefined ? !!tls.insecure : true
    hysteria2Spec.alpn = Array.isArray(tls.alpn) ? tls.alpn.join(',') : (tls.alpn || 'h3')

    if (spec.obfs && spec.obfs.type === 'salamander') {
      hysteria2Spec.obfs_type = 'salamander'
      hysteria2Spec.obfs_password = spec.obfs.password || ''
    } else {
      hysteria2Spec.obfs_type = 'none'
      hysteria2Spec.obfs_password = ''
    }
    hysteria2Spec.up_mbps = spec.up_mbps
    hysteria2Spec.down_mbps = spec.down_mbps
  } else if (type === 'vmess') {
    vmessSpec.uuid = spec.uuid || ''
    vmessSpec.security = spec.security || 'auto'
    vmessSpec.alter_id = spec.alter_id || 0
    const tls = spec.tls || {}
    vmessSpec.tls_enabled = !!tls.enabled
    vmessSpec.tls_server_name = tls.server_name || ''
    vmessSpec.tls_insecure = !!tls.insecure
    const tr = spec.transport || {}
    vmessSpec.transport_type = tr.type || 'tcp'
    vmessSpec.ws_path = tr.path || '/ws'
    vmessSpec.ws_host = tr.headers?.Host || ''
  } else if (type === 'tuic') {
    tuicSpec.uuid = spec.uuid || ''
    tuicSpec.password = spec.password || ''
    tuicSpec.congestion_control = spec.congestion_control || 'bbr'
    const tls = spec.tls || {}
    tuicSpec.server_name = tls.server_name || spec.server_name || ''
    tuicSpec.insecure = tls.insecure !== undefined ? !!tls.insecure : true
    tuicSpec.alpn = Array.isArray(tls.alpn) ? tls.alpn.join(',') : (tls.alpn || 'h3')
  } else if (type === 'shadowsocks') {
    ssSpec.method = spec.method || '2022-blake3-aes-128-gcm'
    ssSpec.password = spec.password || ''
    ssSpec.plugin = spec.plugin || ''
    ssSpec.plugin_opts = spec.plugin_opts || ''
  } else if (type === 'socks' || type === 'http') {
    proxySpec.username = spec.username || ''
    proxySpec.password = spec.password || ''
    const tls = spec.tls || {}
    proxySpec.tls_enabled = !!tls.enabled
    proxySpec.tls_server_name = tls.server_name || ''
    proxySpec.tls_insecure = !!tls.insecure
  }
}

function buildSpecFromForm(type: string): any {
  if (type === 'vless') {
    const spec: any = {
      uuid: vlessSpec.uuid.trim(),
    }
    if (vlessSpec.flow) {
      spec.flow = vlessSpec.flow
    }
    if (vlessSpec.packet_encoding) {
      spec.packet_encoding = vlessSpec.packet_encoding
    }
    if (vlessSpec.tls_enabled) {
      const tls: any = {
        enabled: true,
      }
      if (vlessSpec.tls_server_name.trim()) {
        tls.server_name = vlessSpec.tls_server_name.trim()
      }
      if (vlessSpec.tls_insecure) {
        tls.insecure = true
      }
      if (vlessSpec.tls_utls_fingerprint) {
        tls.utls = {
          enabled: true,
          fingerprint: vlessSpec.tls_utls_fingerprint,
        }
      }
      if (vlessSpec.reality_enabled) {
        tls.reality = {
          enabled: true,
          public_key: vlessSpec.reality_public_key.trim(),
          short_id: vlessSpec.reality_short_id.trim(),
        }
      }
      spec.tls = tls
    }
    if (vlessSpec.transport_type === 'ws') {
      const tr: any = {
        type: 'ws',
        path: vlessSpec.ws_path.trim() || '/ws',
      }
      if (vlessSpec.ws_host.trim()) {
        tr.headers = { Host: vlessSpec.ws_host.trim() }
      }
      spec.transport = tr
    } else if (vlessSpec.transport_type === 'grpc') {
      spec.transport = {
        type: 'grpc',
        service_name: vlessSpec.grpc_service_name.trim(),
      }
    }
    return spec
  }

  if (type === 'trojan') {
    const spec: any = {
      password: trojanSpec.password.trim(),
      tls: {
        enabled: true,
        server_name: trojanSpec.server_name.trim() || form.server.trim(),
        insecure: trojanSpec.insecure,
      },
    }
    if (trojanSpec.alpn.trim()) {
      spec.tls.alpn = trojanSpec.alpn.split(',').map((s) => s.trim()).filter(Boolean)
    }
    if (trojanSpec.transport_type === 'ws') {
      const tr: any = {
        type: 'ws',
        path: trojanSpec.ws_path.trim() || '/ws',
      }
      if (trojanSpec.ws_host.trim()) {
        tr.headers = { Host: trojanSpec.ws_host.trim() }
      }
      spec.transport = tr
    }
    return spec
  }

  if (type === 'hysteria2') {
    const spec: any = {
      password: hysteria2Spec.password.trim(),
      tls: {
        enabled: true,
        server_name: hysteria2Spec.server_name.trim() || form.server.trim(),
        insecure: hysteria2Spec.insecure,
      },
    }
    if (hysteria2Spec.alpn.trim()) {
      spec.tls.alpn = hysteria2Spec.alpn.split(',').map((s) => s.trim()).filter(Boolean)
    }
    if (hysteria2Spec.obfs_type === 'salamander' && hysteria2Spec.obfs_password.trim()) {
      spec.obfs = {
        type: 'salamander',
        password: hysteria2Spec.obfs_password.trim(),
      }
    }
    if (hysteria2Spec.up_mbps && hysteria2Spec.up_mbps > 0) {
      spec.up_mbps = hysteria2Spec.up_mbps
    }
    if (hysteria2Spec.down_mbps && hysteria2Spec.down_mbps > 0) {
      spec.down_mbps = hysteria2Spec.down_mbps
    }
    return spec
  }

  if (type === 'vmess') {
    const spec: any = {
      uuid: vmessSpec.uuid.trim(),
      security: vmessSpec.security || 'auto',
      alter_id: vmessSpec.alter_id || 0,
    }
    if (vmessSpec.tls_enabled) {
      spec.tls = {
        enabled: true,
        server_name: vmessSpec.tls_server_name.trim() || form.server.trim(),
        insecure: vmessSpec.tls_insecure,
      }
    }
    if (vmessSpec.transport_type === 'ws') {
      const tr: any = {
        type: 'ws',
        path: vmessSpec.ws_path.trim() || '/ws',
      }
      if (vmessSpec.ws_host.trim()) {
        tr.headers = { Host: vmessSpec.ws_host.trim() }
      }
      spec.transport = tr
    }
    return spec
  }

  if (type === 'tuic') {
    const spec: any = {
      uuid: tuicSpec.uuid.trim(),
      password: tuicSpec.password.trim(),
      congestion_control: tuicSpec.congestion_control || 'bbr',
      tls: {
        enabled: true,
        server_name: tuicSpec.server_name.trim() || form.server.trim(),
        insecure: tuicSpec.insecure,
      },
    }
    if (tuicSpec.alpn.trim()) {
      spec.tls.alpn = tuicSpec.alpn.split(',').map((s) => s.trim()).filter(Boolean)
    }
    return spec
  }

  if (type === 'shadowsocks') {
    const spec: any = {
      method: ssSpec.method,
      password: ssSpec.password.trim(),
    }
    if (ssSpec.plugin.trim()) {
      spec.plugin = ssSpec.plugin.trim()
      if (ssSpec.plugin_opts.trim()) {
        spec.plugin_opts = ssSpec.plugin_opts.trim()
      }
    }
    return spec
  }

  if (type === 'socks' || type === 'http') {
    const spec: any = {}
    if (proxySpec.username.trim()) spec.username = proxySpec.username.trim()
    if (proxySpec.password.trim()) spec.password = proxySpec.password.trim()
    if (type === 'http' && proxySpec.tls_enabled) {
      spec.tls = {
        enabled: true,
        server_name: proxySpec.tls_server_name.trim() || form.server.trim(),
        insecure: proxySpec.tls_insecure,
      }
    }
    return spec
  }

  return {}
}

function getOutboundFeatures(ob: OutboundItem) {
  const feats: string[] = []
  try {
    const spec = JSON.parse(ob.spec_json || '{}')
    if (spec.flow) feats.push(spec.flow)
    if (spec.tls?.reality?.enabled) feats.push('Reality')
    else if (spec.tls?.enabled) feats.push('TLS')
    if (spec.transport?.type) feats.push(spec.transport.type.toUpperCase())
    if (spec.obfs?.type) feats.push(`Obfs:${spec.obfs.type}`)
    if (spec.plugin) feats.push(`Plugin:${spec.plugin}`)
  } catch {}
  return feats
}

async function loadOutbounds() {
  loading.value = true
  try {
    const res = await api.get('/outbounds')
    outbounds.value = res.data || []

    const compRes = await api.get('/components')
    const sb = (compRes.data || []).find((c: any) => c.name === 'sing-box')
    if (sb) {
      singboxInstalled.value = sb.installed
    }
  } catch (err: any) {
    toast.error('加载出站规则失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

function getTypeBadge(type: string) {
  switch (type) {
    case 'direct': return 'badge-success'
    case 'block': return 'badge-danger'
    case 'socks':
    case 'http': return 'badge-warning'
    case 'shadowsocks':
    case 'vless':
    case 'trojan':
    case 'hysteria2':
    case 'vmess':
    case 'tuic':
    case 'wireguard': return 'badge-primary'
    default: return 'badge-secondary'
  }
}

function onTypeChange() {
  if (form.type === 'direct' || form.type === 'block') {
    form.server = ''
    form.server_port = 0
  } else if (form.server_port === 0) {
    if (form.type === 'vless' || form.type === 'trojan' || form.type === 'hysteria2' || form.type === 'vmess') {
      form.server_port = 443
    } else if (form.type === 'tuic') {
      form.server_port = 8443
    } else if (form.type === 'shadowsocks') {
      form.server_port = 8388
    } else if (form.type === 'socks') {
      form.server_port = 1080
    } else if (form.type === 'http') {
      form.server_port = 8080
    }
  }
}

async function loadRelayCandidates() {
  loadingRelayCandidates.value = true
  try {
    const res = await api.get('/cluster/relay-candidates')
    relayCandidates.value = res.data || []
    if (relayCandidates.value.length > 0) {
      selectedRelayNodeId.value = relayCandidates.value[0].node_id
      if (relayCandidates.value[0].inbounds && relayCandidates.value[0].inbounds.length > 0) {
        selectedRelayInboundId.value = relayCandidates.value[0].inbounds[0].id
        await fetchAndApplyRelayOutbound()
      }
    } else {
      createMode.value = 'manual'
    }
  } catch (err: any) {
    createMode.value = 'manual'
  } finally {
    loadingRelayCandidates.value = false
  }
}

function onRelayNodeChange() {
  if (currentRelayInbounds.value.length > 0) {
    selectedRelayInboundId.value = currentRelayInbounds.value[0].id
    fetchAndApplyRelayOutbound()
  } else {
    selectedRelayInboundId.value = null
    relayGeneratedSummary.value = ''
    relayGeneratedSpec.value = null
  }
}

async function fetchAndApplyRelayOutbound() {
  if (!selectedRelayNodeId.value || !selectedRelayInboundId.value) return
  try {
    const res = await api.post('/cluster/build-relay-outbound', {
      target_node_id: selectedRelayNodeId.value,
      inbound_id: selectedRelayInboundId.value,
      addr_type: selectedRelayAddrType.value,
    })
    const generated = res.data
    form.tag = generated.tag
    form.type = generated.type
    form.server = generated.server
    form.server_port = generated.server_port
    form.priority = 10

    try {
      const parsed = JSON.parse(generated.spec_json || '{}')
      relayGeneratedSpec.value = parsed
      customSpecJSON.value = JSON.stringify(parsed, null, 2)
      populateFromSpec(form.type, parsed)
    } catch {
      relayGeneratedSpec.value = null
    }

    const nodeName = currentRelayNode.value ? currentRelayNode.value.node_name : '目标节点'
    relayGeneratedSummary.value = `已自动从【${nodeName}】解析并注入 ${form.type.toUpperCase()} 中转协议与加密参数`
  } catch (err: any) {
    toast.error('解析中转配置失败: ' + (err.response?.data?.error || err.message))
  }
}

function switchCreateMode(mode: 'relay' | 'manual') {
  createMode.value = mode
  if (mode === 'relay') {
    if (relayCandidates.value.length === 0) {
      loadRelayCandidates()
    } else if (selectedRelayNodeId.value && selectedRelayInboundId.value) {
      fetchAndApplyRelayOutbound()
    }
  } else {
    relayGeneratedSummary.value = ''
    relayGeneratedSpec.value = null
  }
}

function openCreateModal() {
  modal.isEdit = false
  modal.isSystem = false
  modal.id = 0
  modal.show = true

  createMode.value = 'relay'
  relayGeneratedSummary.value = ''
  relayGeneratedSpec.value = null
  selectedRelayAddrType.value = 'ip'

  form.tag = ''
  form.type = 'vless'
  form.server = ''
  form.server_port = 443
  form.priority = 10
  form.enabled = true

  resetSpecs()
  loadRelayCandidates()
}

function openEditModal(ob: OutboundItem) {
  modal.isEdit = true
  modal.isSystem = ob.is_system
  modal.id = ob.id
  modal.show = true

  form.tag = ob.tag
  form.type = ob.type
  form.server = ob.server
  form.server_port = ob.server_port
  form.priority = ob.priority
  form.enabled = ob.enabled

  resetSpecs()
  try {
    const spec = JSON.parse(ob.spec_json || '{}')
    customSpecJSON.value = JSON.stringify(spec, null, 2)
    populateFromSpec(ob.type, spec)
  } catch {
    customSpecJSON.value = ob.spec_json || ''
  }
}

async function submitOutbound() {
  if (!form.tag.trim()) {
    toast.warning('请输入出站标识 Tag')
    return
  }

  let specObj: any = {}
  if (form.type === 'custom') {
    try {
      specObj = JSON.parse(customSpecJSON.value || '{}')
    } catch {
      toast.error('自定义参数必须是合法的 JSON 格式！')
      return
    }
  } else if (form.type !== 'direct' && form.type !== 'block') {
    specObj = buildSpecFromForm(form.type)
  }

  const payload = {
    tag: form.tag.trim(),
    type: form.type,
    server: form.server.trim(),
    server_port: form.server_port,
    spec: specObj,
    priority: form.priority,
    enabled: form.enabled,
  }

  try {
    if (modal.isEdit) {
      await api.put(`/outbounds/${modal.id}`, payload)
      toast.success('出站规则已更新并热重载！')
    } else {
      await api.post('/outbounds', payload)
      toast.success('出站规则已添加并生效！')
    }
    modal.show = false
    await loadOutbounds()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

async function toggleOutbound(id: number) {
  try {
    await api.post(`/outbounds/${id}/toggle`)
    toast.success('已切换出站状态并重载配置！')
    await loadOutbounds()
  } catch (err: any) {
    toast.error('切换状态失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteOutbound(ob: OutboundItem) {
  const confirmed = await showConfirm({
    title: '确认删除出站规则',
    message: `确定要删除出站规则 [${ob.tag}] 吗？删除后 sing-box 将自动重写配置并生效。`,
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.delete(`/outbounds/${ob.id}`)
    toast.success('出站规则已成功删除！')
    await loadOutbounds()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  loadOutbounds()
  window.addEventListener('node-context-changed', loadOutbounds)
})

onUnmounted(() => {
  window.removeEventListener('node-context-changed', loadOutbounds)
})
</script>

