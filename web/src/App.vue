<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  IconAlertCircle, IconBrandGithub, IconCheck, IconCode, IconCopy, IconDownload, IconEye, IconGauge,
  IconLink, IconLogout, IconPlus, IconRefresh, IconServer, IconSettings,
  IconUsers, IconX,
} from '@tabler/icons-vue'
import { api, type CreateInbound, type Inbound } from './api'

type FormState = CreateInbound & { totalGB: number; expiryLocal: string }

const items = ref<Inbound[]>([])
const error = ref('')
const message = ref('')
const preview = ref('')
const previewHash = ref('')
const shareLink = ref('')
const shareRemark = ref('')
const loading = ref(false)
const modalOpen = ref(false)
const previewOpen = ref(false)
const shareOpen = ref(false)

const gib = 1024 ** 3
const form = reactive<FormState>(newForm())

const totalQuota = computed(() => items.value.reduce((sum, item) => sum + item.totalBytes, 0))

function newForm(): FormState {
  return {
    remark: '', listen: '0.0.0.0', port: randomPort(), protocol: 'vless',
    network: 'tcp', security: 'none', clientId: makeUUID(),
    email: `client-${Date.now()}@xpanel.local`, enabled: true, totalBytes: 0,
    expiryTime: '', alterId: 0, sniffing: true, wsPath: '/xpanel',
    tlsCertFile: '', tlsKeyFile: '', totalGB: 0, expiryLocal: '',
  }
}

function randomPort() { return Math.floor(Math.random() * 40000) + 20000 }
function generateUUID() { form.clientId = makeUUID() }
function makeUUID() {
  const randomUUID = globalThis.crypto?.randomUUID?.bind(globalThis.crypto)
  if (randomUUID) return randomUUID()
  const bytes = new Uint8Array(16)
  if (globalThis.crypto?.getRandomValues) {
    globalThis.crypto.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256)
  }
  bytes[6] = (bytes[6]! & 0x0f) | 0x40
  bytes[8] = (bytes[8]! & 0x3f) | 0x80
  const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
function resetForm() { Object.assign(form, newForm()) }
function openCreate() { resetForm(); error.value = ''; modalOpen.value = true }

async function refresh() {
  error.value = ''
  try { items.value = (await api.list()).items }
  catch (cause) { error.value = errorText(cause) }
}

async function createInbound() {
  loading.value = true
  error.value = ''
  try {
    const payload: CreateInbound = {
      remark: form.remark, listen: form.listen, port: form.port,
      protocol: form.protocol, network: form.network, security: form.security,
      clientId: form.clientId, email: form.email, enabled: form.enabled,
      totalBytes: Math.round(form.totalGB * gib),
      expiryTime: form.expiryLocal ? new Date(form.expiryLocal).toISOString() : '',
      alterId: form.protocol === 'vmess' ? form.alterId : 0,
      sniffing: form.sniffing, wsPath: form.network === 'ws' ? form.wsPath : '/xpanel',
      tlsCertFile: form.security === 'tls' ? form.tlsCertFile : '',
      tlsKeyFile: form.security === 'tls' ? form.tlsKeyFile : '',
    }
    const created = await api.create(payload)
    modalOpen.value = false
    message.value = `入站 ${created.remark} 已创建，并已写入配置数据库。`
    await refresh()
    window.setTimeout(() => { message.value = '' }, 3500)
  } catch (cause) { error.value = errorText(cause) }
  finally { loading.value = false }
}

async function showPreview() {
  loading.value = true
  error.value = ''
  try {
    const result = await api.preview()
    previewHash.value = result.sha256
    preview.value = JSON.stringify(result.config, null, 2)
    previewOpen.value = true
  } catch (cause) { error.value = errorText(cause) }
  finally { loading.value = false }
}

async function applyConfig() {
  loading.value = true
  error.value = ''
  try {
    const result = await api.apply()
    message.value = `配置已应用，Xray 已重启：${result.configPath}`
    window.setTimeout(() => { message.value = '' }, 4500)
  } catch (cause) { error.value = errorText(cause) }
  finally { loading.value = false }
}

function exportInbound(item: Inbound) {
  shareRemark.value = item.remark || item.tag
  shareLink.value = buildShareLink(item)
  shareOpen.value = true
  void copyShareLink(false)
}

async function copyShareLink(showToast = true) {
  if (!shareLink.value) return
  await copyText(shareLink.value)
  if (showToast) {
    message.value = '导出链接已复制到剪贴板'
    window.setTimeout(() => { message.value = '' }, 2200)
  }
}

function buildShareLink(item: Inbound) {
  const address = exportAddress(item.listen)
  const name = encodeURIComponent(item.remark || item.tag || `${item.protocol}-${item.port}`)
  return item.protocol === 'vmess'
    ? buildVMessLink(item, address)
    : buildVLESSLink(item, address, name)
}

