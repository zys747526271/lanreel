/**
 * 观看进度同步（v0.3.0）。
 *
 * - 服务端按用户隔离保存每条媒体的进度、观看时长、播放次数；
 * - 前端节流上报（默认每 5 秒一次，避免频繁写库），离开时强制上报；
 * - 提供「进度快照」供随机流判断是否从头播放、列表显示进度条。
 */
import {
  getProgress,
  reportProgress,
  restartProgress,
  type ProgressReport,
} from '@/api/watch'
import type { MediaItem, ProgressView } from '@/api/types'

/** key -> 进度快照 */
let snapshot: Record<string, ProgressView> = {}
let loaded = false

/** 加载当前用户的进度快照（登录后调用一次）。 */
export async function loadProgressMap(): Promise<Record<string, ProgressView>> {
  try {
    snapshot = (await getProgress()).data
  } catch {
    snapshot = {}
  }
  loaded = true
  return snapshot
}

export function getSnapshot(): Record<string, ProgressView> {
  return snapshot
}

export function progressOf(key: string): ProgressView | undefined {
  return snapshot[key]
}

/** 是否已经看完（>=95% 视为看完，再次打开从头播放）。 */
export function isWatchedThrough(key: string): boolean {
  const p = snapshot[key]
  return !!p && (p.completed || p.progress >= 0.95)
}

/** 从收藏/历史打开时的续播位置；看完的返回 0（从头）。 */
export function resumePosition(key: string): number {
  const p = snapshot[key]
  if (!p) return 0
  if (p.completed || p.progress >= 0.95) return 0
  return p.position
}

/** 更新本地快照（进度上报后立即反映到列表）。 */
function updateLocal(payload: ProgressReport) {
  const prev = snapshot[payload.video_key]
  const duration = payload.duration ?? prev?.duration ?? 0
  const position = payload.position
  snapshot[payload.video_key] = {
    position,
    duration,
    progress: duration > 0 ? Math.min(1, position / duration) : 0,
    completed: !!payload.completed || (prev?.completed ?? false),
    play_count: (prev?.play_count ?? 0) + (payload.restart ? 1 : 0),
    watched_seconds: (prev?.watched_seconds ?? 0) + Math.max(0, payload.watched_delta),
  }
}

// 每个 key 上次上报的时间与累计未上报的观看秒数。
const lastReport = new Map<string, number>()
const pendingDelta = new Map<string, number>()
/** 确实触发过 play 的 key；预加载/缩略图抽帧不会进入这里。 */
const realStartedKeys = new Set<string>()
/** 「从头开始」已计过一次播放，随后 seekTo(0) 触发的 play 不再重复 +1。 */
const suppressNextFirst = new Set<string>()
const REPORT_INTERVAL = 5000

/** 记录一次进度变化（由播放器 progress 事件每 ~1s 触发）。节流上报服务端。 */
export function trackProgress(payload: {
  key: string
  current: number
  duration: number
  started?: boolean
  justStarted?: boolean
  watchedDelta?: number
}, item?: MediaItem) {
  if (payload.started) realStartedKeys.add(payload.key)
  // watchedDelta 由播放器按自然播放推进累计；拖动进度条产生的跳变为 0。
  const delta = Math.max(0, payload.watchedDelta ?? 0)
  if (realStartedKeys.has(payload.key) && delta > 0) {
    pendingDelta.set(payload.key, (pendingDelta.get(payload.key) ?? 0) + delta)
  }
  let firstThisPlay = false
  if (payload.justStarted) {
    // 每次真实开始播放都要计入播放次数；从头开始接口已计数的那一次除外。
    firstThisPlay = !suppressNextFirst.has(payload.key)
    if (!firstThisPlay) suppressNextFirst.delete(payload.key)
  }
  const last = lastReport.get(payload.key) ?? 0
  // 刚开始播放必须立刻落历史；普通进度仍按 5 秒节流。
  if (!payload.justStarted && performance.now() - last < REPORT_INTERVAL) return
  void flush(payload.key, payload.current, payload.duration, item, firstThisPlay)
}

/** 立即上报某条进度（离开/切换/结束时调用）。 */
export async function flush(key: string, current: number, duration: number, item?: MediaItem, firstThisPlay = false) {
  const delta = pendingDelta.get(key) ?? 0
  const started = realStartedKeys.has(key)
  pendingDelta.delete(key)
  lastReport.set(key, performance.now())
  // 没有真实播放且没有观看时长：直接跳过，避免缓存/预加载污染历史。
  if (!started && delta <= 0) return
  const prevWatched = snapshot[key]?.watched_seconds ?? 0
  const completed = duration > 0 && prevWatched + delta >= duration * 0.98
  // 只有刚开始播放的那一次上报 first=true，服务端据此把播放次数 +1。
  const first = started && firstThisPlay
  const report: ProgressReport = {
    video_key: key,
    position: current,
    duration,
    watched_delta: delta,
    completed,
    first,
    started,
    ...(item
      ? {
          rel_path: item.rel_path,
          media_type: item.media_type,
          root_index: item.root_index,
          folder_path: item.folder,
        }
      : {}),
  }
  updateLocal(report)
  try {
    await reportProgress(report)
  } catch {
    /* 上报失败不影响播放 */
  }
}

/** 从头播放：清零服务端与本地进度。 */
export async function restart(key: string) {
  try {
    await restartProgress(key)
  } catch {
    /* ignore */
  }
  // restart 接口已让服务端播放次数 +1，这里标记避免随后 flush 再次计首播。
  realStartedKeys.add(key)
  suppressNextFirst.add(key)
  snapshot[key] = { position: 0, duration: snapshot[key]?.duration ?? 0, progress: 0, completed: false, play_count: (snapshot[key]?.play_count ?? 0) + 1, watched_seconds: 0 }
  lastReport.delete(key)
  pendingDelta.delete(key)
}

export function isLoaded() {
  return loaded
}
