<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🌐</span>
          <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">Sing-box 入站节点配置</h1>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          支持直连首选的 VLESS-Reality、Cloudflared 隧道 VLESS-WS、极速防封 Hysteria 2、Trojan、Shadowsocks、VMess、TUIC v5 及本地 SOCKS5/Mixed 代理入站
        </p>
      </div>

      <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap;">
        <button @click="openCreateModal" class="btn btn-primary">+ 新增节点</button>
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
              入站节点由底层的 sing-box 驱动并监听。您可以先在此预设规则，或直接进入软件中心一键安装。
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
      <p class="loading-text">正在加载入站节点配置...</p>
    </div>

    <!-- 节点列表空状态 -->
    <div v-else-if="inbounds.length === 0" class="card" style="text-align: center; padding: 2.5rem 1.5rem;">
      <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">🌐</div>
      <h3 style="font-size: 1.1rem; font-weight: 600; color: #f8fafc; margin-bottom: 0.5rem;">暂无已配置的入站节点</h3>
      <p style="color: #94a3b8; font-size: 0.85rem; max-width: 540px; margin: 0 auto 1.25rem; line-height: 1.5;">
        若在安装面板前宿主机已安装 Sing-box 并在 <code>/etc/sing-box/config.json</code> 中配置过节点，点击下方按钮可一键导入至面板集中管理；亦可直接新增首个节点。
      </p>
      <div style="display: flex; justify-content: center; gap: 0.75rem; flex-wrap: wrap;">
        <button @click="importExistingConfig" class="btn btn-secondary" :disabled="importing">
          <span>📥</span>
          <span>{{ importing ? '正在导入...' : '一键导入宿主机已有配置' }}</span>
        </button>
        <button @click="openCreateModal" class="btn btn-primary">+ 一键配置首个 VLESS-Reality 节点</button>
      </div>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div v-for="ib in inbounds" :key="ib.id" class="card" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">{{ ib.tag }}</span>
            <span :class="['badge', getProtocolBadge(ib.protocol)]">
              {{ ib.protocol }}
            </span>
            <span :class="['badge', ib.enabled ? 'badge-success' : 'badge-secondary']">
              {{ ib.enabled ? '已启用' : '已停用' }}
            </span>
          </div>

          <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; display: flex; gap: 1.5rem; flex-wrap: wrap;">
            <span>监听端口: <b style="color: #f1f5f9;">{{ ib.port }}</b></span>
            <span>绑定地址: <b style="color: #f1f5f9;">{{ ib.listen }}</b></span>
            <span v-if="ib.protocol === 'vless-reality'">
              SNI 伪装: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).server_name }}</b>
            </span>
            <span v-else-if="ib.protocol === 'vless-ws'">
              WS Path: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).path || '/ws' }}</b>
              <span v-if="parseSpec(ib.spec_json).group_domain" style="margin-left: 0.75rem;">
                分组域名: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).group_domain }}</b>
              </span>
            </span>
            <span v-else-if="ib.protocol === 'hysteria2'">
              SNI: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).server_name || '默认' }}</b>
              <span v-if="parseSpec(ib.spec_json).up_mbps" style="margin-left: 0.75rem;">
                带宽限制: <b style="color: #34d399;">↑{{ parseSpec(ib.spec_json).up_mbps }}M ↓{{ parseSpec(ib.spec_json).down_mbps || parseSpec(ib.spec_json).up_mbps }}M</b>
              </span>
            </span>
            <span v-else-if="ib.protocol === 'trojan'">
              SNI: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).server_name || '默认' }}</b>
            </span>
            <span v-else-if="ib.protocol === 'shadowsocks'">
              加密方法: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).method || '2022-blake3-aes-128-gcm' }}</b>
            </span>
            <span v-else-if="ib.protocol === 'vmess'">
              传输模式: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).network === 'ws' ? 'WebSocket (' + (parseSpec(ib.spec_json).path || '/vmess') + ')' : 'TCP' }}</b>
            </span>
            <span v-else-if="ib.protocol === 'tuic'">
              拥塞控制: <b style="color: #38bdf8;">{{ (parseSpec(ib.spec_json).congestion_control || 'bbr').toUpperCase() }}</b>
            </span>
            <span v-else-if="ib.protocol === 'socks' || ib.protocol === 'mixed' || ib.protocol === 'http'">
              鉴权方式: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).username ? '账号密码 (' + parseSpec(ib.spec_json).username + ')' : '免密匿名' }}</b>
            </span>
          </div>
        </div>

        <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap;">
          <button
            @click="toggleInbound(ib)"
            :class="['btn', ib.enabled ? 'btn-secondary' : 'btn-primary']"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
          >
            {{ ib.enabled ? '⏸️ 停用' : '▶️ 启用' }}
          </button>
          <button @click="showShareModal(ib)" class="btn btn-secondary" style="padding: 0.4rem 0.75rem; font-size: 0.8rem;">
            📱 二维码 / 节点链接
          </button>
          <button @click="openEditModal(ib)" class="btn btn-secondary" style="padding: 0.4rem 0.75rem; font-size: 0.8rem;">
            ✏️ 编辑
          </button>
          <button
            @click="deleteInbound(ib.id)"
            class="btn btn-danger"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
            :disabled="deletingId === ib.id"
          >
            {{ deletingId === ib.id ? '⏳ 删除中...' : '🗑️ 删除' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem;">
          {{ modal.isEdit ? '编辑入站节点' : '新建入站节点' }}
        </h3>

        <form @submit.prevent="submitInbound" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">协议类型</label>
            <select v-model="form.protocol" class="input" @change="onProtocolChange">
              <option value="vless-reality">VLESS-Vision-Reality (直连首选、极速免证书)</option>
              <option value="vless-ws">VLESS-WS (与 Cloudflared 配合使用)</option>
              <option value="hysteria2">Hysteria 2 (UDP/QUIC 极速免封、抗丢包利器)</option>
              <option value="trojan">Trojan (标准 TLS 伪装协议)</option>
              <option value="shadowsocks">Shadowsocks (经典 / SS-2022 对称加密)</option>
              <option value="vmess">VMess (经典 VMess-TCP / WS)</option>
              <option value="tuic">TUIC v5 (基于 QUIC 拥塞控制现代协议)</option>
              <option value="socks">SOCKS5 (本地/回环独立代理)</option>
              <option value="mixed">Mixed (HTTP & SOCKS5 双协议混合入站)</option>
            </select>
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">节点标识 Tag</label>
              <input v-model="form.tag" class="input" required placeholder="如 vless-reality-443" />
            </div>
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">监听端口</label>
              <input v-model.number="form.port" type="number" class="input" required placeholder="443" />
            </div>
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">绑定监听地址</label>
              <input v-model="form.listen" class="input" placeholder="如 :: 或 0.0.0.0 或 127.0.0.1" />
            </div>
            <div style="display: flex; align-items: center; padding-top: 1.25rem;">
              <label style="display: flex; align-items: center; gap: 0.5rem; font-size: 0.875rem; color: #f1f5f9; cursor: pointer;">
                <input type="checkbox" v-model="form.enabled" style="accent-color: #38bdf8; width: 1.1rem; height: 1.1rem;" />
                启用此入站规则
              </label>
            </div>
          </div>

          <!-- Reality 专属参数 -->
          <div v-if="form.protocol === 'vless-reality'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-weight: 500; font-size: 0.875rem; color: #38bdf8;">Reality 伪装与密钥配置</span>
              <button type="button" @click="generateRealityKeys" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                ⚡ 自动生成密钥对与 ShortID
              </button>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">伪装目标域名 (SNI)</label>
                <input v-model="realitySpec.server_name" class="input" placeholder="dl.google.com" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">客户端指纹 (Fingerprint)</label>
                <input v-model="realitySpec.fingerprint" class="input" placeholder="chrome" />
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">服务端私钥 (Private Key)</label>
              <input v-model="realitySpec.private_key" class="input" required placeholder="点击右上角自动生成" @blur="onPrivateKeyBlur" />
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">
                客户端公钥 (Public Key - pbk)
                <span style="color: #64748b; font-weight: normal; margin-left: 0.35rem;">(选填，留空将自动从私钥推导)</span>
              </label>
              <input v-model="realitySpec.public_key" class="input" placeholder="选填：留空将自动根据私钥推导，或点击右上角生成" />
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">Short ID (短 ID)</label>
              <input v-model="realitySpec.short_id" class="input" required placeholder="8位十六进制" />
            </div>
          </div>

          <!-- VLESS-WS 专属参数 -->
          <div v-else-if="form.protocol === 'vless-ws'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.25rem;">分组指定域名 (SNI / Host)</label>
              <input v-model="wsSpec.group_domain" class="input" placeholder="如 argo.example.com 或留空继承默认" />
              <div style="font-size: 0.725rem; color: #64748b; margin-top: 0.25rem;">
                生成订阅时，优选 IP 作为连接目标，该域名作为分组 TLS SNI 与 HTTP Host。留空时自动回退继承全局优选域名或穿透域名。
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.25rem;">WebSocket Path</label>
              <input v-model="wsSpec.path" class="input" placeholder="/ws" />
            </div>
          </div>

          <!-- Hysteria 2 专属参数 -->
          <div v-else-if="form.protocol === 'hysteria2'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-weight: 500; font-size: 0.875rem; color: #38bdf8;">Hysteria 2 UDP 极速参数</span>
              <div style="display: flex; gap: 0.5rem;">
                <button type="button" @click="generateTLSCertForInbound('hy2')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                  ⚡ 一键就绪自签名证书
                </button>
                <button type="button" @click="generateRandomPassword('hy2')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                  🔑 随机密码
                </button>
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">专属鉴权密码 (留空则自动复用团队多租户身份)</label>
                <input v-model="hy2Spec.password" class="input" placeholder="留空则复用成员多租户" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS 伪装域名 (SNI)</label>
                <input v-model="hy2Spec.server_name" class="input" placeholder="如 mydomain.com 或留空使用服务器 IP" />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">证书路径 (Certificate Path)</label>
                <input v-model="hy2Spec.certificate_path" class="input" placeholder="留空自动使用 /etc/sing-box/cert.pem" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">私钥路径 (Key Path)</label>
                <input v-model="hy2Spec.key_path" class="input" placeholder="留空自动使用 /etc/sing-box/key.pem" />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上行带宽 Mbps (0为无限制)</label>
                <input v-model.number="hy2Spec.up_mbps" type="number" class="input" placeholder="0" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">下行带宽 Mbps (0为无限制)</label>
                <input v-model.number="hy2Spec.down_mbps" type="number" class="input" placeholder="0" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">混淆密码 Salamander (选填)</label>
                <input v-model="hy2Spec.obfs_password" class="input" placeholder="选填混淆密码" />
              </div>
            </div>
          </div>

          <!-- Trojan 专属参数 -->
          <div v-else-if="form.protocol === 'trojan'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-weight: 500; font-size: 0.875rem; color: #38bdf8;">Trojan TLS 伪装配置</span>
              <div style="display: flex; gap: 0.5rem;">
                <button type="button" @click="generateTLSCertForInbound('trojan')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                  ⚡ 一键就绪自签名证书
                </button>
                <button type="button" @click="generateRandomPassword('trojan')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                  🔑 随机密码
                </button>
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">专属鉴权密码 (留空则自动复用团队多租户身份)</label>
                <input v-model="trojanSpec.password" class="input" placeholder="留空则复用成员多租户" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS 伪装域名 (SNI)</label>
                <input v-model="trojanSpec.server_name" class="input" placeholder="如 mydomain.com 或留空使用服务器 IP" />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">证书路径 (Certificate Path)</label>
                <input v-model="trojanSpec.certificate_path" class="input" placeholder="留空自动使用 /etc/sing-box/cert.pem" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">私钥路径 (Key Path)</label>
                <input v-model="trojanSpec.key_path" class="input" placeholder="留空自动使用 /etc/sing-box/key.pem" />
              </div>
            </div>
          </div>

          <!-- Shadowsocks 专属参数 -->
          <div v-else-if="form.protocol === 'shadowsocks'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-weight: 500; font-size: 0.875rem; color: #38bdf8;">Shadowsocks 对称加密配置</span>
              <button type="button" @click="generateRandomSSPassword" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                🔑 随机生成密码
              </button>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">加密方法 (Method)</label>
                <select v-model="ssSpec.method" class="input">
                  <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm (SS2022 极速)</option>
                  <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm (SS2022 高强)</option>
                  <option value="aes-128-gcm">aes-128-gcm (AEAD)</option>
                  <option value="aes-256-gcm">aes-256-gcm (AEAD)</option>
                  <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
                </select>
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">密码 (Password)</label>
                <input v-model="ssSpec.password" class="input" required placeholder="SS 密码" />
              </div>
            </div>
          </div>

          <!-- VMess 专属参数 -->
          <div v-else-if="form.protocol === 'vmess'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="font-size: 0.8rem; color: #94a3b8;">
              💡 <strong>VMess 协议</strong>：经典 VMess 入站，支持直接 TCP 接入或搭配 WebSocket 传输。
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">底层传输协议</label>
                <select v-model="vmessSpec.network" class="input">
                  <option value="ws">WebSocket (推荐，配合 CDN/穿透)</option>
                  <option value="tcp">TCP (直连原生)</option>
                </select>
              </div>
              <div v-if="vmessSpec.network === 'ws'">
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">WebSocket Path</label>
                <input v-model="vmessSpec.path" class="input" placeholder="/vmess" />
              </div>
            </div>
          </div>

          <!-- TUIC 专属参数 -->
          <div v-else-if="form.protocol === 'tuic'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-weight: 500; font-size: 0.875rem; color: #38bdf8;">TUIC v5 极速 QUIC 代理</span>
              <div style="display: flex; gap: 0.5rem;">
                <button type="button" @click="generateTLSCertForInbound('tuic')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                  ⚡ 一键就绪自签名证书
                </button>
                <button type="button" @click="generateRandomPassword('tuic')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                  🔑 随机密码
                </button>
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证密码 (Password)</label>
                <input v-model="tuicSpec.password" class="input" placeholder="默认 tuic-password" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">拥塞控制算法 (Congestion Control)</label>
                <select v-model="tuicSpec.congestion_control" class="input">
                  <option value="bbr">BBR (推荐，高速防丢包)</option>
                  <option value="cubic">Cubic</option>
                  <option value="new_reno">New Reno</option>
                </select>
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS 伪装域名 (SNI)</label>
                <input v-model="tuicSpec.server_name" class="input" placeholder="留空使用服务器 IP" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">证书路径 (Cert)</label>
                <input v-model="tuicSpec.certificate_path" class="input" placeholder="默认 /etc/sing-box/cert.pem" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">私钥路径 (Key)</label>
                <input v-model="tuicSpec.key_path" class="input" placeholder="默认 /etc/sing-box/key.pem" />
              </div>
            </div>
          </div>

          <!-- SOCKS / Mixed 专属参数 -->
          <div v-else-if="form.protocol === 'socks' || form.protocol === 'mixed'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="font-size: 0.8rem; color: #94a3b8; line-height: 1.4;">
              💡 <strong>{{ form.protocol === 'mixed' ? 'Mixed 混合代理' : 'SOCKS5 代理' }}</strong>：通常监听在本地回环（如 <code>127.0.0.1:{{ form.port }}</code>）供本机应用、Argo 隧道端口反代或本地应用代理。可按需设置账号密码鉴权，留空则为匿名免密。
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">用户名 (选填，留空免密)</label>
                <input v-model="socksSpec.username" class="input" placeholder="留空为匿名免密" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">密码 (选填，留空免密)</label>
                <input v-model="socksSpec.password" type="text" class="input" placeholder="留空为匿名免密" />
              </div>
            </div>
          </div>

          <!-- 用户鉴权与 Name 来源说明 -->
          <div v-if="['vless-reality', 'vless-ws', 'vmess', 'trojan', 'hysteria2', 'tuic'].includes(form.protocol)" style="background-color: rgba(30, 41, 59, 0.5); border: 1px dashed #334155; padding: 0.75rem 1rem; border-radius: 0.5rem; font-size: 0.8rem; color: #94a3b8;">
            <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.5rem;">
              <span>👥 <strong>用户列表 (users)</strong>：已自动同步当前所有启用的团队成员多租户身份。</span>
              <router-link to="/services?tab=subscriptions&sub=members" class="btn btn-secondary" style="font-size: 0.725rem; padding: 0.2rem 0.5rem; text-decoration: none;">
                管理成员与 Name ↗
              </router-link>
            </div>
            <div style="font-size: 0.725rem; color: #64748b; margin-top: 0.35rem; line-height: 1.4;">
              当本入站未设置独立专属密码时，系统将自动绑定所有成员的 UUID 作为凭证，支持一人一码及独立订阅分发。
            </div>
          </div>
          <div v-else style="background-color: rgba(30, 41, 59, 0.5); border: 1px dashed #334155; padding: 0.75rem 1rem; border-radius: 0.5rem; font-size: 0.8rem; color: #94a3b8;">
            ℹ️ <strong>用户鉴权说明</strong>：此协议入站使用上方独立的账号密码（或免密访问），不绑定团队成员 UUID 列表。
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary" :disabled="saving">取消</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              {{ saving ? '保存并应用中...' : '保存并应用' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- 节点链接与二维码分享弹窗 -->
    <div v-if="shareModal.show" class="modal-overlay" @click.self="shareModal.show = false">
      <div class="modal-content" style="text-align: center;">
        <h3 style="font-size: 1.125rem; font-weight: 600; margin-bottom: 1rem;">
          {{ shareModal.title }}
        </h3>

        <div style="background-color: white; padding: 1rem; border-radius: 0.5rem; display: inline-block; margin-bottom: 1rem;">
          <qrcode-vue :value="shareModal.uri" :size="200" level="M" />
        </div>

        <p style="color: #94a3b8; font-size: 0.8rem; margin-bottom: 0.5rem;">可使用 Shadowrocket / v2rayN 等客户端直接扫码导入</p>

        <textarea
          :value="shareModal.uri"
          readonly
          class="input"
          rows="3"
          style="font-size: 0.75rem; margin-bottom: 1rem; word-break: break-all;"
        ></textarea>

        <div style="display: flex; justify-content: center; gap: 0.75rem;">
          <button @click="copyShareUri" class="btn btn-primary">复制节点链接</button>
          <button @click="shareModal.show = false" class="btn btn-secondary">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import QrcodeVue from 'qrcode.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'
import { useNodeContext } from '../composables/useNodeContext'

const { toast } = useToast()
const { showConfirm } = useConfirm()
const { activeNode } = useNodeContext()

const inbounds = ref<any[]>([])
const loading = ref(true)
const saving = ref(false)
const singboxInstalled = ref(true)
const deletingId = ref<number | null>(null)

const modal = reactive({
  show: false,
  isEdit: false,
  id: 0,
})

const form = reactive({
  tag: '',
  protocol: 'vless-reality',
  listen: '::',
  port: 443,
  enabled: true,
})

const realitySpec = reactive({
  flow: 'xtls-rprx-vision',
  server_name: 'dl.google.com',
  handshake_server: 'dl.google.com',
  handshake_port: 443,
  private_key: '',
  public_key: '',
  short_id: '',
  fingerprint: 'chrome',
})

const wsSpec = reactive({
  path: '/ws',
  group_domain: '',
})

const socksSpec = reactive({
  username: '',
  password: '',
})

const mixedSpec = reactive({
  username: '',
  password: '',
})

const ssSpec = reactive({
  method: '2022-blake3-aes-128-gcm',
  password: '',
})

const trojanSpec = reactive({
  password: '',
  server_name: '',
  certificate_path: '',
  key_path: '',
})

const hy2Spec = reactive({
  password: '',
  server_name: '',
  certificate_path: '',
  key_path: '',
  up_mbps: 0,
  down_mbps: 0,
  obfs_password: '',
  ignore_client_bandwidth: false,
})

const vmessSpec = reactive({
  network: 'ws',
  path: '/vmess',
  group_domain: '',
})

const tuicSpec = reactive({
  password: '',
  congestion_control: 'bbr',
  server_name: '',
  certificate_path: '',
  key_path: '',
})

function getProtocolBadge(proto: string) {
  switch (proto) {
    case 'vless-reality':
    case 'hysteria2':
      return 'badge-success'
    case 'trojan':
    case 'tuic':
      return 'badge-primary'
    case 'vless-ws':
    case 'shadowsocks':
      return 'badge-warning'
    case 'vmess':
    case 'socks':
    case 'mixed':
    case 'http':
      return 'badge-info'
    default:
      return 'badge-secondary'
  }
}

async function generateTLSCertForInbound(target: 'hy2' | 'trojan' | 'tuic') {
  try {
    const res = await api.post('/inbounds/tls-cert')
    if (target === 'hy2') {
      hy2Spec.certificate_path = res.data.certificate_path
      hy2Spec.key_path = res.data.key_path
    } else if (target === 'trojan') {
      trojanSpec.certificate_path = res.data.certificate_path
      trojanSpec.key_path = res.data.key_path
    } else if (target === 'tuic') {
      tuicSpec.certificate_path = res.data.certificate_path
      tuicSpec.key_path = res.data.key_path
    }
    toast.success('自签名证书已就绪！')
  } catch (err: any) {
    toast.error('生成证书失败: ' + (err.response?.data?.error || err.message))
  }
}

function generateRandomSSPassword() {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let pwd = ''
  for (let i = 0; i < 24; i++) {
    pwd += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  ssSpec.password = pwd
}

function generateRandomPassword(target: 'hy2' | 'trojan' | 'tuic') {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let pwd = ''
  for (let i = 0; i < 18; i++) {
    pwd += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  if (target === 'hy2') hy2Spec.password = pwd
  else if (target === 'trojan') trojanSpec.password = pwd
  else if (target === 'tuic') tuicSpec.password = pwd
}

const shareModal = reactive({
  show: false,
  title: '',
  uri: '',
})

const importing = ref(false)

async function importExistingConfig() {
  importing.value = true
  try {
    const res = await api.post('/inbounds/import')
    toast.success(res.data.message || '导入成功！')
    await loadInbounds()
  } catch (err: any) {
    toast.error('导入宿主机配置失败: ' + (err.response?.data?.error || err.message))
  } finally {
    importing.value = false
  }
}

async function loadInbounds() {
  loading.value = true
  try {
    const res = await api.get('/inbounds')
    inbounds.value = res.data || []

    const compRes = await api.get('/components')
    const sb = (compRes.data || []).find((c: any) => c.name === 'sing-box')
    if (sb) {
      singboxInstalled.value = sb.installed
    }
  } catch (err: any) {
    toast.error('加载入站节点列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

function parseSpec(jsonStr: string) {
  try {
    return JSON.parse(jsonStr)
  } catch {
    return {}
  }
}

async function openCreateModal() {
  modal.isEdit = false
  modal.id = 0
  modal.show = true
  form.protocol = 'vless-reality'
  form.listen = '::'
  form.port = 443
  form.tag = 'vless-reality-443'
  form.enabled = true
  realitySpec.server_name = 'dl.google.com'
  realitySpec.handshake_server = 'dl.google.com'
  wsSpec.path = '/ws'
  wsSpec.group_domain = ''
  socksSpec.username = ''
  socksSpec.password = ''
  mixedSpec.username = ''
  mixedSpec.password = ''
  ssSpec.method = '2022-blake3-aes-128-gcm'
  ssSpec.password = ''
  trojanSpec.password = ''
  trojanSpec.server_name = ''
  trojanSpec.certificate_path = ''
  trojanSpec.key_path = ''
  hy2Spec.password = ''
  hy2Spec.server_name = ''
  hy2Spec.certificate_path = ''
  hy2Spec.key_path = ''
  hy2Spec.up_mbps = 0
  hy2Spec.down_mbps = 0
  hy2Spec.obfs_password = ''
  hy2Spec.ignore_client_bandwidth = false
  vmessSpec.network = 'ws'
  vmessSpec.path = '/vmess'
  vmessSpec.group_domain = ''
  tuicSpec.password = ''
  tuicSpec.congestion_control = 'bbr'
  tuicSpec.server_name = ''
  tuicSpec.certificate_path = ''
  tuicSpec.key_path = ''
  await generateRealityKeys()
}

function openEditModal(ib: any) {
  modal.isEdit = true
  modal.id = ib.id
  form.tag = ib.tag
  form.protocol = ib.protocol
  form.listen = ib.listen || (['vless-ws', 'socks', 'mixed', 'http'].includes(ib.protocol) ? '127.0.0.1' : '::')
  form.port = ib.port
  form.enabled = !!ib.enabled

  const spec = parseSpec(ib.spec_json)
  if (ib.protocol === 'vless-reality') {
    realitySpec.flow = spec.flow || 'xtls-rprx-vision'
    realitySpec.server_name = spec.server_name || 'dl.google.com'
    realitySpec.handshake_server = spec.handshake_server || spec.server_name || 'dl.google.com'
    realitySpec.handshake_port = spec.handshake_port || 443
    realitySpec.private_key = spec.private_key || ''
    realitySpec.public_key = spec.public_key || ''
    realitySpec.short_id = spec.short_id || ''
    realitySpec.fingerprint = spec.fingerprint || 'chrome'
  } else if (ib.protocol === 'vless-ws') {
    wsSpec.path = spec.path || '/ws'
    wsSpec.group_domain = spec.group_domain || spec.host || ''
  } else if (ib.protocol === 'socks' || ib.protocol === 'mixed' || ib.protocol === 'http') {
    socksSpec.username = spec.username || ''
    socksSpec.password = spec.password || ''
  } else if (ib.protocol === 'shadowsocks') {
    ssSpec.method = spec.method || '2022-blake3-aes-128-gcm'
    ssSpec.password = spec.password || ''
  } else if (ib.protocol === 'trojan') {
    trojanSpec.password = spec.password || ''
    trojanSpec.server_name = spec.server_name || ''
    trojanSpec.certificate_path = spec.certificate_path || ''
    trojanSpec.key_path = spec.key_path || ''
  } else if (ib.protocol === 'hysteria2') {
    hy2Spec.password = spec.password || ''
    hy2Spec.server_name = spec.server_name || ''
    hy2Spec.certificate_path = spec.certificate_path || ''
    hy2Spec.key_path = spec.key_path || ''
    hy2Spec.up_mbps = spec.up_mbps || 0
    hy2Spec.down_mbps = spec.down_mbps || 0
    hy2Spec.obfs_password = spec.obfs_password || ''
    hy2Spec.ignore_client_bandwidth = !!spec.ignore_client_bandwidth
  } else if (ib.protocol === 'vmess') {
    vmessSpec.network = spec.network || 'ws'
    vmessSpec.path = spec.path || '/vmess'
    vmessSpec.group_domain = spec.group_domain || ''
  } else if (ib.protocol === 'tuic') {
    tuicSpec.password = spec.password || ''
    tuicSpec.congestion_control = spec.congestion_control || 'bbr'
    tuicSpec.server_name = spec.server_name || ''
    tuicSpec.certificate_path = spec.certificate_path || ''
    tuicSpec.key_path = spec.key_path || ''
  }
  modal.show = true
}

async function toggleInbound(ib: any) {
  try {
    const res = await api.post(`/inbounds/${ib.id}/toggle`)
    ib.enabled = res.data.enabled
    toast.success(`入站规则 ${ib.tag} 已${res.data.enabled ? '启用' : '停用'}`)
  } catch (err: any) {
    toast.error('切换状态失败: ' + (err.response?.data?.error || err.message))
    await loadInbounds()
  }
}

async function generateRealityKeys() {
  try {
    const res = await api.post('/inbounds/reality-keys')
    realitySpec.private_key = res.data.private_key
    realitySpec.public_key = res.data.public_key
    realitySpec.short_id = res.data.short_id
  } catch (err: any) {
    toast.error('生成密钥失败: ' + (err.response?.data?.error || err.message))
  }
}

async function onPrivateKeyBlur() {
  const priv = realitySpec.private_key?.trim()
  if (priv && !realitySpec.public_key?.trim()) {
    try {
      const res = await api.post('/inbounds/derive-reality-key', { private_key: priv })
      if (res.data?.public_key) {
        realitySpec.public_key = res.data.public_key
      }
    } catch {
      // 容错处理：不阻断用户输入流程
    }
  }
}

function onProtocolChange() {
  if (form.protocol === 'vless-reality') {
    form.listen = '::'
    form.port = 443
    if (!modal.isEdit) form.tag = 'vless-reality-443'
  } else if (form.protocol === 'vless-ws') {
    form.listen = '127.0.0.1'
    form.port = 10086
    if (!modal.isEdit) form.tag = 'vless-ws-10086'
  } else if (form.protocol === 'hysteria2') {
    form.listen = '::'
    form.port = 8443
    if (!modal.isEdit) form.tag = 'hy2-8443'
  } else if (form.protocol === 'trojan') {
    form.listen = '::'
    form.port = 443
    if (!modal.isEdit) form.tag = 'trojan-443'
  } else if (form.protocol === 'shadowsocks') {
    form.listen = '::'
    form.port = 8388
    if (!modal.isEdit) form.tag = 'ss-8388'
  } else if (form.protocol === 'vmess') {
    form.listen = vmessSpec.network === 'ws' ? '127.0.0.1' : '::'
    form.port = 10087
    if (!modal.isEdit) form.tag = 'vmess-10087'
  } else if (form.protocol === 'tuic') {
    form.listen = '::'
    form.port = 8444
    if (!modal.isEdit) form.tag = 'tuic-8444'
  } else if (form.protocol === 'socks') {
    form.listen = '127.0.0.1'
    form.port = 1080
    if (!modal.isEdit) form.tag = 'socks-in-1080'
  } else if (form.protocol === 'mixed') {
    form.listen = '127.0.0.1'
    form.port = 2080
    if (!modal.isEdit) form.tag = 'mixed-in-2080'
  }
}

async function submitInbound() {
  if (saving.value) return
  saving.value = true
  let specObj: any = {}
  if (form.protocol === 'vless-reality') {
    if (!realitySpec.server_name) {
      realitySpec.server_name = 'dl.google.com'
    }
    if (!realitySpec.handshake_server) {
      realitySpec.handshake_server = realitySpec.server_name
    }
    // 客户端公钥选填：若留空且私钥存在，尝试前端预先推导一次
    if (!realitySpec.public_key?.trim() && realitySpec.private_key?.trim()) {
      try {
        const res = await api.post('/inbounds/derive-reality-key', { private_key: realitySpec.private_key.trim() })
        if (res.data?.public_key) {
          realitySpec.public_key = res.data.public_key
        }
      } catch {
        // 允许继续，后端落盘时亦会兜底推导
      }
    }
    specObj = realitySpec
  } else if (form.protocol === 'vless-ws') {
    specObj = {
      path: wsSpec.path || '/ws',
      group_domain: wsSpec.group_domain || '',
      host: wsSpec.group_domain || '',
    }
  } else if (form.protocol === 'socks' || form.protocol === 'mixed') {
    specObj = {
      username: socksSpec.username || '',
      password: socksSpec.password || '',
    }
  } else if (form.protocol === 'shadowsocks') {
    specObj = {
      method: ssSpec.method || '2022-blake3-aes-128-gcm',
      password: ssSpec.password || '',
    }
  } else if (form.protocol === 'trojan') {
    specObj = {
      password: trojanSpec.password || '',
      server_name: trojanSpec.server_name || '',
      certificate_path: trojanSpec.certificate_path || '',
      key_path: trojanSpec.key_path || '',
    }
  } else if (form.protocol === 'hysteria2') {
    specObj = {
      password: hy2Spec.password || '',
      server_name: hy2Spec.server_name || '',
      certificate_path: hy2Spec.certificate_path || '',
      key_path: hy2Spec.key_path || '',
      up_mbps: hy2Spec.up_mbps || 0,
      down_mbps: hy2Spec.down_mbps || 0,
      obfs_password: hy2Spec.obfs_password || '',
      ignore_client_bandwidth: hy2Spec.ignore_client_bandwidth,
    }
  } else if (form.protocol === 'vmess') {
    specObj = {
      network: vmessSpec.network || 'ws',
      path: vmessSpec.path || '/vmess',
      group_domain: vmessSpec.group_domain || '',
    }
  } else if (form.protocol === 'tuic') {
    specObj = {
      password: tuicSpec.password || '',
      congestion_control: tuicSpec.congestion_control || 'bbr',
      server_name: tuicSpec.server_name || '',
      certificate_path: tuicSpec.certificate_path || '',
      key_path: tuicSpec.key_path || '',
    }
  }

  try {
    const payload = {
      tag: form.tag,
      protocol: form.protocol,
      listen: form.listen || (['vless-ws', 'socks', 'mixed', 'http'].includes(form.protocol) ? '127.0.0.1' : '::'),
      port: form.port,
      spec: specObj,
      enabled: form.enabled,
    }

    if (modal.isEdit) {
      await api.put(`/inbounds/${modal.id}`, payload)
      toast.success('入站规则已更新并重载！')
    } else {
      await api.post('/inbounds', payload)
      toast.success('入站节点已创建并重载！')
    }
    modal.show = false
    await loadInbounds()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  } finally {
    saving.value = false
  }
}

async function deleteInbound(id: number) {
  if (deletingId.value !== null) return
  const confirmed = await showConfirm({
    title: '确认删除入站节点',
    message: '确定要删除该入站节点吗？删除后 sing-box 配置将自动重写并热重载生效。',
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  deletingId.value = id
  try {
    await api.delete(`/inbounds/${id}`)
    toast.success('节点已成功删除！')
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  } finally {
    deletingId.value = null
    await loadInbounds()
  }
}

async function showShareModal(ib: any) {
  try {
    const sysRes = await api.get('/system/overview')
    const ip = sysRes.data.public_ip || '127.0.0.1'
    const spec = parseSpec(ib.spec_json)

    // 计算当前受管节点定制前缀 (全协议统一生效)
    const prefix = (activeNode.value?.sub_prefix?.trim()) || (activeNode.value?.name?.trim()) || '节点'
    const remark = (ib.protocol === 'vless-ws' && spec.group_domain)
      ? `${prefix} | ${ib.tag}-Argo`
      : `${prefix} | ${ib.tag}-${ib.port}`

    if (ib.protocol === 'socks' || ib.protocol === 'mixed' || ib.protocol === 'http') {
      const auth = (spec.username && spec.password)
        ? `${encodeURIComponent(spec.username)}:${encodeURIComponent(spec.password)}@`
        : ''
      const host = (ib.listen === '::' || ib.listen === '0.0.0.0') ? ip : (ib.listen || '127.0.0.1')
      const scheme = ib.protocol === 'http' ? 'http' : 'socks5'
      const uri = `${scheme}://${auth}${host}:${ib.port}#${encodeURIComponent(remark)}`
      shareModal.title = `${remark} (${scheme.toUpperCase()})`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    if (ib.protocol === 'shadowsocks') {
      const method = spec.method || '2022-blake3-aes-128-gcm'
      const pwd = spec.password || ''
      const userPart = btoa(`${method}:${pwd}`)
      const host = (ib.listen === '::' || ib.listen === '0.0.0.0') ? ip : (ib.listen || '127.0.0.1')
      const uri = `ss://${userPart}@${host}:${ib.port}#${encodeURIComponent(remark)}`
      shareModal.title = `${remark} (Shadowsocks)`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    // 需要成员多租户 UUID 的协议
    const membersRes = await api.get('/members')
    const members = membersRes.data || []
    const m = members.length > 0 ? members[0] : { name: 'default', uuid: 'c3a6027a-7d96-4018-a5d9-34c2e0fbee37' }

    if (ib.protocol === 'hysteria2') {
      const pwd = spec.password || m.uuid
      const sni = spec.server_name || ip
      let obfsPart = ''
      if (spec.obfs_password) {
        obfsPart = `&obfs=salamander&obfs-password=${encodeURIComponent(spec.obfs_password)}`
      }
      const uri = `hysteria2://${encodeURIComponent(pwd)}@${ip}:${ib.port}?sni=${encodeURIComponent(sni)}&insecure=1${obfsPart}#${encodeURIComponent(remark)}`
      shareModal.title = `${remark} (Hysteria 2)`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    if (ib.protocol === 'trojan') {
      const pwd = spec.password || m.uuid
      const sni = spec.server_name || ip
      const uri = `trojan://${encodeURIComponent(pwd)}@${ip}:${ib.port}?sni=${encodeURIComponent(sni)}&security=tls#${encodeURIComponent(remark)}`
      shareModal.title = `${remark} (Trojan)`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    if (ib.protocol === 'tuic') {
      const pwd = spec.password || 'tuic-password'
      const cc = spec.congestion_control || 'bbr'
      const sni = spec.server_name || ip
      const uri = `tuic://${m.uuid}:${encodeURIComponent(pwd)}@${ip}:${ib.port}?congestion_control=${cc}&sni=${encodeURIComponent(sni)}&allow_insecure=1#${encodeURIComponent(remark)}`
      shareModal.title = `${remark} (TUIC v5)`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    if (ib.protocol === 'vmess') {
      const net = spec.network || 'ws'
      const path = spec.path || '/vmess'
      const host = spec.group_domain || ''
      const isDomain = net === 'ws' && !!host
      const vmessObj = {
        v: '2',
        ps: remark,
        add: isDomain ? host : ip,
        port: isDomain ? 443 : ib.port,
        id: m.uuid,
        aid: 0,
        scy: 'auto',
        net: net,
        type: 'none',
        host: host,
        path: net === 'ws' ? path : '',
        tls: isDomain ? 'tls' : '',
        sni: isDomain ? host : '',
      }
      const uri = 'vmess://' + btoa(JSON.stringify(vmessObj))
      shareModal.title = `${remark} (VMess)`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    if (ib.protocol === 'vless-ws') {
      const isDomain = !!(spec.group_domain || spec.host)
      const host = spec.group_domain || spec.host || ip
      const port = isDomain ? 443 : ib.port
      const path = spec.path || '/ws'
      const q = new URLSearchParams({
        encryption: 'none',
        security: isDomain ? 'tls' : 'none',
        sni: host,
        type: 'ws',
        host: host,
        path: path,
      })
      const uri = `vless://${m.uuid}@${host}:${port}?${q.toString()}#${encodeURIComponent(remark)}`
      shareModal.title = `${remark} (VLESS-WS 默认用户: ${m.name})`
      shareModal.uri = uri
      shareModal.show = true
      return
    }

    let pbk = spec.public_key || ''
    if (!pbk && spec.private_key) {
      try {
        const res = await api.post('/inbounds/derive-reality-key', { private_key: spec.private_key })
        if (res.data?.public_key) {
          pbk = res.data.public_key
        }
      } catch {
        // 忽略推导错误
      }
    }

    const q = new URLSearchParams({
      encryption: 'none',
      flow: spec.flow || 'xtls-rprx-vision',
      security: 'reality',
      sni: spec.server_name || 'dl.google.com',
      fp: spec.fingerprint || 'chrome',
      pbk: pbk,
      sid: spec.short_id || '',
      type: 'tcp',
    })

    const uri = `vless://${m.uuid}@${ip}:${ib.port}?${q.toString()}#${encodeURIComponent(remark)}`
    shareModal.title = `${remark} (默认用户: ${m.name})`
    shareModal.uri = uri
    shareModal.show = true
  } catch (err: any) {
    toast.error('获取节点信息失败: ' + (err.response?.data?.error || err.message))
  }
}

function copyShareUri() {
  copyToClipboard(shareModal.uri, '节点链接')
}

onMounted(() => {
  loadInbounds()
  window.addEventListener('node-context-changed', loadInbounds)
})

onUnmounted(() => {
  window.removeEventListener('node-context-changed', loadInbounds)
})
</script>
