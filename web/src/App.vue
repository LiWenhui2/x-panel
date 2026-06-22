<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  IconAlertCircle, IconCheck, IconCode, IconCopy, IconDownload, IconEye,
  IconLock, IconLogout, IconPlus, IconRefresh, IconRocket, IconServer,
  IconSettings, IconShieldCheck, IconX,
} from '@tabler/icons-vue'
import { api, type CreateInbound, type Inbound } from './api'
import { messages, type Language, type MessageKey } from './i18n'
import { buildShareLink } from './share'

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
const shareExpiry = ref('')
const shareTotal = ref(0)
const shareUsed = ref(0)
const shareRemaining = ref(0)
const authForm = reactive({ username: 'admin', password: '' })
const language = ref<Language>((localStorage.getItem('xpanel-language') as Language) === 'zh' ? 'zh' : 'en')
const gib = 1024 ** 3
const form = reactive<FormState>(newForm())
let sessionTimer: number | undefined

const totalQuota = computed(() => items.value.reduce((sum, item) => sum + item.totalBytes, 0))
const enabledCount = computed(() => items.value.filter(item => item.enabled).length)

function t(key: MessageKey, values: Record<string, string> = {}) {
  let text: string = messages[language.value][key]
  for (const [name, value] of Object.entries(values)) text = text.replace(`{${name}}`, value)
  return text
}

function setLanguage(value: Language) {
  language.value = value
  localStorage.setItem('xpanel-language', value)
  document.documentElement.lang = value === 'zh' ? 'zh-CN' : 'en'
}

