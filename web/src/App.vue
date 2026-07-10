<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import {
  IconAlertCircle, IconBrandGithub, IconChartDonut, IconCheck, IconChevronDown, IconCopy, IconCpu,
  IconDashboard, IconDatabase, IconDownload, IconEdit, IconExternalLink, IconKey, IconLink, IconLock,
  IconLogout, IconPlus, IconRefresh, IconRocket, IconServer, IconSettings, IconTrash, IconUpload,
  IconShieldCheck, IconUserCog, IconX,
} from '@tabler/icons-vue'
import QRCode from 'qrcode'
import {
  api,
  type CreateInbound,
  type Inbound,
  type Settings,
  type Subscription,
  type SystemStatus,
} from './api'
import { messages, type Language, type MessageKey } from './i18n'
import { buildClientExport, type ExportClientId } from './share'

type View = 'overview' | 'inbounds' | 'subscriptions' | 'settings'
type FormState = CreateInbound & { totalGB: number; expiryLocal: string }

const items = ref<Inbound[]>([])
const subscriptions = ref<Subscription[]>([])
const systemStatus = ref<SystemStatus | null>(null)
const panelSettings = ref<Settings | null>(null)
const error = ref('')
const message = ref('')
const username = ref('')
const authenticated = ref(false)
const needsSetup = ref(false)
const authReady = ref(false)
const loading = ref(false)
const loadingNodes = ref(false)
const modalOpen = ref(false)
const shareOpen = ref(false)
const githubExpanded = ref(false)
const subscriptionModalOpen = ref(false)
const editingInboundId = ref<number | null>(null)
const savedView = localStorage.getItem('xpanel-active-view') as View | null
const activeView = ref<View>(savedView && ['overview', 'inbounds', 'subscriptions', 'settings'].includes(savedView) ? savedView : 'overview')
const restartNeeded = ref(false)
const shareLink = ref('')
const shareRemark = ref('')
const shareExpiry = ref('')
const shareTotal = ref(0)
const shareUsed = ref(0)
const shareRemaining = ref(0)
const shareNode = ref<Inbound | null>(null)
const shareSource = ref<'inbound' | 'subscription'>('inbound')
const shareSubscriptionURL = ref('')
const selectedExportClient = ref<ExportClientId>('nexora')
const shareQRCode = ref<HTMLCanvasElement | null>(null)
const subscriptionURLs = reactive<Record<number, string>>({})
const subscriptionForm = reactive({ id: 0, name: '', enabled: true, inboundIds: [] as number[], totalGB: 0, expiryLocal: '2099-12-31T23:59' })
const authForm = reactive({ username: '', password: '' })
const settingsForm = reactive({ port: 0, username: '', password: '' })
const integrationForm = reactive({ allowedIps: '' })
const freshIntegrationToken = ref('')
const language = ref<Language>((localStorage.getItem('xpanel-language') as Language) === 'en' ? 'en' : 'zh')
const gib = 1024 ** 3
const exportClients: Array<{ id: ExportClientId; name: string }> = [
  { id: 'nexora', name: 'Nexora' },
  { id: 'v2rayn', name: 'v2rayN' },
  { id: 'shadowrocket', name: 'Shadowrocket' },
  { id: 'clash', name: 'Clash' },
  { id: 'sing-box', name: 'sing-box' },
]
const githubProjects = [
  { name: 'Nexora', url: 'https://github.com/LiWenhui2/Nexora' },
  { name: 'XPanel', url: 'https://github.com/LiWenhui2/x-panel' },
]
const form = reactive<FormState>(newForm())
let sessionTimer: number | undefined
let statusTimer: number | undefined
let toastTimer: number | undefined

const totalQuota = computed(() => items.value.reduce((sum, item) => sum + item.totalBytes, 0))
const enabledCount = computed(() => items.value.filter(item => item.enabled).length)
const editingInbound = computed(() => items.value.find(item => item.id === editingInboundId.value) || null)

function t(key: MessageKey, values: Record<string, string> = {}) {
  let text: string = messages[language.value][key]
  for (const [name, value] of Object.entries(values)) text = text.replace(`{${name}}`, value)
  return text
}

function subscriptionBlockText(reason: string) {
  if (reason === 'subscription_disabled') return t('subscriptionDisabledReason')
  if (reason === 'traffic_exhausted') return t('subscriptionTrafficExhaustedReason')
  if (reason === 'expired') return t('subscriptionExpiredReason')
  return t('subscriptionActiveReason')
}

function setLanguage(value: Language) {
  language.value = value
  localStorage.setItem('xpanel-language', value)
  document.documentElement.lang = value === 'zh' ? 'zh-CN' : 'en'
}

function notify(text: string) { message.value = text }
function fail(text: string) { error.value = text }

watch([message, error], () => {
  if (toastTimer) window.clearTimeout(toastTimer)
  if (!message.value && !error.value) return
  toastTimer = window.setTimeout(() => {
    message.value = ''
    error.value = ''
  }, 3000)
})

function newForm(): FormState {
  return {
    remark: '',
    listen: '0.0.0.0',
    port: randomPort(),
    protocol: 'vless',
    network: 'tcp',
    security: 'none',
    clientId: makeUUID(),
    email: `client-${Date.now()}@xpanel.local`,
    enabled: true,
    totalBytes: 0,
    expiryTime: '',
    alterId: 0,
    sniffing: true,
    wsPath: '/xpanel',
    tlsCertFile: '',
    tlsKeyFile: '',
    totalGB: 0,
    expiryLocal: '2099-12-31T23:59',
  }
}