function exportAddress(listen: string) {
  const normalized = listen.trim()
  if (normalized && normalized !== '0.0.0.0' && normalized !== '::' && normalized !== '127.0.0.1') return normalized
  return window.location.hostname || '127.0.0.1'
}

function buildVLESSLink(item: Inbound, address: string, name: string) {
  const params = new URLSearchParams()
  params.set('type', item.network)
  params.set('security', item.security)
  if (item.wsPath) params.set('path', item.wsPath)
  return `vless://${item.clientId}@${address}:${item.port}?${params.toString()}#${name}`
}

function buildVMessLink(item: Inbound, address: string) {
  const payload = {
    v: '2',
    ps: item.remark || item.tag,
    add: address,
    port: item.port,
    id: item.clientId,
    aid: item.alterId,
    net: item.network,
    type: 'none',
    host: '',
    path: item.network === 'ws' ? item.wsPath : '',
    tls: item.security === 'tls' ? 'tls' : 'none',
  }
  return `vmess://${base64Encode(JSON.stringify(payload, null, 2))}`
}

function base64Encode(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary)
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return
    } catch {
      // Browser may block Clipboard API on plain HTTP/private IP origins.
    }
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', 'readonly')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

function selectShareText(event: FocusEvent) {
  if (event.target instanceof HTMLTextAreaElement) event.target.select()
}

function formatBytes(value: number) {
  if (!value) return '不限'
  if (value >= gib) return `${(value / gib).toFixed(1)} GB`
  return `${(value / 1024 ** 2).toFixed(1)} MB`
}
function formatExpiry(value: string) { return value ? new Date(value).toLocaleString('zh-CN') : '无限期' }
function errorText(cause: unknown) { return cause instanceof Error ? cause.message : String(cause) }

