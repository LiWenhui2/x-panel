import { describe, expect, it } from 'vitest'
import type { Inbound } from './api'
import { buildShareLink } from './share'

const inbound: Inbound = {
  id: 1, remark: 'demo', tag: 'inbound-1', listen: '0.0.0.0', port: 24443,
  protocol: 'vless', network: 'tcp', security: 'none',
  clientId: '11111111-1111-4111-8111-111111111111', email: 'demo@example.com', enabled: true,
  totalBytes: 1000, usedBytes: 250, remainingBytes: 750, expiryTime: '2099-12-31T23:59:59Z',
  alterId: 0, sniffing: true, wsPath: '/xpanel', tlsCertFile: '', tlsKeyFile: '', createdAt: '2026-01-01T00:00:00Z',
  subscriptionControlled: false, subscriptionNames: [],
}

describe('share links', () => {
  it('adds quota and expiry metadata to VLESS links', () => {
    const url = new URL(buildShareLink(inbound, '203.0.113.10'))
    expect(url.searchParams.get('xpanel_expiry')).toBe('2099-12-31T23:59:59Z')
    expect(url.searchParams.get('xpanel_total_bytes')).toBe('1000')
    expect(url.searchParams.get('xpanel_used_bytes')).toBe('250')
    expect(url.searchParams.get('xpanel_remaining_bytes')).toBe('750')
  })

  it('adds quota and expiry metadata to VMess payloads', () => {
    const link = buildShareLink({ ...inbound, protocol: 'vmess' }, '203.0.113.10')
    const payload = JSON.parse(atob(link.slice('vmess://'.length)))
    expect(payload.xpanel).toMatchObject({ totalBytes: 1000, usedBytes: 250, remainingBytes: 750 })
  })
})
