import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => vi.restoreAllMocks())

describe('api client', () => {
  it('returns inbound collection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ items: [] }),
    }))

    await expect(api.list()).resolves.toEqual({ items: [] })
    expect(fetch).toHaveBeenCalledWith('/api/v1/inbounds', expect.any(Object))
  })

  it('turns API errors into exceptions', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 422,
      text: async () => JSON.stringify({ message: 'validation failed' }),
    }))

    await expect(api.preview()).rejects.toThrow('validation failed')
  })

  it('accepts empty delete subscription responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 204, text: async () => '' }))
    await expect(api.deleteSubscription(7)).resolves.toBeUndefined()
    expect(fetch).toHaveBeenCalledWith('/api/v1/subscriptions/7', expect.objectContaining({ method: 'DELETE' }))
  })

  it('accepts empty delete inbound responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 204, text: async () => '' }))
    await expect(api.deleteInbound(3)).resolves.toBeUndefined()
    expect(fetch).toHaveBeenCalledWith('/api/v1/inbounds/3', expect.objectContaining({ method: 'DELETE' }))
  })

  it('reads the current subscription URL without rotating it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ url: 'https://example.test/sub/stable' }),
    }))

    await expect(api.subscriptionURL(7)).resolves.toEqual({ url: 'https://example.test/sub/stable' })
    expect(fetch).toHaveBeenCalledWith('/api/v1/subscriptions/7/url', expect.any(Object))
    const [, init] = vi.mocked(fetch).mock.calls[0]!
    expect(init).not.toEqual(expect.objectContaining({ method: 'POST' }))
  })

  it('renews a subscription by day count', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ id: 7 }),
    }))

    await api.renewSubscription(7, 30)

    expect(fetch).toHaveBeenCalledWith('/api/v1/subscriptions/7/renew', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ days: 30 }),
    }))
  })

  it('accepts empty successful responses without parsing JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => '' }))
    await expect(api.restartPanel()).resolves.toBeUndefined()
  })
})
