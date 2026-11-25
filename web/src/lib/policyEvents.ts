/**
 * 可播放策略变更订阅（v0.3.1 实时同步加固）。
 *
 * 服务端只推送全局版本号，不推送用户或媒体细节；客户端收到后自行调用
 * /media/status 重新判定自己可见的视频。
 *
 * 注意：单二进制服务通常跑在 HTTP/1.1，浏览器对同源并发连接数有限制。
 * 播放中的 <video> 会长期占用连接，SSE 可能被排到视频播完才连上。
 * 因此这里采用「SSE + 轻量版本轮询」双通道：SSE 负责即时通知，
 * 短轮询负责在 SSE 被连接队列阻塞时快速兜底（前台 1.5 秒 / 后台 10 秒）。
 */
import { logger } from '@/lib/logger'

type PolicyListener = (version: number) => void

const listeners = new Set<PolicyListener>()
let source: EventSource | null = null
let pollTimer: number | null = null
let pollingActive = false
let watchdogTimer: number | null = null
let reconnectTimer: number | null = null
let visibilityListening = false
let lastVersion = 0
let lastEventAt = 0

function notify(version: number) {
  if (!Number.isFinite(version) || version <= 0 || version === lastVersion) return
  lastVersion = version
  lastEventAt = Date.now()
  for (const listener of listeners) {
    try {
      listener(version)
    } catch (err) {
      logger.warn('app', 'policy', '策略变更回调失败', { error: String(err) })
    }
  }
}

function parseEvent(raw: string | null | undefined): number {
  if (!raw) return 0
  try {
    const data = JSON.parse(raw) as { version?: number }
    return typeof data.version === 'number' ? data.version : 0
  } catch {
    return 0
  }
}

async function pollVersion(): Promise<number> {
  try {
    const res = await fetch('/api/v1/events/policy/version', {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    })
    if (!res.ok) return lastVersion
    const json = (await res.json()) as { data?: { version?: number } }
    const version = json.data?.version ?? 0
    if (version > 0) notify(version)
  } catch {
    /* 网络抖动时保持上次版本，等待下次轮询或 SSE 自动重连 */
  }
  return lastVersion
}

function pollDelay(): number {
  return typeof document !== 'undefined' && document.hidden ? 10000 : 1500
}

function scheduleNextPoll() {
  if (!listeners.size) return
  if (pollTimer !== null) window.clearTimeout(pollTimer)
  pollTimer = window.setTimeout(() => {
    pollTimer = null
    void pollVersion().finally(scheduleNextPoll)
  }, pollDelay())
}

function startPolling() {
  if (pollTimer !== null || pollingActive) return
  pollingActive = true
  void pollVersion().finally(() => {
    pollingActive = false
    scheduleNextPoll()
  })
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

function markAlive() {
  lastEventAt = Date.now()
}

function onVisibilityChange() {
  if (typeof document === 'undefined' || document.visibilityState === 'visible') {
    markAlive()
    // 页面从后台回到前台时立即补一次，避免用户看到旧状态。
    void pollVersion()
  }
  scheduleNextPoll()
}

function connect() {
  if (typeof EventSource === 'undefined') {
    startPolling()
    return
  }
  try {
    markAlive()
    source = new EventSource('/api/v1/events/policy')
    source.addEventListener('policy', (event) => {
      markAlive()
      notify(parseEvent((event as MessageEvent).data))
    })
    source.addEventListener('ping', markAlive)
    source.onerror = () => {
      // 半开连接不会触发 error； watchdog 与轮询会继续兜底。
      logger.debug('app', 'policy', '策略事件流断开，已由版本轮询兜底')
      void pollVersion()
    }
    if (watchdogTimer === null) {
      watchdogTimer = window.setInterval(() => {
        if (source && Date.now() - lastEventAt > 90000) {
          source.close()
          source = null
          startPolling()
          // 半开连接不能永久退化成轮询：稍后重建一次 SSE。
          if (reconnectTimer === null) {
            reconnectTimer = window.setTimeout(() => {
              reconnectTimer = null
              if (listeners.size && !source) connect()
            }, 10000)
          }
        }
      }, 30000)
    }
  } catch (err) {
    logger.warn('app', 'policy', '策略事件流不可用，改用轮询', { error: String(err) })
    startPolling()
  }
}

/** 订阅策略版本变化；返回取消订阅函数。无订阅者时自动关闭连接。 */
export function subscribePolicyChanges(listener: PolicyListener): () => void {
  listeners.add(listener)
  if (listeners.size === 1) {
    if (typeof document !== 'undefined' && !visibilityListening) {
      document.addEventListener('visibilitychange', onVisibilityChange)
      visibilityListening = true
    }
    connect()
    // 关键：无论 SSE 是否可用，都保留轻量轮询，避免视频流占满连接队列。
    startPolling()
  }
  return () => {
    listeners.delete(listener)
    if (listeners.size) return
    source?.close()
    source = null
    stopPolling()
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (watchdogTimer !== null) {
      window.clearInterval(watchdogTimer)
      watchdogTimer = null
    }
    if (visibilityListening && typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibilityChange)
      visibilityListening = false
    }
    // 重新订阅时允许首帧再触发一次全量校验，避免离线期间的变更被吞掉。
    lastVersion = 0
    lastEventAt = 0
  }
}

/** 供测试或主动刷新使用：立即读取一次当前策略版本。 */
export async function refreshPolicyVersion(): Promise<number> {
  return pollVersion()
}
