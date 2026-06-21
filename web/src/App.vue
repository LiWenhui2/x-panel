<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  IconAlertCircle, IconCheck, IconCode, IconCopy, IconDownload, IconEye,
  IconLock, IconLogout, IconPlus, IconRefresh, IconRocket, IconServer,
  IconSettings, IconShieldCheck, IconX,
} from '@tabler/icons-vue'
import { api, type CreateInbound, type Inbound } from './api'

type FormState = CreateInbound & { totalGB: number; expiryLocal: string }

const items = ref<Inbound[]>([])
const error = ref('')
const message = ref('')
const username = ref('')
const authenticated = ref(false)
const needsSetup = ref(false)
const loading = ref(false)
const modalOpen = ref(false)
const previewOpen = ref(false)
const shareOpen = ref(false)
const preview = ref('')
const previewHash = ref('')
const shareLink = ref('')
const shareRemark = ref('')
const authForm = reactive({ username: 'admin', password: '' })
const gib = 1024 ** 3
const form = reactive<FormState>(newForm())

const totalQuota = computed(() => items.value.reduce((sum, item) => sum + item.totalBytes, 0))
const enabledCount = computed(() => items.value.filter(item => item.enabled).length)

function newForm(): FormState {
  return {
    remark: '', listen: window.location.hostname || '0.0.0.0', port: randomPort(), protocol: 'vless',
    network: 'tcp', security: 'none', clientId: makeUUID(),
    email: `client-${Date.now()}@xpanel.local`, enabled: true, totalBytes: 0,
    expiryTime: '', alterId: 0, sniffing: true, wsPath: '/xpanel',
    tlsCertFile: '', tlsKeyFile: '', totalGB: 0, expiryLocal: '',
  }
}

function randomPort() { return Math.floor(Math.random() * 40000) + 20000 }
function resetForm() { Object.assign(form, newForm()) }
function openCreate() { resetForm(); error.value = ''; modalOpen.value = true }
function generateUUID() { form.clientId = makeUUID() }

function makeUUID() {
  const randomUUID = globalThis.crypto?.randomUUID?.bind(globalThis.crypto)
  if (randomUUID) return randomUUID()
  const bytes = new Uint8Array(16)
  if (globalThis.crypto?.getRandomValues) globalThis.crypto.getRandomValues(bytes)
  else for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256)
  bytes[6] = (bytes[6]! & 0x0f) | 0x40
  bytes[8] = (bytes[8]! & 0x3f) | 0x80
  const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

async function loadStatus() {
  try {
    const status = await api.authStatus()
    authenticated.value = status.authenticated
    needsSetup.value = status.needsSetup
    username.value = status.username || ''
    if (authenticated.value) await refresh()
  } catch (cause) {
    error.value = errorText(cause)
  }
}

async function submitAuth() {
  loading.value = true
  error.value = ''
  try {
    const result = needsSetup.value ? await api.setup(authForm) : await api.login(authForm)
    authenticated.value = result.authenticated
    username.value = result.username
    needsSetup.value = false
    message.value = needsSetup.value ? 'Administrator account created.' : 'Signed in successfully.'
    await refresh()
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    loading.value = false
  }
}

async function logout() {
  await api.logout()
  authenticated.value = false
  items.value = []
}

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
    message.value = `Inbound ${created.remark} created. Click Apply Config to restart Xray.`
    await refresh()
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
    message.value = `Configuration applied: ${result.configPath}`
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
  if (showToast) message.value = 'Import link copied.'
}

function buildShareLink(item: Inbound) {
  const address = exportAddress(item.listen)
  const name = encodeURIComponent(item.remark || item.tag || `${item.protocol}-${item.port}`)
  return item.protocol === 'vmess' ? buildVMessLink(item, address) : buildVLESSLink(item, address, name)
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
    v: '2', ps: item.remark || item.tag, add: address, port: item.port,
    id: item.clientId, aid: item.alterId, net: item.network, type: 'none',
    host: '', path: item.network === 'ws' ? item.wsPath : '', tls: item.security === 'tls' ? 'tls' : 'none',
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
    try { await navigator.clipboard.writeText(value); return } catch { /* fallback below */ }
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
  if (!value) return 'Unlimited'
  if (value >= gib) return `${(value / gib).toFixed(1)} GB`
  return `${(value / 1024 ** 2).toFixed(1)} MB`
}
function formatExpiry(value: string) { return value ? new Date(value).toLocaleString() : 'Never' }
function errorText(cause: unknown) { return cause instanceof Error ? cause.message : String(cause) }