onMounted(refresh)
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand"><IconServer :size="25"/><div><strong>XPanel</strong><small>XRAY CONTROL</small></div></div>
      <nav>
        <a href="#"><IconGauge/><span>系统状态</span></a>
        <a class="active" href="#"><IconUsers/><span>入站列表</span><b>{{ items.length }}</b></a>
        <a href="#"><IconSettings/><span>面板设置</span></a>
        <a href="#"><IconLink/><span>其他</span></a>
      </nav>
      <div class="sidebar-bottom">
        <a href="https://github.com" target="_blank"><IconBrandGithub/><span>GitHub</span></a>
        <a href="#"><IconLogout/><span>退出登录</span></a>
      </div>
    </aside>

    <main class="content">
      <header class="page-header">
        <div><p>节点管理</p><h1>入站列表</h1></div>
        <div class="health"><i></i><span>服务运行中</span></div>
      </header>

      <div v-if="error" class="toast error"><IconAlertCircle/>{{ error }}<button @click="error=''">×</button></div>
      <div v-if="message" class="toast success"><IconCheck/>{{ message }}</div>

      <section class="summary-grid">
        <article><span>入站数量</span><strong>{{ items.length }}</strong><small>已配置监听端口</small></article>
        <article><span>已启用</span><strong>{{ items.filter(i => i.enabled).length }}</strong><small>当前生效节点</small></article>
        <article><span>总流量额度</span><strong>{{ formatBytes(totalQuota) }}</strong><small>0 表示不限制</small></article>
      </section>

      <section class="table-panel">
        <div class="table-toolbar">
          <div><h2>入站节点</h2><p>管理协议、监听端口和客户端身份</p></div>
          <div class="toolbar-actions">
            <button class="ghost" :disabled="loading" @click="refresh"><IconRefresh/>刷新</button>
            <button class="ghost" :disabled="loading || !items.length" @click="showPreview"><IconCode/>配置预览</button>
            <button class="ghost" :disabled="loading || !items.length" @click="applyConfig"><IconCheck/>应用配置</button>
            <button class="primary" @click="openCreate"><IconPlus/>添加入站</button>
          </div>
        </div>

        <div class="table-wrap">
          <table>
            <thead><tr><th>状态</th><th>备注</th><th>协议</th><th>监听地址</th><th>端口</th><th>传输</th><th>流量限制</th><th>到期时间</th><th>导出</th><th>详情</th></tr></thead>
            <tbody>
              <tr v-for="item in items" :key="item.id">
                <td><span :class="['state-dot', { off: !item.enabled }]"></span>{{ item.enabled ? '启用' : '停用' }}</td>
                <td><strong>{{ item.remark }}</strong><small>{{ item.tag }}</small></td>
                <td><span class="protocol">{{ item.protocol }}</span></td>
                <td><code>{{ item.listen }}</code></td>
                <td><code>{{ item.port }}</code></td>
                <td><span class="transport">{{ item.network }}</span><em v-if="item.security==='tls'">TLS</em></td>
                <td>{{ formatBytes(item.totalBytes) }}</td>
                <td>{{ formatExpiry(item.expiryTime) }}</td>
                <td><button class="icon-button" title="导出分享链接" @click="exportInbound(item)"><IconDownload/></button></td>
                <td><button class="icon-button" title="查看生成配置" @click="showPreview"><IconEye/></button></td>
              </tr>
              <tr v-if="!items.length"><td colspan="10" class="empty-state"><IconServer/><strong>还没有入站节点</strong><span>点击“添加入站”创建第一个节点</span></td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>

    <div v-if="modalOpen" class="modal-backdrop" @mousedown.self="modalOpen=false">
      <section class="modal" role="dialog" aria-modal="true" aria-labelledby="create-title">
        <header><div><p>NEW INBOUND</p><h2 id="create-title">添加入站</h2></div><button class="close" aria-label="关闭" @click="modalOpen=false"><IconX/></button></header>
        <form @submit.prevent="createInbound">
          <div class="form-grid">
            <label class="wide"><span>备注名称</span><input v-model.trim="form.remark" placeholder="例如：家庭网络 VLESS" required autofocus /></label>
            <label><span>启用状态</span><button type="button" :class="['switch', { on: form.enabled }]" @click="form.enabled=!form.enabled"><i></i>{{ form.enabled ? '已启用' : '已停用' }}</button></label>
            <label><span>协议</span><select v-model="form.protocol"><option value="vless">VLESS</option><option value="vmess">VMess</option></select></label>
            <label><span>监听 IP</span><input v-model="form.listen" required /></label>
            <label><span>端口</span><div class="input-action"><input v-model.number="form.port" type="number" min="1" max="65535" required /><button type="button" @click="form.port=randomPort()">随机</button></div></label>
            <label><span>总流量 (GB)</span><input v-model.number="form.totalGB" type="number" min="0" step="0.1" /></label>
            <label class="wide"><span>到期时间</span><input v-model="form.expiryLocal" type="datetime-local" /></label>
            <label class="wide"><span>客户端 UUID</span><div class="input-action"><input v-model="form.clientId" required /><button type="button" @click="generateUUID">生成</button></div></label>
            <label class="wide"><span>客户端标识</span><input v-model="form.email" type="email" required /></label>
            <label v-if="form.protocol==='vmess'"><span>Alter ID</span><input v-model.number="form.alterId" type="number" min="0" max="65535" /></label>
            <label><span>传输方式</span><select v-model="form.network"><option value="tcp">TCP</option><option value="ws">WebSocket</option></select></label>
            <label v-if="form.network==='ws'" class="wide"><span>WebSocket 路径</span><input v-model="form.wsPath" placeholder="/xpanel" required /></label>
            <label><span>TLS</span><button type="button" :class="['switch', { on: form.security==='tls' }]" @click="form.security=form.security==='tls'?'none':'tls'"><i></i>{{ form.security==='tls' ? '已开启' : '未开启' }}</button></label>
            <label><span>流量探测</span><button type="button" :class="['switch', { on: form.sniffing }]" @click="form.sniffing=!form.sniffing"><i></i>{{ form.sniffing ? '已开启' : '未开启' }}</button></label>
            <template v-if="form.security==='tls'">
              <label class="wide"><span>证书文件（VM 绝对路径）</span><input v-model="form.tlsCertFile" placeholder="/etc/xpanel/certs/fullchain.pem" required /></label>
              <label class="wide"><span>私钥文件（VM 绝对路径）</span><input v-model="form.tlsKeyFile" placeholder="/etc/xpanel/certs/privkey.pem" required /></label>
            </template>
          </div>
          <p v-if="error" class="form-error"><IconAlertCircle/>{{ error }}</p>
          <footer><button type="button" class="cancel" @click="modalOpen=false">取消</button><button class="primary submit" :disabled="loading">{{ loading ? '正在保存…' : '创建入站' }}</button></footer>
        </form>
      </section>
    </div>

    <div v-if="previewOpen" class="modal-backdrop" @mousedown.self="previewOpen=false">
      <section class="modal preview-modal" role="dialog" aria-modal="true">
        <header><div><p>XRAY CONFIGURATION</p><h2>真实配置预览</h2></div><button class="close" @click="previewOpen=false"><IconX/></button></header>
        <div class="hash">SHA-256 <code>{{ previewHash }}</code></div>
        <pre>{{ preview }}</pre>
      </section>
    </div>

    <div v-if="shareOpen" class="modal-backdrop" @mousedown.self="shareOpen=false">
      <section class="modal share-modal" role="dialog" aria-modal="true">
        <header><div><p>CLIENT IMPORT LINK</p><h2>导出节点：{{ shareRemark }}</h2></div><button class="close" @click="shareOpen=false"><IconX/></button></header>
        <div class="share-body">
          <p>复制下面的链接，可直接粘贴到 v2rayN、v2rayNG、Shadowrocket 等客户端导入。</p>
          <textarea readonly :value="shareLink" @focus="selectShareText"></textarea>
          <div class="share-actions">
            <button class="ghost" @click="() => copyShareLink()"><IconCopy/>复制链接</button>
            <button class="primary" @click="shareOpen=false">完成</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
