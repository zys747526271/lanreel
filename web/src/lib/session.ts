/**
 * 浏览会话状态（仅存 sessionStorage，关闭标签页即失效）。
 *
 * 用途：
 *  1. 从设置/后台返回时回到进入前的视频与播放进度；
 *  2. 记住最近一次播放器路由，让设置页/后台的「返回」按钮直达播放器，
 *     而不是永远回到随机流第一屏。
 */
import type { MediaItem } from '@/api/types'

const FEED_KEY = 'lanreel.session.feed'
const PLAY_KEY = 'lanreel.session.play'
const ROUTE_KEY = 'lanreel.session.playerRoute'
const KEY_TTL = 12 * 60 * 60 * 1000 // 半天后不再恢复

export interface FeedSession {
  sessionKey: string
  items: MediaItem[]
  activeIndex: number
  /** media key -> 播放秒数 */
  positions: Record<string, number>
  savedAt: number
}

export interface PlaySession {
  src: 'favorites' | 'history'
  items: MediaItem[]
  activeIndex: number
  positions: Record<string, number>
  savedAt: number
}

function read<T>(key: string): T | null {
  try {
    const raw = sessionStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as T & { savedAt?: number }
    if (parsed.savedAt && Date.now() - parsed.savedAt > KEY_TTL) return null
    return parsed
  } catch {
    return null
  }
}

function write(key: string, value: unknown) {
  try {
    sessionStorage.setItem(key, JSON.stringify({ ...(value as object), savedAt: Date.now() }))
  } catch {
    /* 隐私模式 / 容量不足时跳过，不影响播放 */
  }
}

export function saveFeedSession(session: Omit<FeedSession, 'savedAt'>) {
  write(FEED_KEY, session)
}

export function loadFeedSession(sessionKey: string): FeedSession | null {
  const saved = read<FeedSession>(FEED_KEY)
  return saved?.sessionKey === sessionKey && Array.isArray(saved.items) && saved.items.length
    ? saved
    : null
}

export function clearFeedSession() {
  try {
    sessionStorage.removeItem(FEED_KEY)
  } catch {
    /* ignore */
  }
}

export function savePlaySession(session: Omit<PlaySession, 'savedAt'>) {
  write(PLAY_KEY, session)
}

export function loadPlaySession(src: 'favorites' | 'history'): PlaySession | null {
  const saved = read<PlaySession>(PLAY_KEY)
  return saved?.src === src && Array.isArray(saved.items) && saved.items.length ? saved : null
}

/** 播放器路由白名单：只记住这三个页面，避免把登录页等记住导致循环跳转。 */
const PLAYER_ROUTES = ['/', '/favorites/play', '/play']

export function rememberPlayerRoute(fullPath: string) {
  try {
    const path = fullPath.split('?')[0]
    if (PLAYER_ROUTES.includes(path)) sessionStorage.setItem(ROUTE_KEY, fullPath)
  } catch {
    /* ignore */
  }
}

/** 设置页 / 后台「返回前台」优先回到播放器；没有记录时回退到指定页面。 */
export function playerReturnPath(fallback = '/'): string {
  try {
    const saved = sessionStorage.getItem(ROUTE_KEY) ?? ''
    return PLAYER_ROUTES.includes(saved.split('?')[0]) ? saved : fallback
  } catch {
    return fallback
  }
}