onMounted(loadStatus)
</script>

<template>
  <div v-if="!authenticated" class="auth-page">
    <section class="auth-card">
      <div class="logo-orb"><IconShieldCheck/></div>
      <p class="eyebrow">{{ needsSetup ? 'FIRST RUN SETUP' : 'SECURE SIGN IN' }}</p>
      <h1>{{ needsSetup ? 'Create administrator' : 'Welcome back' }}</h1>
      <p class="auth-copy">
        {{ needsSetup ? 'Initialize your local XPanel administrator before managing nodes.' : 'Sign in to manage Xray inbound nodes and service configuration.' }}
      </p>
      <form @submit.prevent="submitAuth">
        <label><span>Username</span><input v-model.trim="authForm.username" autocomplete="username" required /></label>
        <label><span>Password</span><input v-model="authForm.password" type="password" :autocomplete="needsSetup ? 'new-password' : 'current-password'" required /></label>
        <button class="primary wide" :disabled="loading"><IconLock/>{{ needsSetup ? 'Create account' : 'Sign in' }}</button>
      </form>
      <div v-if="error" class="inline-error"><IconAlertCircle/>{{ error }}</div>
    </section>
  </div>

  <div v-else class="app-shell">
    <aside class="sidebar">
      <div class="brand"><IconRocket/><div><strong>XPanel</strong><small>XRAY OPERATIONS</small></div></div>
      <nav>
        <a class="active" href="#"><IconServer/><span>Inbounds</span><b>{{ items.length }}</b></a>
        <a href="#"><IconSettings/><span>Settings</span></a>
      </nav>
      <button class="logout" @click="logout"><IconLogout/>Sign out</button>
    </aside>

    <main class="content">
      <header class="page-header">
        <div><p>Signed in as {{ username }}</p><h1>Node Console</h1></div>
        <div class="health"><i></i><span>Panel online</span></div>
      </header>

      <div v-if="error" class="toast error"><IconAlertCircle/>{{ error }}<button @click="error=''">×</button></div>
      <div v-if="message" class="toast success"><IconCheck/>{{ message }}<button @click="message=''">×</button></div>

      <section class="summary-grid">
        <article><span>Total inbounds</span><strong>{{ items.length }}</strong><small>Configured listeners</small></article>
        <article><span>Enabled</span><strong>{{ enabledCount }}</strong><small>Active records</small></article>
        <article><span>Traffic quota</span><strong>{{ formatBytes(totalQuota) }}</strong><small>0 means unlimited</small></article>
      </section>

      <section class="table-panel">
        <div class="table-toolbar">
          <div><h2>Inbound nodes</h2><p>Create, apply and export client import links.</p></div>
          <div class="toolbar-actions">
            <button class="ghost" :disabled="loading" @click="refresh"><IconRefresh/>Refresh</button>
            <button class="ghost" :disabled="loading || !items.length" @click="showPreview"><IconCode/>Preview</button>
            <button class="ghost" :disabled="loading || !items.length" @click="applyConfig"><IconCheck/>Apply Config</button>
            <button class="primary" @click="openCreate"><IconPlus/>Add Inbound</button>
          </div>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Status</th><th>Remark</th><th>Protocol</th><th>Listen</th><th>Port</th><th>Transport</th><th>Quota</th><th>Expires</th><th>Export</th><th>Config</th></tr></thead>
            <tbody>
              <tr v-for="item in items" :key="item.id">
                <td><span :class="['state-dot', { off: !item.enabled }]"></span>{{ item.enabled ? 'Enabled' : 'Disabled' }}</td>
                <td><strong>{{ item.remark }}</strong><small>{{ item.tag }}</small></td>
                <td><span class="protocol">{{ item.protocol }}</span></td>
                <td><code>{{ item.listen }}</code></td>
                <td><code>{{ item.port }}</code></td>
                <td><span class="transport">{{ item.network }}</span><em v-if="item.security==='tls'">TLS</em></td>
                <td>{{ formatBytes(item.totalBytes) }}</td>
                <td>{{ formatExpiry(item.expiryTime) }}</td>
                <td><button class="icon-button" title="Export import link" @click="exportInbound(item)"><IconDownload/></button></td>
                <td><button class="icon-button" title="Preview generated config" @click="showPreview"><IconEye/></button></td>
              </tr>
              <tr v-if="!items.length"><td colspan="10" class="empty-state"><IconServer/><strong>No inbound nodes yet</strong><span>Create the first inbound and apply configuration.</span></td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>

    <div v-if="modalOpen" class="modal-backdrop" @mousedown.self="modalOpen=false">
      <section class="modal" role="dialog" aria-modal="true">
        <header><div><p>NEW INBOUND</p><h2>Add inbound</h2></div><button class="close" @click="modalOpen=false"><IconX/></button></header>
        <form @submit.prevent="createInbound">
          <div class="form-grid">
            <label class="wide"><span>Remark</span><input v-model.trim="form.remark" placeholder="VPS VLESS TCP" required autofocus /></label>
            <label><span>Enabled</span><button type="button" :class="['switch', { on: form.enabled }]" @click="form.enabled=!form.enabled"><i></i>{{ form.enabled ? 'Enabled' : 'Disabled' }}</button></label>
            <label><span>Protocol</span><select v-model="form.protocol"><option value="vless">VLESS</option><option value="vmess">VMess</option></select></label>
            <label><span>Listen IP</span><input v-model="form.listen" required /></label>
            <label><span>Port</span><div class="input-action"><input v-model.number="form.port" type="number" min="1" max="65535" required /><button type="button" @click="form.port=randomPort()">Random</button></div></label>
            <label><span>Total traffic (GB)</span><input v-model.number="form.totalGB" type="number" min="0" step="0.1" /></label>
            <label class="wide"><span>Expiry</span><input v-model="form.expiryLocal" type="datetime-local" /></label>
            <label class="wide"><span>Client UUID</span><div class="input-action"><input v-model="form.clientId" required /><button type="button" @click="generateUUID">Generate</button></div></label>
            <label class="wide"><span>Client email</span><input v-model="form.email" type="email" required /></label>
            <label v-if="form.protocol==='vmess'"><span>Alter ID</span><input v-model.number="form.alterId" type="number" min="0" max="65535" /></label>
            <label><span>Transport</span><select v-model="form.network"><option value="tcp">TCP</option><option value="ws">WebSocket</option></select></label>
            <label v-if="form.network==='ws'" class="wide"><span>WebSocket path</span><input v-model="form.wsPath" placeholder="/xpanel" required /></label>
            <label><span>TLS</span><button type="button" :class="['switch', { on: form.security==='tls' }]" @click="form.security=form.security==='tls'?'none':'tls'"><i></i>{{ form.security==='tls' ? 'Enabled' : 'Disabled' }}</button></label>
            <label><span>Sniffing</span><button type="button" :class="['switch', { on: form.sniffing }]" @click="form.sniffing=!form.sniffing"><i></i>{{ form.sniffing ? 'Enabled' : 'Disabled' }}</button></label>
            <template v-if="form.security==='tls'">
              <label class="wide"><span>Certificate file</span><input v-model="form.tlsCertFile" placeholder="/etc/xpanel/certs/fullchain.pem" required /></label>
              <label class="wide"><span>Key file</span><input v-model="form.tlsKeyFile" placeholder="/etc/xpanel/certs/privkey.pem" required /></label>
            </template>
          </div>
          <footer><button type="button" class="cancel" @click="modalOpen=false">Cancel</button><button class="primary submit" :disabled="loading">{{ loading ? 'Saving…' : 'Create inbound' }}</button></footer>
        </form>
      </section>
    </div>

    <div v-if="previewOpen" class="modal-backdrop" @mousedown.self="previewOpen=false">
      <section class="modal preview-modal" role="dialog" aria-modal="true">
        <header><div><p>XRAY CONFIGURATION</p><h2>Generated config</h2></div><button class="close" @click="previewOpen=false"><IconX/></button></header>
        <div class="hash">SHA-256 <code>{{ previewHash }}</code></div>
        <pre>{{ preview }}</pre>
      </section>
    </div>

    <div v-if="shareOpen" class="modal-backdrop" @mousedown.self="shareOpen=false">
      <section class="modal share-modal" role="dialog" aria-modal="true">
        <header><div><p>CLIENT IMPORT LINK</p><h2>Export {{ shareRemark }}</h2></div><button class="close" @click="shareOpen=false"><IconX/></button></header>
        <div class="share-body">
          <p>Paste this link into a compatible client.</p>
          <textarea readonly :value="shareLink" @focus="selectShareText"></textarea>
          <div class="share-actions">
            <button class="ghost" @click="() => copyShareLink()"><IconCopy/>Copy link</button>
            <button class="primary" @click="shareOpen=false">Done</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
