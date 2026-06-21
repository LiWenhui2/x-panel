import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => vi.restoreAllMocks())

describe('api client', () => {
  it('returns inbound collection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ items: [] }),
    }))

    await expect(api.list()).resolves.toEqual({ items: [] })
    expect(fetch).toHaveBeenCalledWith('/api/v1/inbounds', expect.any(Object))
  })

  it('turns API errors into exceptions', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 422,
      json: async () => ({ message: 'validation failed' }),
    }))

    await expect(api.preview()).rejects.toThrow('validation failed')
  })
})

