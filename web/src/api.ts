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
  usedBytes: number
  remainingBytes: number
  expiryTime: string
  alterId: number
  sniffing: boolean
  wsPath: string
  tlsCertFile: string
  tlsKeyFile: string
  createdAt: string
}

export type CreateInbound = Omit<Inbound, 'id' | 'tag' | 'createdAt' | 'usedBytes' | 'remainingBytes'>
export type Credentials = { username: string; password: string }
export type AuthStatus = { needsSetup: boolean; authenticated: boolean; username: string }

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  const body = await response.json()
  if (!response.ok) throw new Error(body.message ?? `HTTP ${response.status}`)
  return body as T
}

export const api = {
  authStatus: () => request<AuthStatus>('/api/v1/auth/status'),
  setup: (input: Credentials) =>
    request<{ authenticated: boolean; username: string }>('/api/v1/auth/setup', { method: 'POST', body: JSON.stringify(input) }),
  login: (input: Credentials) =>
    request<{ authenticated: boolean; username: string }>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify(input) }),
  logout: () => request<{ authenticated: boolean }>('/api/v1/auth/logout', { method: 'POST' }),
  list: () => request<{ items: Inbound[] }>('/api/v1/inbounds'),
  create: (input: CreateInbound) =>
    request<Inbound>('/api/v1/inbounds', { method: 'POST', body: JSON.stringify(input) }),
  preview: () =>
    request<{ sha256: string; config: Record<string, unknown> }>('/api/v1/config/preview', { method: 'POST' }),
  apply: () =>
    request<{ configPath: string; sha256: string; output?: string }>('/api/v1/config/apply', { method: 'POST' }),
}
