import { request } from './client'
import type { MediaItem } from './types'

export type FeedMode = 'all' | 'favorites'

export function fetchRandom(params: {
  limit?: number
  exclude?: string[]
  mode?: FeedMode
  signal?: AbortSignal
}) {
  const search = new URLSearchParams()
  search.set('limit', String(params.limit ?? 5))
  if (params.mode === 'favorites') search.set('mode', 'favorites')
  if (params.exclude?.length) search.set('exclude', params.exclude.join(','))
  return request<{ data: MediaItem[] }>(`/media/random?${search.toString()}`, {
    signal: params.signal,
  })
}
