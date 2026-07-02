import type { Inbound } from './api'

export type ExportClientId = 'nexora' | 'v2rayn' | 'shadowrocket' | 'clash' | 'sing-box'

export function buildClientExport(item: Inbound, address: string, client: ExportClientId) {
  switch (client) {
    case 'nexora':
      return buildNexoraNodeExport(item, address)
    case 'clash':
      return buildClashNode(item, address)
    case 'sing-box':
      return buildSingBoxNode(item, address)
    case 'shadowrocket':
      return buildShareLink(item, address, true)
    default:
      return buildShareLink(item, address)
  }
}

export function buildShareLink(item: Inbound, address: string, shadowrocket = false) {
  const name = encodeURIComponent(item.remark || item.tag || `${item.protocol}-${item.port}`)
  return item.protocol === 'vmess' ? buildVMessLink(item, address) : buildVLESSLink(item, address, name, shadowrocket)
}

export function buildNexoraNodeExport(item: Inbound, address: string) {
  const node = nexoraNode(item, address)
  return JSON.stringify({
    version: 1,
    client: 'Nexora',
    type: 'node',
    generated_at: new Date().toISOString(),
    proxy_nodes: [node],
  }, null, 2)
}

function buildVLESSLink(item: Inbound, address: string, name: string, shadowrocket = false) {
  const params = new URLSearchParams()
  params.set('type', item.network)
  params.set('security', item.security)
  if (shadowrocket) params.set('encryption', 'none')
  if (item.wsPath) params.set('path', item.wsPath)
  return `vless://${item.clientId}@${address}:${item.port}?${params.toString()}#${name}`
}

function buildVMessLink(item: Inbound, address: string) {
  const payload = {
    v: '2',
    ps: item.remark || item.tag,
    add: address,
    port: String(item.port),
    id: item.clientId,
    aid: String(item.alterId || 0),
    net: item.network,
    type: 'none',
    host: '',
    path: item.network === 'ws' ? item.wsPath : '',
    tls: item.security === 'tls' ? 'tls' : '',
  }
  return `vmess://${base64Encode(JSON.stringify(payload))}`
}

function buildClashNode(item: Inbound, address: string) {
  const name = item.remark || item.tag
  const lines = [
    'proxies:',
    `  - name: ${quoteYaml(name)}`,
    `    type: ${item.protocol}`,
    `    server: ${quoteYaml(address)}`,
    `    port: ${item.port}`,
    `    uuid: ${item.clientId}`,
  ]
  if (item.protocol === 'vmess') {
    lines.push(`    alterId: ${item.alterId || 0}`, '    cipher: auto')
  }
  lines.push(`    tls: ${item.security === 'tls'}`, `    network: ${item.network}`)
  if (item.network === 'ws') lines.push('    ws-opts:', `      path: ${quoteYaml(item.wsPath || '/')}`)
  lines.push('proxy-groups:', '  - name: Nexora', '    type: select', '    proxies:', `      - ${quoteYaml(name)}`, 'rules:', '  - MATCH,Nexora')
  return lines.join('\n')
}

function buildSingBoxNode(item: Inbound, address: string) {
  const outbound: Record<string, unknown> = {
    type: item.protocol,
    tag: item.remark || item.tag,
    server: address,
    server_port: item.port,
    uuid: item.clientId,
  }
  if (item.protocol === 'vmess') {
    outbound.alter_id = item.alterId || 0
    outbound.security = 'auto'
  }
  if (item.security === 'tls') outbound.tls = { enabled: true }
  if (item.network === 'ws') outbound.transport = { type: 'ws', path: item.wsPath || '/' }
  return JSON.stringify({ log: { level: 'warn' }, outbounds: [outbound] }, null, 2)
}

function nexoraNode(item: Inbound, address: string) {
  const name = item.remark || item.tag
  return {
    id: item.id,
    user_id: 0,
    subscription_id: null,
    name,
    original_name: name,
    remark: item.remark,
    protocol: item.protocol,
    address,
    port: item.port,
    transport: item.network,
    security: item.security,
    sni: '',
    host: '',
    path: item.network === 'ws' ? item.wsPath : '',
    alpn: '',
    country_code: '',
    region: '',
    city: '',
    credential_ciphertext: '',
    credential: {
      uuid: item.clientId,
      email: item.email,
      alter_id: item.alterId || 0,
    },
    config_json: {
      network: item.network,
      sniffing: item.sniffing,
      tls_cert_file: item.tlsCertFile,
      tls_key_file: item.tlsKeyFile,
      total_bytes: item.totalBytes,
      used_bytes: item.usedBytes,
      remain_bytes: item.remainingBytes,
      expire_at: item.expiryTime,
    },
    share_link_ciphertext: '',
    share_link: buildShareLink(item, address),
    node_hash: '',
    enabled: item.enabled ? 1 : 0,
    created_at: item.createdAt,
    updated_at: item.createdAt,
  }
}

function quoteYaml(value: string) {
  return JSON.stringify(value)
}

function base64Encode(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary)
}
