export type Inbound = {
  id: number
  remark: string
  tag: string
  listen: string
  port: number
  protocol: 'vless' | 'vmess'
  network: 'tcp' | 'ws'
  security: 'none' | 'tls'
  clientId: string
  email: string
  enabled: boolean
  totalBytes: number
  expiryTime: string
  alterId: number
  sniffing: boolean
  wsPath: string
  tlsCertFile: string
  tlsKeyFile: string
  createdAt: string
}

export type CreateInbound = Omit<Inbound, 'id' | 'tag' | 'createdAt'>

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  const body = await response.json()
  if (!response.ok) throw new Error(body.message ?? `HTTP ${response.status}`)
  return body as T
}

export const api = {
  list: () => request<{ items: Inbound[] }>('/api/v1/inbounds'),
  create: (input: CreateInbound) =>
    request<Inbound>('/api/v1/inbounds', { method: 'POST', body: JSON.stringify(input) }),
  preview: () =>
    request<{ sha256: string; config: Record<string, unknown> }>('/api/v1/config/preview', { method: 'POST' }),
  apply: () =>
    request<{ configPath: string; sha256: string; output?: string }>('/api/v1/config/apply', { method: 'POST' }),
}
