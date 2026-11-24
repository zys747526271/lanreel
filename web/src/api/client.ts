/**
 * 统一请求层（9.6 前端可扩展性）：
 * 封装鉴权、Token 刷新、错误归一、请求 ID 透传与 api 分类日志。
 */
import { logger } from '@/lib/logger'
import type { PublicConfig, TokenPair } from './types'

const API_PREFIX = '/api/v1'
const ACCESS_KEY = 'lanreel.access'
const REFRESH_KEY = 'lanreel.refresh'

export class ApiError extends Error {
  status: number
  code: string
  requestId?: string

  constructor(status: number, code: string, message: string, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

export const tokenStorage = {
  get access() {
    return localStorage.getItem(ACCESS_KEY) ?? ''
  },
  get refresh() {
    return localStorage.getItem(REFRESH_KEY) ?? ''
  },
  save(tokens: TokenPair) {
    localStorage.setItem(ACCESS_KEY, tokens.access_token)
    localStorage.setItem(REFRESH_KEY, tokens.refresh_token)
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY)
    localStorage.removeItem(REFRESH_KEY)
  },
}

function newRequestId(): string {
  const bytes = new Uint8Array(8)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

let refreshing: Promise<boolean> | null = null

async function refreshToken(): Promise<boolean> {
  if (refreshing) return refreshing
  const refresh = tokenStorage.refresh
  if (!refresh) return false
  refreshing = (async () => {
    try {
      const res = await fetch(`${API_PREFIX}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refresh }),
      })
      if (!res.ok) return false
      const json = await res.json()
      tokenStorage.save(json.data.tokens)
      return true
    } catch {
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  auth?: boolean
  signal?: AbortSignal
  /** 跳过一次自动刷新重试，避免登录接口递归 */
  noRetry?: boolean
  quiet?: boolean
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, auth = true, signal, noRetry = false, quiet = false } = options
  const requestId = newRequestId()
  const headers: Record<string, string> = { 'X-Request-ID': requestId }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (auth && tokenStorage.access) headers.Authorization = `Bearer ${tokenStorage.access}`

  const started = performance.now()
  let response: Response
  try {
    response = await fetch(`${API_PREFIX}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    })
  } catch (err) {
    logger.error('api', 'fetch', '网络请求失败', {
      path,
      method,
      error: err instanceof Error ? err.message : String(err),
      request_id: requestId,
    })
    throw new ApiError(0, 'network_error', '网络不可达，请检查局域网连接')
  }
  const duration = Math.round(performance.now() - started)

  if (response.status === 401 && auth && !noRetry) {
    const okRefresh = await refreshToken()
    if (okRefresh) {
      return request<T>(path, { ...options, noRetry: true })
    }
    tokenStorage.clear()
  }

  let payload: unknown = null
  const text = await response.text()
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }

  if (!response.ok) {
    const errPayload = (payload as { error?: { code?: string; message?: string } } | null)?.error
    const code = errPayload?.code ?? `http_${response.status}`
    const message = errPayload?.message ?? `请求失败（${response.status}）`
    logger.warn('api', 'request', `接口错误 ${method} ${path}`, {
      status: response.status,
      code,
      message,
      duration_ms: duration,
      request_id: requestId,
    })
    throw new ApiError(response.status, code, message, requestId)
  }

  if (!quiet) {
    logger.debug('api', 'request', `${method} ${path}`, {
      status: response.status,
      duration_ms: duration,
      request_id: requestId,
    })
  }
  return payload as T
}

export async function fetchPublicConfig(): Promise<PublicConfig> {
  const res = await request<{ data: PublicConfig }>('/config/public', {
    auth: false,
    noRetry: true,
  })
  return res.data
}

/** 媒体地址附上访问令牌（<video>/<img> 无法自定义请求头，见 N-14）。 */
export function withToken(url: string): string {
  const token = tokenStorage.access
  if (!token) return url
  const sep = url.includes('?') ? '&' : '?'
  return `${url}${sep}token=${encodeURIComponent(token)}`
}
