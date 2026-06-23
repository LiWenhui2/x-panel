import type { Inbound } from './api'

export function buildShareLink(item: Inbound, address: string) {
  const name = encodeURIComponent(item.remark || item.tag || `${item.protocol}-${item.port}`)
  return item.protocol === 'vmess' ? buildVMessLink(item, address) : buildVLESSLink(item, address, name)
}

function buildVLESSLink(item: Inbound, address: string, name: string) {
  const params = new URLSearchParams()
  params.set('type', item.network)
  params.set('security', item.security)
  if (item.wsPath) {
    params.set('path', item.wsPath)
    params.set('xpanel_path', item.wsPath)
  }
  for (const [key, value] of Object.entries(metadataStrings(item, address))) params.set(key, value)
  return `vless://${item.clientId}@${address}:${item.port}?${params.toString()}#${name}`
}

function buildVMessLink(item: Inbound, address: string) {
  const payload = {
    v: '2', ps: item.remark || item.tag, add: address, port: item.port,
    id: item.clientId, aid: item.alterId, net: item.network, type: 'none',
    host: '', path: item.network === 'ws' ? item.wsPath : '', tls: item.security === 'tls' ? 'tls' : 'none',
    xpanel: metadataObject(item, address),
  }
  return `vmess://${base64Encode(JSON.stringify(payload, null, 2))}`
}

function metadataStrings(item: Inbound, address: string) {
  return {
    xpanel_name: item.remark || item.tag,
    xpanel_original_name: item.remark || item.tag,
    xpanel_remark: item.remark,
    xpanel_protocol: item.protocol,
    xpanel_address: address,
    xpanel_port: String(item.port),
    xpanel_transport: item.network,
    xpanel_security: item.security,
    xpanel_sni: '',
    xpanel_host: '',
    xpanel_alpn: '',
    xpanel_email: item.email,
    xpanel_expire_at: item.expiryTime,
    xpanel_expiry: item.expiryTime,
    xpanel_total_bytes: String(item.totalBytes),
    xpanel_used_bytes: String(item.usedBytes),
    xpanel_remain_bytes: String(item.remainingBytes),
    xpanel_remaining_bytes: String(item.remainingBytes),
  }
}

function metadataObject(item: Inbound, address: string) {
  return {
    name: item.remark || item.tag,
    original_name: item.remark || item.tag,
    remark: item.remark,
    protocol: item.protocol,
    address,
    port: item.port,
    transport: item.network,
    security: item.security,
    sni: '',
    host: '',
    path: item.wsPath,
    alpn: '',
    email: item.email,
    credential: { uuid: item.clientId, alter_id: item.alterId },
    config: { network: item.network, security: item.security, ws_path: item.wsPath },
    expire_at: item.expiryTime,
    total_bytes: item.totalBytes,
    used_bytes: item.usedBytes,
    remain_bytes: item.remainingBytes,
  }
}

function base64Encode(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary)
}
