import { request } from './client'
import type { MediaItem } from './types'

export function listFavorites() {
  return request<{ data: MediaItem[] }>('/favorites')
}

export function addFavorite(videoKey: string) {
  return request<{ data: MediaItem }>('/favorites', {
    method: 'POST',
    body: { video_key: videoKey },
  })
}

export function removeFavorite(videoKey: string) {
  return request<{ data: { success: boolean } }>(`/favorites/${encodeURIComponent(videoKey)}`, { method: 'DELETE' })
}

/** 批量取消收藏（可包含被管理员屏蔽或文件丢失的条目）。 */
export function batchRemoveFavorites(videoKeys: string[]) {
  return request<{ data: { removed: number } }>('/favorites/batch-remove', {
    method: 'POST',
    body: { video_keys: videoKeys },
  })
}

/** 一键清理文件已丢失的收藏；被管理员屏蔽的文件不会被删。 */
export function cleanMissingFavorites() {
  return request<{ data: { removed: number } }>('/favorites/clean-missing', { method: 'POST' })
}
