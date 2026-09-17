export type UserRole = 'owner' | 'admin' | 'operator' | 'viewer'

export type User = {
  id: string
  username: string
  role: UserRole
  enabled: boolean
  createdAt?: string
  updatedAt?: string
  lastLoginAt?: string
}

const roleLevel: Record<UserRole, number> = { owner: 4, admin: 3, operator: 2, viewer: 1 }

export function can(user: User, required: UserRole) {
  return roleLevel[user.role] >= roleLevel[required]
}

export type Instance = {
  id: string
  name: string
  engine: string
  status: string
  createdAt: string
  apiToken?: string
  alwaysOnline: boolean
  rejectCall: boolean
  msgRejectCall: string
  readMessages: boolean
  ignoreGroups: boolean
  ignoreStatus: boolean
}

export type InstanceSettings = Pick<Instance,
  'alwaysOnline' | 'rejectCall' | 'msgRejectCall' | 'readMessages' | 'ignoreGroups' | 'ignoreStatus'>

export type ConnectionState = {
  status: string
  qrAvailable: boolean
  qrExpiresAt?: string
  lastError?: string
}

export type SentMessage = {
  id: string
  recipient: string
  timestamp: string
  type: 'text' | 'image' | 'video' | 'audio' | 'document' | 'sticker' | 'location' | 'contact' | 'poll' | 'reaction'
}

export type WebhookConfig = {
  url: string
  enabled: boolean
  hasSecret: boolean
  secret?: string
  events: string[]
}

export type Contact = {
  jid: string
  name: string
  phone?: string
  firstName?: string
  fullName?: string
  pushName?: string
  businessName?: string
}

export type ChatMessage = {
  eventId: string
  messageId?: string
  event: string
  chat: string
  sender?: string
  fromMe: boolean
  isGroup: boolean
  pushName?: string
  type?: string
  text?: string
  timestamp: string
  data: Record<string, unknown>
}

export type ChatSummary = {
  chat: string
  name: string
  isGroup: boolean
  unreadCount: number
  lastMessage: ChatMessage
}

export type MessageJob = {
  id: string
  instanceId: string
  kind: 'text' | 'image' | 'video' | 'audio' | 'document' | 'sticker'
  recipient: string
  payload: { message?: string; caption?: string; voice?: boolean; mimeType?: string; fileName?: string; fileSize?: number }
  idempotencyKey?: string
  status: 'queued' | 'processing' | 'retrying' | 'sent' | 'failed' | 'canceled'
  attempt: number
  maxAttempts: number
  scheduledAt: string
  nextAttemptAt?: string
  lastError?: string
  result?: SentMessage
  createdAt: string
  updatedAt: string
  completedAt?: string
}

export type MetricsSnapshot = {
  range: '24h' | '7d' | '30d'
  from: string
  to: string
  generatedAt: string
  uptimeSeconds: number
  instances: { total: number; connected: number }
  messages: { sent: number; received: number; total: number }
  queue: { queued: number; processing: number; retrying: number; pending: number; sent: number; failed: number; canceled: number }
  webhooks: { delivered: number; failed: number; total: number; successRate: number; averageTimeMs: number }
  byInstance: Array<{
    id: string
    name: string
    status: string
    messages: { sent: number; received: number; total: number }
    queue: { queued: number; processing: number; retrying: number; pending: number; sent: number; failed: number; canceled: number }
    webhooks: { delivered: number; failed: number; total: number; successRate: number; averageTimeMs: number }
  }>
}

export type ActivityEvent = {
  id: string
  instanceId: string
  event: string
  timestamp: string
  data: Record<string, unknown>
}

export type WebhookDelivery = {
  id: number
  eventId: string
  instanceId: string
  event: string
  url: string
  attempt: number
  status: 'delivered' | 'failed'
  httpStatus?: number
  error?: string
  durationMs: number
  manual: boolean
  createdAt: string
}

export type Page<T> = {
  data: T[]
  page: number
  pageSize: number
  total: number
  totalPages: number
}

export type Alert = {
  id: string
  severity: 'info' | 'warning' | 'critical'
  title: string
  message: string
  instanceId?: string
}

export type Backup = {
  id: string
  createdAt: string
  size: number
  reason: 'manual' | 'automatic' | 'pre-restore' | string
  version: number
}

export type BackupStatus = {
  data: Backup[]
  automatic: boolean
  intervalHours: number
  retention: number
  nextBackupAt?: string
}

export type AuditEntry = {
  id: string
  userId?: string
  username?: string
  action: string
  targetType?: string
  targetId?: string
  status: number
  sourceIp?: string
  details: Record<string, unknown>
  createdAt: string
}

type ApiError = { error?: string }

export async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers)
  if (options?.body && !(options.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(url, {
    ...options,
    headers,
  })
  if (!response.ok) {
    if (response.status === 401 && url !== '/api/auth/login') window.dispatchEvent(new Event('wirely:unauthorized'))
    const body = (await response.json().catch(() => ({}))) as ApiError
    throw new Error(body.error ?? 'Não foi possível concluir a operação')
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function statusLabel(status: string) {
  const labels: Record<string, string> = {
    connected: 'Conectada',
    connecting: 'Conectando',
    qr: 'Aguardando QR',
    error: 'Erro',
    logged_out: 'Desconectada',
    disconnected: 'Desconectada',
  }
  return labels[status] ?? status
}