function newForm(): FormState {
  return {
    remark: '', listen: window.location.hostname || '0.0.0.0', port: randomPort(), protocol: 'vless',
    network: 'tcp', security: 'none', clientId: makeUUID(),
    email: `client-${Date.now()}@xpanel.local`, enabled: true, totalBytes: 0,
    expiryTime: '', alterId: 0, sniffing: true, wsPath: '/xpanel',
    tlsCertFile: '', tlsKeyFile: '', totalGB: 0, expiryLocal: '2099-12-31T23:59',
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

async function verifySession() {
  if (!authenticated.value) return
  try {
    const status = await api.authStatus()
    if (status.authenticated) return
    authenticated.value = false
    username.value = ''
    items.value = []
    authForm.password = ''
    message.value = ''
    error.value = t('sessionExpired')
  } catch {
    // A brief service restart should not sign the user out until the server responds.
  }
}

async function submitAuth() {
  loading.value = true
  error.value = ''
  try {
    const wasSetup = needsSetup.value
    const result = wasSetup ? await api.setup(authForm) : await api.login(authForm)
    authenticated.value = result.authenticated
    username.value = result.username
    needsSetup.value = false
    message.value = wasSetup ? t('adminCreated') : t('signedIn')
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
    message.value = t('inboundCreated', { name: created.remark })
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
    message.value = t('configApplied', { path: result.configPath })
  } catch (cause) { error.value = errorText(cause) }
  finally { loading.value = false }
}

function exportInbound(item: Inbound) {
  shareRemark.value = item.remark || item.tag
  shareExpiry.value = item.expiryTime
  shareTotal.value = item.totalBytes
  shareUsed.value = item.usedBytes
  shareRemaining.value = item.remainingBytes
  shareLink.value = buildShareLink(item, exportAddress(item.listen))
  shareOpen.value = true
  void copyShareLink(false)
}

async function copyShareLink(showToast = true) {
  if (!shareLink.value) return
  await copyText(shareLink.value)
  if (showToast) message.value = t('linkCopied')
}

function exportAddress(listen: string) {
  const normalized = listen.trim()
  if (normalized && normalized !== '0.0.0.0' && normalized !== '::' && normalized !== '127.0.0.1') return normalized
  return window.location.hostname || '127.0.0.1'
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
  if (!value) return t('unlimited')
  if (value >= gib) return `${(value / gib).toFixed(1)} GB`
  return `${(value / 1024 ** 2).toFixed(1)} MB`
}
function formatBytesExact(value: number) {
  if (!value) return '0 B'
  if (value >= gib) return `${(value / gib).toFixed(1)} GB`
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}
function formatExpiry(value: string) { return value ? new Date(value).toLocaleString(language.value === 'zh' ? 'zh-CN' : 'en-US') : t('never') }
function errorText(cause: unknown) { return cause instanceof Error ? cause.message : String(cause) }

onMounted(() => {
  setLanguage(language.value)
  void loadStatus()
  sessionTimer = window.setInterval(() => void verifySession(), 3000)
  window.addEventListener('focus', verifySession)
})
onBeforeUnmount(() => {
  if (sessionTimer !== undefined) window.clearInterval(sessionTimer)
  window.removeEventListener('focus', verifySession)
})
</script>

<template>
  <div v-if="!authenticated" class="auth-page">
    <div class="language-switch auth-language" role="group" aria-label="Language">
      <button :class="{ active: language === 'en' }" @click="setLanguage('en')">{{ t('english') }}</button>
      <button :class="{ active: language === 'zh' }" @click="setLanguage('zh')">{{ t('chinese') }}</button>
    </div>
    <section class="auth-card">
      <div class="logo-orb"><IconShieldCheck/></div>
      <p class="eyebrow">{{ needsSetup ? t('firstRun') : t('signInEyebrow') }}</p>
      <h1>{{ needsSetup ? t('createAdmin') : t('welcome') }}</h1>
      <p class="auth-copy">
        {{ needsSetup ? t('setupCopy') : t('signInCopy') }}
      </p>
      <form @submit.prevent="submitAuth">
        <label><span>{{ t('username') }}</span><input v-model.trim="authForm.username" autocomplete="username" required /></label>
        <label><span>{{ t('password') }}</span><input v-model="authForm.password" type="password" :autocomplete="needsSetup ? 'new-password' : 'current-password'" required /></label>
        <button class="primary wide" :disabled="loading"><IconLock/>{{ needsSetup ? t('createAccount') : t('signIn') }}</button>
      </form>
      <div v-if="error" class="inline-error"><IconAlertCircle/>{{ error }}</div>
    </section>
  </div>

  <div v-else class="app-shell">
    <aside class="sidebar">
      <div class="brand"><IconRocket/><div><strong>XPanel</strong><small>XRAY OPERATIONS</small></div></div>
      <nav>
        <a class="active" href="#"><IconServer/><span>{{ t('inbounds') }}</span><b>{{ items.length }}</b></a>
        <a href="#"><IconSettings/><span>{{ t('settings') }}</span></a>
      </nav>
      <button class="logout" @click="logout"><IconLogout/>{{ t('signOut') }}</button>
    </aside>

    <main class="content">
      <header class="page-header">
        <div><p>{{ t('signedInAs', { name: username }) }}</p><h1>{{ t('nodeConsole') }}</h1></div>
        <div class="header-actions">
          <div class="language-switch" role="group" aria-label="Language">
            <button :class="{ active: language === 'en' }" @click="setLanguage('en')">EN</button>
            <button :class="{ active: language === 'zh' }" @click="setLanguage('zh')">中文</button>
          </div>
          <div class="health"><i></i><span>{{ t('panelOnline') }}</span></div>
        </div>
      </header>

      <div v-if="error" class="toast error"><IconAlertCircle/>{{ error }}<button @click="error=''">×</button></div>
      <div v-if="message" class="toast success"><IconCheck/>{{ message }}<button @click="message=''">×</button></div>

      <section class="summary-grid">
        <article><span>{{ t('totalInbounds') }}</span><strong>{{ items.length }}</strong><small>{{ t('configuredListeners') }}</small></article>
        <article><span>{{ t('enabled') }}</span><strong>{{ enabledCount }}</strong><small>{{ t('activeRecords') }}</small></article>
        <article><span>{{ t('trafficQuota') }}</span><strong>{{ formatBytes(totalQuota) }}</strong><small>{{ t('zeroUnlimited') }}</small></article>
      </section>

      <section class="table-panel">
        <div class="table-toolbar">
          <div><h2>{{ t('inboundNodes') }}</h2><p>{{ t('inboundHelp') }}</p></div>
          <div class="toolbar-actions">
            <button class="ghost" :disabled="loading" @click="refresh"><IconRefresh/>{{ t('refresh') }}</button>
            <button class="ghost" :disabled="loading || !items.length" @click="showPreview"><IconCode/>{{ t('preview') }}</button>
            <button class="ghost" :disabled="loading || !items.length" @click="applyConfig"><IconCheck/>{{ t('applyConfig') }}</button>
            <button class="primary" @click="openCreate"><IconPlus/>{{ t('addInbound') }}</button>
          </div>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>{{ t('status') }}</th><th>{{ t('remark') }}</th><th>{{ t('protocol') }}</th><th>{{ t('listen') }}</th><th>{{ t('port') }}</th><th>{{ t('transport') }}</th><th>{{ t('quota') }}</th><th>{{ t('expires') }}</th><th>{{ t('export') }}</th><th>{{ t('config') }}</th></tr></thead>
            <tbody>
              <tr v-for="item in items" :key="item.id">
                <td><span :class="['state-dot', { off: !item.enabled }]"></span>{{ item.enabled ? t('enabled') : t('disabled') }}</td>
                <td><strong>{{ item.remark }}</strong><small>{{ item.tag }}</small></td>
                <td><span class="protocol">{{ item.protocol }}</span></td>
                <td><code>{{ item.listen }}</code></td>
                <td><code>{{ item.port }}</code></td>
                <td><span class="transport">{{ item.network }}</span><em v-if="item.security==='tls'">TLS</em></td>
                <td>{{ formatBytes(item.totalBytes) }}</td>
                <td>{{ formatExpiry(item.expiryTime) }}</td>
                <td><button class="icon-button" :title="t('exportTitle')" @click="exportInbound(item)"><IconDownload/></button></td>
                <td><button class="icon-button" :title="t('previewTitle')" @click="showPreview"><IconEye/></button></td>
              </tr>
              <tr v-if="!items.length"><td colspan="10" class="empty-state"><IconServer/><strong>{{ t('noNodes') }}</strong><span>{{ t('noNodesHelp') }}</span></td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>

    <div v-if="modalOpen" class="modal-backdrop" @mousedown.self="modalOpen=false">
      <section class="modal" role="dialog" aria-modal="true">
        <header><div><p>{{ t('newInbound') }}</p><h2>{{ t('addInbound') }}</h2></div><button class="close" @click="modalOpen=false"><IconX/></button></header>
        <form @submit.prevent="createInbound">
          <div class="form-grid">
            <label class="wide"><span>{{ t('remark') }}</span><input v-model.trim="form.remark" placeholder="VPS VLESS TCP" required autofocus /></label>
            <label><span>{{ t('enabled') }}</span><button type="button" :class="['switch', { on: form.enabled }]" @click="form.enabled=!form.enabled"><i></i>{{ form.enabled ? t('enabled') : t('disabled') }}</button></label>
            <label><span>{{ t('protocol') }}</span><select v-model="form.protocol"><option value="vless">VLESS</option><option value="vmess">VMess</option></select></label>
            <label><span>{{ t('listenIp') }}</span><input v-model="form.listen" required /></label>
            <label><span>{{ t('port') }}</span><div class="input-action"><input v-model.number="form.port" type="number" min="1" max="65535" required /><button type="button" @click="form.port=randomPort()">{{ t('random') }}</button></div></label>
            <label><span>{{ t('totalTraffic') }}</span><input v-model.number="form.totalGB" type="number" min="0" step="0.1" /></label>
            <label class="wide"><span>{{ t('expiry') }}</span><input v-model="form.expiryLocal" type="datetime-local" /></label>
            <label class="wide"><span>{{ t('clientUuid') }}</span><div class="input-action"><input v-model="form.clientId" required /><button type="button" @click="generateUUID">{{ t('generate') }}</button></div></label>
            <label class="wide"><span>{{ t('clientEmail') }}</span><input v-model="form.email" type="email" required /></label>
            <label v-if="form.protocol==='vmess'"><span>{{ t('alterId') }}</span><input v-model.number="form.alterId" type="number" min="0" max="65535" /></label>
            <label><span>{{ t('transport') }}</span><select v-model="form.network"><option value="tcp">TCP</option><option value="ws">WebSocket</option></select></label>
            <label v-if="form.network==='ws'" class="wide"><span>{{ t('wsPath') }}</span><input v-model="form.wsPath" placeholder="/xpanel" required /></label>
            <label><span>{{ t('tls') }}</span><button type="button" :class="['switch', { on: form.security==='tls' }]" @click="form.security=form.security==='tls'?'none':'tls'"><i></i>{{ form.security==='tls' ? t('enabled') : t('disabled') }}</button></label>
            <label><span>{{ t('sniffing') }}</span><button type="button" :class="['switch', { on: form.sniffing }]" @click="form.sniffing=!form.sniffing"><i></i>{{ form.sniffing ? t('enabled') : t('disabled') }}</button></label>
            <template v-if="form.security==='tls'">
              <label class="wide"><span>{{ t('certificateFile') }}</span><input v-model="form.tlsCertFile" placeholder="/etc/xpanel/certs/fullchain.pem" required /></label>
              <label class="wide"><span>{{ t('keyFile') }}</span><input v-model="form.tlsKeyFile" placeholder="/etc/xpanel/certs/privkey.pem" required /></label>
            </template>
          </div>
          <footer><button type="button" class="cancel" @click="modalOpen=false">{{ t('cancel') }}</button><button class="primary submit" :disabled="loading">{{ loading ? t('saving') : t('createInbound') }}</button></footer>
        </form>
      </section>
    </div>

    <div v-if="previewOpen" class="modal-backdrop" @mousedown.self="previewOpen=false">
      <section class="modal preview-modal" role="dialog" aria-modal="true">
        <header><div><p>{{ t('xrayConfig') }}</p><h2>{{ t('generatedConfig') }}</h2></div><button class="close" @click="previewOpen=false"><IconX/></button></header>
        <div class="hash">SHA-256 <code>{{ previewHash }}</code></div>
        <pre>{{ preview }}</pre>
      </section>
    </div>

    <div v-if="shareOpen" class="modal-backdrop" @mousedown.self="shareOpen=false">
      <section class="modal share-modal" role="dialog" aria-modal="true">
        <header><div><p>{{ t('clientImportLink') }}</p><h2>{{ t('exportName', { name: shareRemark }) }}</h2></div><button class="close" @click="shareOpen=false"><IconX/></button></header>
        <div class="share-body">
          <p>{{ t('pasteLink') }}</p>
          <div class="share-info">
            <div><span>{{ t('expiry') }}</span><strong>{{ formatExpiry(shareExpiry) }}</strong></div>
            <div><span>{{ t('totalTraffic') }}</span><strong>{{ formatBytes(shareTotal) }}</strong></div>
            <div><span>{{ t('usedTraffic') }}</span><strong>{{ formatBytesExact(shareUsed) }}</strong></div>
            <div><span>{{ t('remainingTraffic') }}</span><strong>{{ shareTotal ? formatBytesExact(shareRemaining) : t('unlimited') }}</strong></div>
          </div>
          <textarea readonly :value="shareLink" @focus="selectShareText"></textarea>
          <div class="share-actions">
            <button class="ghost" @click="() => copyShareLink()"><IconCopy/>{{ t('copyLink') }}</button>
            <button class="primary" @click="shareOpen=false">{{ t('done') }}</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
