/** 悬浮抽帧封面缓存：把客户端从视频里截出的帧存到 IndexedDB，按 media key 复用。 */
import { clearStore, deleteItem, getItem, putItem } from './db'

export interface CachedThumb {
  key: string
  /** JPEG data URL */
  data: string
  created_at: number
}

export async function getCachedThumb(key: string): Promise<string | null> {
  try {
    const entry = await getItem<CachedThumb>('thumbs', key)
    return entry?.data ?? null
  } catch {
    return null
  }
}

export async function saveCachedThumb(key: string, data: string): Promise<void> {
  try {
    await putItem<CachedThumb>('thumbs', { key, data, created_at: Date.now() })
  } catch {
    /* 容量不足时忽略：覆盖图仍可用 */
  }
}

export async function removeCachedThumb(key: string): Promise<void> {
  try {
    await deleteItem('thumbs', key)
  } catch {
    /* ignore */
  }
}

export async function clearThumbs(): Promise<void> {
  try {
    await clearStore('thumbs')
  } catch {
    /* ignore */
  }
}
