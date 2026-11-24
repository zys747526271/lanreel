/** 观看历史（仅存本机 IndexedDB，最多 100 条，按最近观看排序）。 */
import { countAll, deleteItem, getAll, putItem } from './db'
import type { MediaItem, MediaType } from '@/api/types'

const MAX_ENTRIES = 100
const KEY = 'lanreel.watched'

export interface HistoryEntry {
  key: string
  media_type: MediaType
  rel_path: string
  folder: string
  root_name: string
  thumb_url: string
  stream_url: string
  duration: number
  playable: boolean
  played_at: string
}

export async function recordHistory(item: MediaItem): Promise<void> {
  if (!item?.key) return
  const entry: HistoryEntry = {
    key: item.key,
    media_type: item.media_type,
    rel_path: item.rel_path,
    folder: item.folder,
    root_name: item.root_name ?? '',
    thumb_url: item.thumb_url,
    stream_url: item.stream_url,
    duration: item.duration,
    playable: item.playable,
    played_at: new Date().toISOString(),
  }
  try {
    await putItem('history', entry)
    await trim()
  } catch {
    /* 历史记录失败不影响播放 */
  }
}

async function trim(): Promise<void> {
  const total = await countAll('history')
  if (total <= MAX_ENTRIES) return
  const all = await getAll<HistoryEntry>('history')
  const remove = all
    .sort((a, b) => (a.played_at < b.played_at ? -1 : 1))
    .slice(0, total - MAX_ENTRIES)
  for (const e of remove) await deleteItem('history', e.key)
}

export async function listHistory(limit = 60): Promise<HistoryEntry[]> {
  const all = await getAll<HistoryEntry>('history')
  return all.sort((a, b) => (a.played_at < b.played_at ? 1 : -1)).slice(0, limit)
}

export async function clearHistory(): Promise<void> {
  const all = await getAll<HistoryEntry>('history')
  for (const e of all) await deleteItem('history', e.key)
}

export async function removeHistory(key: string): Promise<void> {
  await deleteItem('history', key)
}
