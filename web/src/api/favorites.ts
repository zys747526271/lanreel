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
  return request<{ data: { success: boolean } }>(`/favorites/${videoKey}`, { method: 'DELETE' })
}
