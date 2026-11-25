/** 观看历史、进度与统计 API（v0.3.0，服务端按用户隔离）。 */
import { request } from './client'
import type {
  HeatPage, HeatView, HistoryEntryView, ProgressView, UserHeatRow, WatchSummary,
} from './types'

export function listHistory(limit = 100) {
  return request<{ data: HistoryEntryView[] }>(`/history?limit=${limit}`)
}

export function clearHistory() {
  return request<{ data: { success: boolean } }>('/history', { method: 'DELETE' })
}

export function removeHistory(key: string) {
  return request<{ data: { success: boolean } }>(`/history/${encodeURIComponent(key)}`, {
    method: 'DELETE',
  })
}

/** 获取全部观看进度（key -> 进度）。 */
export function getProgress() {
  return request<{ data: Record<string, ProgressView> }>('/progress')
}

export interface ProgressReport {
  video_key: string
  rel_path?: string
  media_type?: string
  root_index?: number
  folder_path?: string
  duration?: number
  /** 当前播放秒数 */
  position: number
  /** 距上次上报新增的观看秒数 */
  watched_delta: number
  completed?: boolean
  restart?: boolean
  /** 本次会话首次真实播放该条（用于准确计播放次数） */
  first?: boolean
  /** 播放器确实触发过 play 事件；预加载/封面抽帧不能写历史 */
  started?: boolean
  /** 本次是刚开始播放，必须立即写历史，不能被 5 秒节流挡住 */
  justStarted?: boolean
}

export function reportProgress(payload: ProgressReport) {
  return request<{ data: { success: boolean } }>('/progress', {
    method: 'POST',
    body: payload,
    quiet: true,
  })
}

/** 从头开始播放：清零该视频的进度。 */
export function restartProgress(videoKey: string) {
  return request<{ data: { success: boolean } }>('/progress/restart', {
    method: 'POST',
    body: { video_key: videoKey },
  })
}

export function getMyWatchSummary() {
  return request<{ data: WatchSummary }>('/me/watch-summary')
}

// ---------- 管理端 ----------

export type HeatSortKey = 'heat' | 'play' | 'complete' | 'watched' | 'favorite' | 'name'
export type HeatOrder = 'asc' | 'desc'

export interface HeatQuery {
  page: number
  pageSize: number
  sort: HeatSortKey
  order: HeatOrder
}

function heatQuery(q: HeatQuery) {
  const params = new URLSearchParams({
    page: String(q.page),
    page_size: String(q.pageSize),
    sort: q.sort,
    order: q.order,
  })
  return params.toString()
}

/** 热度统计：服务端全量排序后再分页，避免只排当前页。 */
export function getMediaHeatPage(q: HeatQuery) {
  return request<HeatPage<HeatView>>(`/admin/watch/heat/media?${heatQuery(q)}`)
}

export function getFolderHeatPage(q: HeatQuery) {
  return request<HeatPage<HeatView>>(`/admin/watch/heat/folders?${heatQuery(q)}`)
}

export function getUserHeatPage(q: HeatQuery) {
  return request<HeatPage<UserHeatRow>>(`/admin/watch/heat/users?${heatQuery(q)}`)
}

/** 推荐候选（预留接口，基于热度评分）。 */
export function getRecommendations(limit = 20, exclude: string[] = []) {
  const q = new URLSearchParams({ limit: String(limit) })
  if (exclude.length) q.set('exclude', exclude.join(','))
  return request<{ data: Array<{ key: string; score: number; play_count: number; favorite_count: number }> }>(
    `/admin/recommend?${q.toString()}`,
  )
}
