import type { Inbound } from './api'

export function buildShareLink(item: Inbound, address: string) {
  const name = encodeURIComponent(item.remark || item.tag || `${item.protocol}-${item.port}`)
  return item.protocol === 'vmess' ? buildVMessLink(item, address) : buildVLESSLink(item, address, name)
}

function buildVLESSLink(item: Inbound, address: string, name: string) {
  const params = new URLSearchParams()
  params.set('type', item.network)
  params.set('security', item.security)
  if (item.wsPath) params.set('path', item.wsPath)
  params.set('xpanel_email', item.email)
  params.set('xpanel_expiry', item.expiryTime)
  params.set('xpanel_total_bytes', String(item.totalBytes))
  params.set('xpanel_used_bytes', String(item.usedBytes))
  params.set('xpanel_remaining_bytes', String(item.remainingBytes))
  return `vless://${item.clientId}@${address}:${item.port}?${params.toString()}#${name}`
}

function buildVMessLink(item: Inbound, address: string) {
  const payload = {
    v: '2', ps: item.remark || item.tag, add: address, port: item.port,
    id: item.clientId, aid: item.alterId, net: item.network, type: 'none',
    host: '', path: item.network === 'ws' ? item.wsPath : '', tls: item.security === 'tls' ? 'tls' : 'none',
    xpanel: {
      email: item.email,
      expiryTime: item.expiryTime,
      totalBytes: item.totalBytes,
      usedBytes: item.usedBytes,
      remainingBytes: item.remainingBytes,
    },
  }
  return `vmess://${base64Encode(JSON.stringify(payload, null, 2))}`
}

function base64Encode(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary)
}
