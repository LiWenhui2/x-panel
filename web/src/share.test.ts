import { describe, expect, it } from 'vitest'
import type { Inbound } from './api'
import { buildClientExport, buildNexoraNodeExport, buildShareLink } from './share'

const inbound: Inbound = {
  id: 1, remark: 'demo', tag: 'inbound-1', listen: '0.0.0.0', port: 24443,
  protocol: 'vless', network: 'tcp', security: 'none',
  clientId: '11111111-1111-4111-8111-111111111111', email: 'demo@example.com', enabled: true,
  totalBytes: 1000, usedBytes: 250, remainingBytes: 750, expiryTime: '2099-12-31T23:59:59Z',
  alterId: 0, sniffing: true, wsPath: '/xpanel', tlsCertFile: '', tlsKeyFile: '', createdAt: '2026-01-01T00:00:00Z',
  subscriptionControlled: false, subscriptionNames: [], subscriptionBlockReason: '',
}

describe('share links', () => {
  it('keeps standard VLESS links compact for v2rayN', () => {
    const url = new URL(buildShareLink(inbound, '203.0.113.10'))
    expect(url.protocol).toBe('vless:')
    expect(url.searchParams.get('type')).toBe('tcp')
    expect(url.searchParams.has('xpanel_total_bytes')).toBe(false)
  })

  it('keeps standard VMess payloads compact', () => {
    const link = buildShareLink({ ...inbound, protocol: 'vmess' }, '203.0.113.10')
    const payload = JSON.parse(atob(link.slice('vmess://'.length)))
    expect(payload.add).toBe('203.0.113.10')
    expect(payload.xpanel).toBeUndefined()
  })

  it('exports Nexora node data with backend schema keys', () => {
    const document = JSON.parse(buildNexoraNodeExport(inbound, '203.0.113.10'))
    expect(document.client).toBe('Nexora')
    expect(document.proxy_nodes[0]).toMatchObject({
      name: 'demo',
      address: '203.0.113.10',
      credential: { uuid: inbound.clientId, email: inbound.email },
    })
    expect(document.proxy_nodes[0].config_json).toMatchObject({ total_bytes: 1000, remain_bytes: 750 })
  })

  it('builds client-specific exports', () => {
    expect(buildClientExport(inbound, '203.0.113.10', 'clash')).toContain('proxies:')
    expect(buildClientExport(inbound, '203.0.113.10', 'sing-box')).toContain('"outbounds"')
    const shadowrocket = new URL(buildClientExport(inbound, '203.0.113.10', 'shadowrocket'))
    expect(shadowrocket.searchParams.get('encryption')).toBe('none')
    expect(shadowrocket.searchParams.get('headerType')).toBe('none')
  })
})
