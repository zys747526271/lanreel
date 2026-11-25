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

/** 批量刷新条目的实时权限（收藏/历史存的是加入时的旧快照）。 */
export interface MediaStatus {
  key: string
  playable: boolean
  reason?: string
}

export function fetchMediaStatus(keys: string[]) {
  return request<{ data: MediaStatus[] }>('/media/status', {
    method: 'POST',
    body: { keys },
    quiet: true,
  })
}

/** 批量重查全部条目；服务端单次上限 200，这里自动分片去重。 */
export async function fetchAllMediaStatus(keys: string[]): Promise<MediaStatus[]> {
  const unique = [...new Set(keys.map((k) => k.trim()).filter(Boolean))]
  const out: MediaStatus[] = []
  for (let i = 0; i < unique.length; i += 200) {
    const res = await fetchMediaStatus(unique.slice(i, i + 200))
    out.push(...res.data)
  }
  return out
}