function randomPort() { return Math.floor(Math.random() * 40000) + 20000 }
function resetForm() { Object.assign(form, newForm()); editingInboundId.value = null }
function openCreate() { resetForm(); error.value = ''; modalOpen.value = true }
function openEdit(item: Inbound) {
  editingInboundId.value = item.id
  Object.assign(form, {
    remark: item.remark,
    listen: item.listen,
    port: item.port,
    protocol: item.protocol,
    network: item.network,
    security: item.security,
    clientId: item.clientId,
    email: item.email,
    enabled: item.enabled,
    totalBytes: item.totalBytes,
    expiryTime: item.expiryTime,
    alterId: item.alterId,
    sniffing: item.sniffing,
    wsPath: item.wsPath || '/xpanel',
    tlsCertFile: item.tlsCertFile,
    tlsKeyFile: item.tlsKeyFile,
    totalGB: item.totalBytes ? Number((item.totalBytes / gib).toFixed(2)) : 0,
    expiryLocal: toLocalInput(item.expiryTime),
  })
  modalOpen.value = true
}
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
    settingsForm.username = status.username || ''
    if (authenticated.value) {
      await Promise.all([refresh(), refreshSystem(), loadSettings()])
      if (activeView.value === 'subscriptions') await refreshSubscriptions()
      startStatusPolling()
    }
  } catch (cause) {
    fail(errorText(cause))
  } finally {
    authReady.value = true
  }
}

async function setActiveView(view: View) {
  activeView.value = view
  localStorage.setItem('xpanel-active-view', view)
  if (view === 'subscriptions' && !subscriptions.value.length) await refreshSubscriptions()
  if (view === 'settings' && !panelSettings.value) await loadSettings()
}

async function verifySession() {
  if (!authenticated.value) return
  try {
    const status = await api.authStatus()
    if (status.authenticated) return
    authenticated.value = false
    stopStatusPolling()
    username.value = ''
    items.value = []
    authForm.password = ''
    fail(t('sessionExpired'))
  } catch {
    // Keep the user in place during brief restarts.
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
    settingsForm.username = result.username
    needsSetup.value = false
    notify(wasSetup ? t('adminCreated') : t('signedIn'))
    await Promise.all([refresh(), refreshSystem(), loadSettings()])
    startStatusPolling()
  } catch (cause) {
    fail(errorText(cause))
  } finally {
    loading.value = false
  }
}

async function logout() {
  await api.logout()
  authenticated.value = false
  stopStatusPolling()
  items.value = []
  systemStatus.value = null
}

async function refresh() {
  error.value = ''
  loadingNodes.value = true
  try { items.value = (await api.list()).items }
  catch (cause) { fail(errorText(cause)) }
  finally { loadingNodes.value = false }
}

async function refreshSystem() {
  try { systemStatus.value = await api.systemStatus() }
  catch (cause) { fail(errorText(cause)) }
}

async function loadSettings() {
  try {
    panelSettings.value = await api.settings()
    settingsForm.port = panelSettings.value.port || Number(window.location.port) || 8080
    integrationForm.allowedIps = panelSettings.value.integration?.allowedIps?.join('\n') || ''
  } catch (cause) {
    fail(errorText(cause))
  }
}

function startStatusPolling() {
  stopStatusPolling()
  statusTimer = window.setInterval(() => void refreshSystem(), 1000)
}
function stopStatusPolling() {
  if (statusTimer !== undefined) window.clearInterval(statusTimer)
  statusTimer = undefined
}

async function showSubscriptions() {
  await setActiveView('subscriptions')
}

async function refreshSubscriptions() {
  error.value = ''
  try { subscriptions.value = (await api.subscriptions()).items }
  catch (cause) { fail(errorText(cause)) }
}

function openSubscription(item?: Subscription) {
  subscriptionForm.id = item?.id || 0
  subscriptionForm.name = item?.name || ''
  subscriptionForm.enabled = item?.enabled ?? true
  subscriptionForm.inboundIds = [...(item?.inboundIds || [])]
  subscriptionForm.totalGB = item?.totalBytes ? Number((item.totalBytes / gib).toFixed(2)) : 0
  subscriptionForm.expiryLocal = toLocalInput(item?.expiryTime || '2099-12-31T23:59:59Z')
  subscriptionModalOpen.value = true
}

function toggleSubscriptionNode(id: number) {
  const index = subscriptionForm.inboundIds.indexOf(id)
  if (index >= 0) subscriptionForm.inboundIds.splice(index, 1)
  else subscriptionForm.inboundIds.push(id)
}

