/**
 * 前端分类日志（F-37 / 10.3）。
 *
 * - 写入 IndexedDB 独立 store `app_logs`，与收藏/用户缓存隔离
 * - 批量写入（满 50 条或每 1 秒），失败降级 console，绝不抛出异常
 * - 容量上限 5000 条 / 约 5MB，超出按时间淘汰最旧
 * - 支持按分类检索与脱敏导出
 */
import { putMany, queryLogs, trimLogs, clearStore } from './db'

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type LogCategory =
  | 'api'
  | 'player'
  | 'interaction'
  | 'performance'
  | 'exception'
  | 'app'
  | 'auth'
  | string

export interface LogRecord {
  id?: number
  ts: string
  level: LogLevel
  category: LogCategory
  module: string
  message: string
  context?: Record<string, unknown>
  request_id?: string
  user_id?: number
}

const MAX_ENTRIES = 5000
const FLUSH_INTERVAL = 1000
const FLUSH_SIZE = 50

/** 是否将新日志上报后端（F-41，默认关闭，由服务端配置控制） */ 
let uploadEnabled = false

const SENSITIVE_KEYS = [
  'password',
  'token',
  'access_token',
  'refresh_token',
  'authorization',
  'cookie',
  'secret',
]

let buffer: LogRecord[] = []
let timer: number | null = null
let enabled = true
let currentUserId: number | null = null

/** 脱敏：敏感字段替换为 [redacted]，长字符串截断。 */
export function sanitizeContext(ctx?: Record<string, unknown>): Record<string, unknown> | undefined {
  if (!ctx) return undefined
  const out: Record<string, unknown> = {}
  Object.entries(ctx).forEach(([key, value]) => {
    const lower = key.toLowerCase()
    if (SENSITIVE_KEYS.some((k) => lower.includes(k))) {
      out[key] = '[redacted]'
      return
    }
    if (typeof value === 'string' && value.length > 512) {
      out[key] = value.slice(0, 512) + '…'
      return
    }
    if (value instanceof Error) {
      out[key] = value.message
      return
    }
    out[key] = value
  })
  return out
}

function scheduleFlush() {
  if (timer !== null) return
  timer = window.setTimeout(() => {
    timer = null
    void flush()
  }, FLUSH_INTERVAL)
}

async function upload(batch: LogRecord[]) {
  if (!uploadEnabled || !batch.length) return
  const token = localStorage.getItem('lanreel.access')
  if (!token) return
  try {
    await fetch('/api/v1/logs/client', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({
        entries: batch.map((r) => ({
          ts: r.ts,
          level: r.level,
          category: r.category,
          module: r.module,
          message: r.message,
          context: r.context ?? {},
          request_id: r.request_id ?? '',
        })),
      }),
      keepalive: true,
    })
  } catch {
    // 上报失败静默忽略：本地日志已落库，绝不因上报影响主流程（F-39）
  }
}

async function flush() {
  if (!buffer.length) return
  const batch = buffer
  buffer = []
  try {
    await putMany('app_logs', batch)
    if (batch.length >= FLUSH_SIZE) void trimLogs(MAX_ENTRIES)
    void upload(batch)
  } catch (err) {
    // 降级 console，不抛未捕获异常（F-39）
    console.warn('[lanreel][logger] 日志写入失败，降级输出', err)
    batch.forEach((r) => console.info(formatLine(r)))
  }
}

function formatLine(r: LogRecord): string {
  const ctx = r.context ? ' ' + JSON.stringify(r.context) : ''
  return `${r.ts} ${r.level.toUpperCase()} [${r.category}] ${r.module} ${r.message}${ctx}`
}

export const logger = {
  setEnabled(v: boolean) {
    enabled = v
  },
  setUploadEnabled(v: boolean) {
    uploadEnabled = v
  },
  setUser(userId: number | null) {
    currentUserId = userId
  },
  log(
    level: LogLevel,
    category: LogCategory,
    module: string,
    message: string,
    context?: Record<string, unknown>,
    requestId?: string,
  ) {
    if (!enabled) return
    const record: LogRecord = {
      ts: new Date().toISOString(),
      level,
      category,
      module,
      message,
      context: sanitizeContext(context),
      request_id: requestId,
      user_id: currentUserId ?? undefined,
    }
    buffer.push(record)
    if (buffer.length >= FLUSH_SIZE) {
      void flush()
    } else {
      scheduleFlush()
    }
  },
  debug(category: LogCategory, module: string, message: string, context?: Record<string, unknown>) {
    this.log('debug', category, module, message, context)
  },
  info(category: LogCategory, module: string, message: string, context?: Record<string, unknown>) {
    this.log('info', category, module, message, context)
  },
  warn(category: LogCategory, module: string, message: string, context?: Record<string, unknown>) {
    this.log('warn', category, module, message, context)
  },
  error(category: LogCategory, module: string, message: string, context?: Record<string, unknown>) {
    this.log('error', category, module, message, context)
  },
  flush,
  async query(options: { category?: string; level?: string; keyword?: string; limit?: number }) {
    await flush()
    return queryLogs(options)
  },
  async export(format: 'json' | 'text' = 'json'): Promise<string> {
    const records = await queryLogs({})
    if (format === 'text') {
      return records.map((r) => formatLine(r)).join('\n')
    }
    return JSON.stringify(
      records.map((r) => ({ ...r, context: sanitizeContext(r.context) })),
      null,
      2,
    )
  },
  async clear() {
    buffer = []
    await clearStore('app_logs')
  },
  /** 登出时对日志中的 user_id 做匿名化处理（N-27 / N-09）。 */
  async anonymize() {
    await flush()
    try {
      const all = await queryLogs({})
      const owned = all.filter((r) => r.user_id != null)
      if (!owned.length) return
      await putMany(
        'app_logs',
        owned.map((r) => ({ ...r, user_id: undefined })),
      )
    } catch (err) {
      console.warn('[lanreel][logger] 日志匿名化失败', err)
    }
  },
}

/** 注册全局异常钩子（10.3）。 */
export function installGlobalErrorHooks() {
  window.addEventListener('error', (event) => {
    logger.error('exception', 'window.onerror', event.message || '未捕获异常', {
      source: event.filename,
      line: event.lineno,
      column: event.colno,
      stack: event.error instanceof Error ? event.error.stack?.slice(0, 1024) : undefined,
    })
  })
  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason
    logger.error('exception', 'unhandledrejection', '未处理的 Promise 拒绝', {
      reason: reason instanceof Error ? reason.message : String(reason).slice(0, 300),
      stack: reason instanceof Error ? reason.stack?.slice(0, 1024) : undefined,
    })
  })
}