async function saveSubscription() {
  loading.value = true
  error.value = ''
  try {
    const input = {
      name: subscriptionForm.name,
      enabled: subscriptionForm.enabled,
      inboundIds: subscriptionForm.inboundIds,
      totalBytes: Math.round(subscriptionForm.totalGB * gib),
      expiryTime: subscriptionForm.expiryLocal ? new Date(subscriptionForm.expiryLocal).toISOString() : '',
    }
    if (subscriptionForm.id) {
      await api.updateSubscription(subscriptionForm.id, input)
      notify(t('subscriptionUpdated'))
    } else {
      const result = await api.createSubscription(input)
      subscriptionURLs[result.subscription.id] = result.url
      notify(t('subscriptionCreated'))
    }
    subscriptionModalOpen.value = false
    await refreshSubscriptions()
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function rotateSubscription(item: Subscription) {
  loading.value = true
  error.value = ''
  try {
    const result = await api.rotateSubscription(item.id)
    subscriptionURLs[item.id] = result.url
    await copyText(result.url)
    notify(t('subscriptionRotated'))
    await refreshSubscriptions()
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function renewSubscription(item: Subscription, days: number) {
  loading.value = true
  error.value = ''
  try {
    await api.renewSubscription(item.id, days)
    notify(t('subscriptionRenewed', { days: String(days) }))
    await Promise.all([refreshSubscriptions(), refresh()])
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function ensureSubscriptionURL(item: Subscription) {
  const value = subscriptionURLs[item.id]
  if (value) return value
  const result = await api.subscriptionURL(item.id)
  subscriptionURLs[item.id] = result.url
  return result.url
}

async function exportSubscription(item: Subscription) {
  loading.value = true
  error.value = ''
  try {
    shareSource.value = 'subscription'
    shareNode.value = null
    shareRemark.value = item.name
    shareExpiry.value = item.expiryTime
    shareTotal.value = item.totalBytes
    shareUsed.value = item.usedBytes
    shareRemaining.value = item.remainingBytes
    selectedExportClient.value = 'nexora'
    shareSubscriptionURL.value = await ensureSubscriptionURL(item)
    shareLink.value = withSubscriptionFormat(shareSubscriptionURL.value, selectedExportClient.value)
    shareOpen.value = true
    void renderShareQRCode()
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

function withSubscriptionFormat(value: string, client: ExportClientId) {
  const url = new URL(value, window.location.origin)
  if (client === 'v2rayn') url.searchParams.delete('format')
  else url.searchParams.set('format', client === 'sing-box' ? 'sing-box' : client)
  return url.toString()
}

async function removeSubscription(item: Subscription) {
  if (!window.confirm(t('deleteSubscriptionConfirm', { name: item.name }))) return
  loading.value = true
  try {
    await api.deleteSubscription(item.id)
    delete subscriptionURLs[item.id]
    notify(t('subscriptionDeleted'))
    await refreshSubscriptions()
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

function formPayload(): CreateInbound {
  return {
    remark: form.remark,
    listen: form.listen,
    port: form.port,
    protocol: form.protocol,
    network: form.network,
    security: form.security,
    clientId: form.clientId,
    email: form.email,
    enabled: form.enabled,
    totalBytes: Math.round(form.totalGB * gib),
    expiryTime: form.expiryLocal ? new Date(form.expiryLocal).toISOString() : '',
    alterId: form.protocol === 'vmess' ? form.alterId : 0,
    sniffing: form.sniffing,
    wsPath: form.network === 'ws' ? form.wsPath : '/xpanel',
    tlsCertFile: form.security === 'tls' ? form.tlsCertFile : '',
    tlsKeyFile: form.security === 'tls' ? form.tlsKeyFile : '',
  }
}

async function saveInbound() {
  loading.value = true
  error.value = ''
  try {
    const payload = formPayload()
    const saved = editingInboundId.value
      ? await api.update(editingInboundId.value, payload)
      : await api.create(payload)
    modalOpen.value = false
    notify(editingInboundId.value ? t('inboundUpdated', { name: saved.remark }) : t('inboundCreated', { name: saved.remark }))
    editingInboundId.value = null
    await refresh()
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function removeInbound(item: Inbound) {
  if (!window.confirm(t('deleteInboundConfirm', { name: item.remark || item.tag }))) return
  loadingNodes.value = true
  error.value = ''
  try {
    await api.deleteInbound(item.id)
    notify(t('inboundDeleted', { name: item.remark || item.tag }))
    await Promise.all([refresh(), refreshSubscriptions()])
  } catch (cause) { fail(errorText(cause)) }
  finally { loadingNodes.value = false }
}

async function savePanelPort() {
  loading.value = true
  try {
    await api.updatePanelPort(settingsForm.port)
    restartNeeded.value = true
    notify(t('restartRequired'))
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function saveAccount() {
  loading.value = true
  try {
    await api.updateAccount({ username: settingsForm.username, password: settingsForm.password })
    username.value = settingsForm.username
    authForm.username = settingsForm.username
    authForm.password = ''
    settingsForm.password = ''
    restartNeeded.value = true
    notify(t('restartRequired'))
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function saveIntegration(rotateToken = false) {
  loading.value = true
  try {
    const allowedIps = integrationForm.allowedIps
      .split(/[\n,;]+/)
      .map(value => value.trim())
      .filter(Boolean)
    const result = await api.updateIntegration({ allowedIps, rotateToken })
    if (panelSettings.value) panelSettings.value.integration = result.integration
    integrationForm.allowedIps = result.integration.allowedIps.join('\n')
    if (result.token) {
      freshIntegrationToken.value = result.token
      await copyText(result.token)
      notify(t('integrationTokenRotated'))
    } else {
      notify(t('integrationSaved'))
    }
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

async function restartPanel() {
  loading.value = true
  try {
    await api.restartPanel()
    notify(t('restartingPanel'))
  } catch (cause) { fail(errorText(cause)) }
  finally { loading.value = false }
}

function exportInbound(item: Inbound) {
  shareSource.value = 'inbound'
  shareNode.value = item
  shareSubscriptionURL.value = ''
  shareRemark.value = item.remark || item.tag
  shareExpiry.value = item.expiryTime
  shareTotal.value = item.totalBytes
  shareUsed.value = item.usedBytes
  shareRemaining.value = item.remainingBytes
  selectedExportClient.value = 'nexora'
  shareLink.value = buildClientExport(item, exportAddress(item.listen), selectedExportClient.value)
  shareOpen.value = true
  void renderShareQRCode()
}

async function copyClientExport(client: ExportClientId, showToast = true) {
  selectedExportClient.value = client
  refreshShareLink()
  if (!shareLink.value) return
  await copyText(shareLink.value)
  if (showToast) notify(t('clientExportCopied', { client: clientName(client) }))
}

function selectShareClient(client: ExportClientId) {
  selectedExportClient.value = client
  refreshShareLink()
  void renderShareQRCode()
}

function refreshShareLink() {
  if (shareSource.value === 'subscription') {
    shareLink.value = shareSubscriptionURL.value ? withSubscriptionFormat(shareSubscriptionURL.value, selectedExportClient.value) : ''
    return
  }
  shareLink.value = shareNode.value ? buildClientExport(shareNode.value, exportAddress(shareNode.value.listen), selectedExportClient.value) : ''
}

async function renderShareQRCode() {
  await nextTick()
  if (!shareQRCode.value || !shareLink.value) return
  try {
    await QRCode.toCanvas(shareQRCode.value, shareLink.value, {
      width: 260,
      margin: 2,
      errorCorrectionLevel: 'L',
      color: { dark: '#111827', light: '#ffffff' },
    })
  } catch {
    const context = shareQRCode.value.getContext('2d')
    context?.clearRect(0, 0, shareQRCode.value.width, shareQRCode.value.height)
  }
}

function clientIcon(client: ExportClientId) {
  if (client === 'nexora') return '/nexora.png'
  if (client === 'clash') return '/clash.jpg'
  return `/${client}.png`
}

function clientName(client: ExportClientId) {
  return exportClients.find(item => item.id === client)?.name || client
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

function formatBytes(value: number) {
  if (!value) return t('unlimited')
  if (value >= gib) return `${(value / gib).toFixed(1)} GB`
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}
function formatShareTotal() {
  return shareSource.value === 'subscription' ? formatBytesExact(shareTotal.value) : formatBytes(shareTotal.value)
}
function formatShareRemaining() {
  if (shareSource.value === 'subscription') return formatBytesExact(shareRemaining.value)
  return shareTotal.value ? formatBytesExact(shareRemaining.value) : t('unlimited')
}
function formatBytesExact(value: number) {
  if (!value) return '0 B'
  if (value >= gib) return `${(value / gib).toFixed(1)} GB`
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}
function formatSpeed(value: number) { return `${formatBytesExact(value)}/s` }
function formatExpiry(value: string) { return value ? new Date(value).toLocaleString(language.value === 'zh' ? 'zh-CN' : 'en-US') : t('never') }
function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return days > 0 ? `${days}d ${hours}h ${minutes}m` : `${hours}h ${minutes}m`
}
function percent(used: number, total: number) { return total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0 }
function gaugeStyle(value: number) { return { '--value': `${Math.max(0, Math.min(100, value)) * 3.6}deg` } }
function toLocalInput(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}
function errorText(cause: unknown) { return cause instanceof Error ? cause.message : String(cause) }

onMounted(() => {
  setLanguage(language.value)
  void loadStatus()
  sessionTimer = window.setInterval(() => void verifySession(), 3000)
  window.addEventListener('focus', verifySession)
})
onBeforeUnmount(() => {
  if (sessionTimer !== undefined) window.clearInterval(sessionTimer)
  if (toastTimer !== undefined) window.clearTimeout(toastTimer)
  stopStatusPolling()
  window.removeEventListener('focus', verifySession)
})
</script>

<template>
  <div v-if="!authReady" class="auth-page boot-page">
    <section class="auth-card">
      <div class="logo-orb"><IconShieldCheck /></div>
    </section>
  </div>

  <div v-else-if="!authenticated" class="auth-page">
    <div class="language-switch auth-language" role="group" aria-label="Language">
      <button :class="{ active: language === 'en' }" @click="setLanguage('en')">EN</button>
      <button :class="{ active: language === 'zh' }" @click="setLanguage('zh')">中文</button>
    </div>
    <section class="auth-card">
      <div class="logo-orb"><IconShieldCheck /></div>
      <p class="eyebrow">{{ needsSetup ? t('firstRun') : t('signInEyebrow') }}</p>
      <h1>{{ needsSetup ? t('createAdmin') : t('welcome') }}</h1>
      <p v-if="needsSetup" class="auth-copy">{{ t('setupCopy') }}</p>
      <form @submit.prevent="submitAuth">
        <label><span>{{ t('username') }}</span><input v-model.trim="authForm.username" autocomplete="username" required /></label>
        <label><span>{{ t('password') }}</span><input v-model="authForm.password" type="password" :autocomplete="needsSetup ? 'new-password' : 'current-password'" required /></label>
        <button class="primary wide" :disabled="loading"><IconLock />{{ needsSetup ? t('createAccount') : t('signIn') }}</button>
      </form>
      <div v-if="error" class="inline-error"><IconAlertCircle />{{ error }}</div>
    </section>
  </div>

  <div v-else class="app-shell">
    <aside class="sidebar">
      <div class="brand"><IconRocket /><div><strong>XPanel</strong><small>XRAY OPERATIONS</small></div></div>
      <nav>
        <a :class="{ active: activeView === 'overview' }" href="#" @click.prevent="setActiveView('overview')"><IconDashboard /><span>{{ t('overview') }}</span></a>
        <a :class="{ active: activeView === 'inbounds' }" href="#" @click.prevent="setActiveView('inbounds')"><IconServer /><span>{{ t('inbounds') }}</span><b>{{ items.length }}</b></a>
        <a :class="{ active: activeView === 'subscriptions' }" href="#" @click.prevent="showSubscriptions"><IconLink /><span>{{ t('subscriptions') }}</span><b>{{ subscriptions.length }}</b></a>
        <a :class="{ active: activeView === 'settings' }" href="#" @click.prevent="setActiveView('settings')"><IconSettings /><span>{{ t('settings') }}</span></a>
        <button class="sidebar-menu-button" :class="{ expanded: githubExpanded }" @click="githubExpanded=!githubExpanded">
          <IconBrandGithub /><span>{{ t('github') }}</span><IconChevronDown class="menu-chevron" />
        </button>
        <div v-if="githubExpanded" class="github-submenu">
          <a v-for="project in githubProjects" :key="project.name" :href="project.url" target="_blank" rel="noreferrer">
            <span>{{ project.name }}</span><IconExternalLink />
          </a>
        </div>
      </nav>
      <button class="logout" @click="logout"><IconLogout />{{ t('signOut') }}</button>
    </aside>

    <main class="content">
      <header class="topbar">
        <div class="topbar-user"><span>{{ t('signedInAs', { name: username }) }}</span></div>
        <div class="header-actions">
          <div class="language-switch" role="group" aria-label="Language">
            <button :class="{ active: language === 'en' }" @click="setLanguage('en')">EN</button>
            <button :class="{ active: language === 'zh' }" @click="setLanguage('zh')">中文</button>
          </div>
          <div class="health"><i></i><span>{{ t('panelOnline') }}</span></div>
        </div>
      </header>

      <div v-if="error" class="toast error"><IconAlertCircle />{{ error }}<button @click="error=''">×</button></div>
      <div v-if="message" class="toast success"><IconCheck />{{ message }}<button @click="message=''">×</button></div>

      <template v-if="activeView === 'overview'">
        <section class="summary-grid">
          <article><span>{{ t('totalInbounds') }}</span><strong>{{ items.length }}</strong><small>{{ t('configuredListeners') }}</small></article>
          <article><span>{{ t('enabled') }}</span><strong>{{ enabledCount }}</strong><small>{{ t('activeRecords') }}</small></article>
          <article><span>{{ t('trafficQuota') }}</span><strong>{{ formatBytes(totalQuota) }}</strong><small>{{ t('zeroUnlimited') }}</small></article>
        </section>
        <section class="gauge-grid">
          <article class="gauge-card">
            <div class="gauge" :style="gaugeStyle(systemStatus?.cpuPercent || 0)"><strong>{{ Math.round(systemStatus?.cpuPercent || 0) }}%</strong></div>
            <div><IconCpu /><span>{{ t('cpu') }}</span><small>{{ t('loadAverage') }} {{ systemStatus?.load1.toFixed(2) || '0.00' }} / {{ systemStatus?.load5.toFixed(2) || '0.00' }} / {{ systemStatus?.load15.toFixed(2) || '0.00' }}</small></div>
          </article>
          <article class="gauge-card">
            <div class="gauge" :style="gaugeStyle(percent(systemStatus?.memory.used || 0, systemStatus?.memory.total || 0))"><strong>{{ percent(systemStatus?.memory.used || 0, systemStatus?.memory.total || 0) }}%</strong></div>
            <div><IconChartDonut /><span>{{ t('memory') }}</span><small>{{ formatBytesExact(systemStatus?.memory.used || 0) }} / {{ formatBytesExact(systemStatus?.memory.total || 0) }}</small></div>
          </article>
          <article class="gauge-card">
            <div class="gauge" :style="gaugeStyle(percent(systemStatus?.disk.used || 0, systemStatus?.disk.total || 0))"><strong>{{ percent(systemStatus?.disk.used || 0, systemStatus?.disk.total || 0) }}%</strong></div>
            <div><IconDatabase /><span>{{ t('disk') }}</span><small>{{ formatBytesExact(systemStatus?.disk.used || 0) }} / {{ formatBytesExact(systemStatus?.disk.total || 0) }}</small></div>
          </article>
          <article class="gauge-card">
            <div class="gauge" :style="gaugeStyle(percent(systemStatus?.swap.used || 0, systemStatus?.swap.total || 0))"><strong>{{ percent(systemStatus?.swap.used || 0, systemStatus?.swap.total || 0) }}%</strong></div>
            <div><IconRefresh /><span>{{ t('swap') }}</span><small>{{ formatBytesExact(systemStatus?.swap.used || 0) }} / {{ formatBytesExact(systemStatus?.swap.total || 0) }}</small></div>
          </article>
        </section>
        <section class="detail-grid">
          <article class="detail-panel">
            <h2>{{ t('liveTraffic') }}</h2>
            <div class="metric-row"><IconUpload /><span>{{ t('uploadSpeed') }}</span><strong>{{ formatSpeed(systemStatus?.uploadBps || 0) }}</strong></div>
            <div class="metric-row"><IconDownload /><span>{{ t('downloadSpeed') }}</span><strong>{{ formatSpeed(systemStatus?.downloadBps || 0) }}</strong></div>
          </article>
          <article class="detail-panel">
            <h2>{{ t('serverDetails') }}</h2>
            <div class="metric-row"><span>{{ t('uptime') }}</span><strong>{{ formatUptime(systemStatus?.uptime || 0) }}</strong></div>
            <div class="metric-row"><span>{{ t('platform') }}</span><strong>{{ systemStatus ? `${systemStatus.os}/${systemStatus.arch}` : '-' }}</strong></div>
            <div class="metric-row"><span>{{ t('collectedAt') }}</span><strong>{{ systemStatus ? new Date(systemStatus.collectedAt).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US') : '-' }}</strong></div>
          </article>
        </section>
      </template>

      <template v-else-if="activeView === 'inbounds'">
        <section class="table-panel">
          <div class="table-toolbar">
            <div><h2>{{ t('inboundNodes') }}</h2></div>
            <div class="toolbar-actions">
              <button class="ghost" :disabled="loadingNodes" @click="refresh"><IconRefresh />{{ t('refresh') }}</button>
              <button class="primary" @click="openCreate"><IconPlus />{{ t('addInbound') }}</button>
            </div>
          </div>
          <div v-if="loadingNodes" class="loading-state"><IconRefresh />{{ t('loadingNodes') }}</div>
          <div v-else class="table-wrap">
            <table>
              <thead><tr><th>{{ t('status') }}</th><th>{{ t('remark') }}</th><th>{{ t('protocol') }}</th><th>{{ t('listen') }}</th><th>{{ t('port') }}</th><th>{{ t('transport') }}</th><th>{{ t('quota') }}</th><th>{{ t('expires') }}</th><th>{{ t('actions') }}</th></tr></thead>
              <tbody>
                <tr v-for="item in items" :key="item.id">
                  <td><span :class="['state-dot', { off: !item.enabled }]"></span>{{ item.enabled ? t('enabled') : t('disabled') }}</td>
                  <td><strong>{{ item.remark }}</strong><small>{{ item.tag }}</small></td>
                  <td><span class="protocol">{{ item.protocol }}</span></td>
                  <td><code>{{ item.listen }}</code></td>
                  <td><code>{{ item.port }}</code></td>
                  <td><span class="transport">{{ item.network }}</span><em v-if="item.security==='tls'">TLS</em></td>
                  <td>
                    <template v-if="item.subscriptionControlled">
                      <span class="managed-pill">{{ t('subscriptionControlled') }}</span>
                      <small>{{ item.subscriptionNames.join(', ') }}</small>
                      <small v-if="!item.enabled">{{ subscriptionBlockText(item.subscriptionBlockReason) }}</small>
                    </template>
                    <template v-else>{{ formatBytes(item.totalBytes) }}</template>
                  </td>
                  <td>
                    <template v-if="item.subscriptionControlled"><small>{{ t('nodeQuotaIgnored') }}</small></template>
                    <template v-else>{{ formatExpiry(item.expiryTime) }}</template>
                  </td>
                  <td class="row-actions">
                    <button class="icon-button" :title="t('exportTitle')" @click="exportInbound(item)"><IconDownload /></button>
                    <button class="icon-button" :title="t('edit')" @click="openEdit(item)"><IconEdit /></button>
                    <button class="danger-button" :title="t('deleteNode')" :disabled="loadingNodes" @click="removeInbound(item)"><IconTrash /></button>
                  </td>
                </tr>
                <tr v-if="!items.length"><td colspan="9" class="empty-state"><IconServer /><strong>{{ t('noNodes') }}</strong><span>{{ t('noNodesHelp') }}</span></td></tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>

      <template v-else-if="activeView === 'subscriptions'">
        <section class="table-panel subscription-panel">
          <div class="table-toolbar">
            <div><h2>{{ t('subscriptionLinks') }}</h2></div>
            <div class="toolbar-actions">
              <button class="ghost" :disabled="loading" @click="refreshSubscriptions"><IconRefresh />{{ t('refresh') }}</button>
              <button class="primary" :disabled="!items.length" @click="openSubscription()"><IconPlus />{{ t('addSubscription') }}</button>
            </div>
          </div>
          <div v-if="subscriptions.length" class="subscription-grid">
            <article v-for="item in subscriptions" :key="item.id" class="subscription-card">
              <div class="subscription-card-top">
                <div class="subscription-identity">
                  <header>
                    <div><span :class="['state-dot', { off: !item.enabled }]"></span><strong>{{ item.name }}</strong></div>
                    <small>{{ (item.inboundIds || []).length }} {{ t('nodes') }}</small>
                  </header>
                  <div class="token-row"><IconKey /><code>••••••••{{ item.tokenHint }}</code></div>
                  <div class="subscription-nodes">
                    <span v-for="id in (item.inboundIds || [])" :key="id">{{ items.find(node => node.id === id)?.remark || `#${id}` }}</span>
                  </div>
                </div>
                <div class="subscription-usage">
                  <div><span>{{ t('subscriptionUsage') }}</span><strong>{{ formatBytesExact(item.usedBytes) }} / {{ formatBytesExact(item.totalBytes) }}</strong></div>
                  <div><span>{{ t('remainingTraffic') }}</span><strong>{{ formatBytesExact(item.remainingBytes) }}</strong></div>
                  <div><span>{{ t('expiry') }}</span><strong>{{ formatExpiry(item.expiryTime) }}</strong></div>
                </div>
              </div>
              <p v-if="subscriptionURLs[item.id]" class="fresh-url">{{ subscriptionURLs[item.id] }}</p>
              <p v-else class="url-hint">{{ t('tokenHidden') }}</p>
              <div class="subscription-card-bottom">
                <footer>
                  <button class="icon-button" :title="t('exportTitle')" :disabled="loading" @click="exportSubscription(item)"><IconDownload /></button>
                  <button class="icon-button" :title="t('rotate')" @click="rotateSubscription(item)"><IconRefresh /></button>
                  <button class="icon-button" :title="t('edit')" @click="openSubscription(item)"><IconEdit /></button>
                  <button class="danger-button" @click="removeSubscription(item)"><IconTrash /></button>
                </footer>
              </div>
            </article>
          </div>
          <div v-else class="empty-state subscription-empty"><IconLink /><strong>{{ t('noSubscriptions') }}</strong><span>{{ t('noSubscriptionsHelp') }}</span></div>
        </section>
      </template>

      <template v-else>
        <section class="settings-workspace">
          <header class="settings-heading">
            <div><IconSettings /><div><h2>{{ t('settings') }}</h2><span>{{ t('settingsDescription') }}</span></div></div>
          </header>
          <div class="settings-grid">
            <article class="settings-panel">
              <header><IconSettings /><div><h3>{{ t('panelPort') }}</h3><span>{{ panelSettings?.listen || '-' }}</span></div></header>
              <form class="settings-inline-form" @submit.prevent="savePanelPort">
                <label><span>{{ t('port') }}</span><input v-model.number="settingsForm.port" type="number" min="1" max="65535" required /></label>
                <button class="primary" :disabled="loading"><IconCheck />{{ t('savePort') }}</button>
              </form>
            </article>
            <article class="settings-panel account-panel">
              <header><IconUserCog /><div><h3>{{ t('accountSecurity') }}</h3><span>{{ username }}</span></div></header>
              <form @submit.prevent="saveAccount">
                <label><span>{{ t('newUsername') }}</span><input v-model.trim="settingsForm.username" autocomplete="username" required /></label>
                <label><span>{{ t('newPassword') }}</span><input v-model="settingsForm.password" type="password" autocomplete="new-password" required /></label>
                <button class="primary" :disabled="loading"><IconCheck />{{ t('saveAccount') }}</button>
              </form>
            </article>
            <article class="settings-panel integration-panel">
              <header><IconShieldCheck /><div><h3>{{ t('integrationAccess') }}</h3><span>{{ t('integrationDescription') }}</span></div></header>
              <form class="integration-form" @submit.prevent="saveIntegration(false)">
                <label class="integration-ip-field"><span>{{ t('allowedSourceIps') }}</span><textarea v-model="integrationForm.allowedIps" rows="4" placeholder="43.136.117.106&#10;10.0.0.0/24"></textarea></label>
                <div class="integration-token-status">
                  <span>{{ t('serviceToken') }}</span>
                  <code>{{ panelSettings?.integration?.tokenConfigured ? panelSettings.integration.tokenHint : t('notConfigured') }}</code>
                </div>
                <div v-if="freshIntegrationToken" class="fresh-integration-token">
                  <span>{{ t('newServiceToken') }}</span>
                  <code>{{ freshIntegrationToken }}</code>
                  <button type="button" class="icon-button" :title="t('copyLink')" @click="copyText(freshIntegrationToken)"><IconCopy /></button>
                </div>
                <div class="integration-actions">
                  <button class="primary" :disabled="loading"><IconCheck />{{ t('saveWhitelist') }}</button>
                  <button type="button" class="ghost" :disabled="loading" @click="saveIntegration(true)"><IconKey />{{ t('rotateServiceToken') }}</button>
                </div>
              </form>
            </article>
          </div>
          <article v-if="restartNeeded" class="restart-panel">
            <IconAlertCircle />
            <strong>{{ t('restartRequired') }}</strong>
            <button class="primary" :disabled="loading" @click="restartPanel"><IconRefresh />{{ t('restartPanel') }}</button>
          </article>
        </section>
      </template>
    </main>

    <div v-if="modalOpen" class="modal-backdrop" @mousedown.self="modalOpen=false">
      <section class="modal" role="dialog" aria-modal="true">
        <header><div><p>{{ editingInboundId ? t('editInbound') : t('newInbound') }}</p><h2>{{ editingInboundId ? t('saveInbound') : t('addInbound') }}</h2></div><button class="close" @click="modalOpen=false"><IconX /></button></header>
        <form @submit.prevent="saveInbound">
          <div class="form-grid">
            <label class="wide"><span>{{ t('remark') }}</span><input v-model.trim="form.remark" placeholder="VPS VLESS TCP" required autofocus /></label>
            <label>
              <span>{{ t('enabled') }}</span>
              <button
                type="button"
                :class="['switch', { on: form.enabled }]"
                :disabled="!!editingInbound?.subscriptionControlled"
                @click="form.enabled=!form.enabled"
              ><i></i>{{ form.enabled ? t('enabled') : t('disabled') }}</button>
            </label>
            <div v-if="editingInbound?.subscriptionControlled" class="form-note wide">
              <IconAlertCircle />
              <span>{{ t('subscriptionManagedSwitchHint', { reason: subscriptionBlockText(editingInbound.subscriptionBlockReason) }) }}</span>
            </div>
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
          <footer><button type="button" class="cancel" @click="modalOpen=false">{{ t('cancel') }}</button><button class="primary submit" :disabled="loading">{{ loading ? t('saving') : (editingInboundId ? t('saveInbound') : t('createInbound')) }}</button></footer>
        </form>
      </section>
    </div>

    <div v-if="subscriptionModalOpen" class="modal-backdrop" @mousedown.self="subscriptionModalOpen=false">
      <section class="modal subscription-modal" role="dialog" aria-modal="true">
        <header><div><p>{{ t('subscription') }}</p><h2>{{ subscriptionForm.id ? t('editSubscription') : t('addSubscription') }}</h2></div><button class="close" @click="subscriptionModalOpen=false"><IconX /></button></header>
        <form @submit.prevent="saveSubscription">
          <div class="form-grid">
            <label class="wide"><span>{{ t('subscriptionName') }}</span><input v-model.trim="subscriptionForm.name" :placeholder="t('subscriptionNamePlaceholder')" required autofocus /></label>
            <label><span>{{ t('enabled') }}</span><button type="button" :class="['switch', { on: subscriptionForm.enabled }]" @click="subscriptionForm.enabled=!subscriptionForm.enabled"><i></i>{{ subscriptionForm.enabled ? t('enabled') : t('disabled') }}</button></label>
            <label><span>{{ t('subscriptionTraffic') }}</span><input v-model.number="subscriptionForm.totalGB" type="number" min="0" step="0.1" /></label>
            <label class="wide"><span>{{ t('subscriptionExpiry') }}</span><input v-model="subscriptionForm.expiryLocal" type="datetime-local" /></label>
            <fieldset class="wide node-picker">
              <legend>{{ t('selectNodes') }}</legend>
              <label v-for="item in items" :key="item.id" :class="{ selected: subscriptionForm.inboundIds.includes(item.id) }">
                <input type="checkbox" :checked="subscriptionForm.inboundIds.includes(item.id)" @change="toggleSubscriptionNode(item.id)" />
                <span><strong>{{ item.remark }}</strong><small>{{ item.protocol.toUpperCase() }} · {{ item.listen }}:{{ item.port }}</small></span>
              </label>
            </fieldset>
          </div>
          <footer><button type="button" class="cancel" @click="subscriptionModalOpen=false">{{ t('cancel') }}</button><button class="primary submit" :disabled="loading || !subscriptionForm.inboundIds.length">{{ loading ? t('saving') : t('saveSubscription') }}</button></footer>
        </form>
      </section>
    </div>

    <div v-if="shareOpen" class="modal-backdrop" @mousedown.self="shareOpen=false">
      <section class="modal share-modal" role="dialog" aria-modal="true">
        <header><div><p>{{ t('clientImportLink') }}</p><h2>{{ t('exportName', { name: shareRemark }) }}</h2></div><button class="close" @click="shareOpen=false"><IconX /></button></header>
        <div class="share-body">
          <div class="share-layout">
            <div class="share-options">
              <div class="client-export-row modal-client-row" :aria-label="t('exportClients')">
                <button
                  v-for="client in exportClients"
                  :key="client.id"
                  class="client-export-button"
                  :class="{ active: selectedExportClient === client.id, nexora: client.id === 'nexora' }"
                  @click="selectShareClient(client.id)"
                >
                  <IconRocket v-if="client.id === 'shadowrocket'" />
                  <img v-else :src="clientIcon(client.id)" alt="" />
                  <span>{{ client.name }}</span>
                </button>
              </div>
              <div class="share-info">
                <div><span>{{ t('expiry') }}</span><strong>{{ formatExpiry(shareExpiry) }}</strong></div>
                <div><span>{{ t('totalTraffic') }}</span><strong>{{ formatShareTotal() }}</strong></div>
                <div><span>{{ t('usedTraffic') }}</span><strong>{{ formatBytesExact(shareUsed) }}</strong></div>
                <div><span>{{ t('remainingTraffic') }}</span><strong>{{ formatShareRemaining() }}</strong></div>
              </div>
            </div>
            <div class="qr-panel">
              <canvas ref="shareQRCode"></canvas>
              <strong>{{ clientName(selectedExportClient) }}</strong>
              <span>{{ t('scanToImport') }}</span>
            </div>
          </div>
          <div class="share-actions">
            <button class="ghost" @click="copyClientExport(selectedExportClient)"><IconCopy />{{ t('copyLink') }}</button>
            <button class="primary" @click="shareOpen=false">{{ t('done') }}</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
