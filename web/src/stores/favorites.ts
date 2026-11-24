import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as favApi from '@/api/favorites'
import { getAll, putMany, clearStore } from '@/lib/db'
import { logger } from '@/lib/logger'
import type { MediaItem } from '@/api/types'

interface CachedFavorite {
  key: string
  rel_path: string
  media_type: string
  playable: boolean
  reason?: string
  cached_at: string
}

export const useFavoritesStore = defineStore('favorites', () => {
  const keys = ref<Set<string>>(new Set())
  const offline = ref(false)
  const loading = ref(false)
  const cachedAt = ref<string>('')

  const count = computed(() => keys.value.size)

  /** 先展示缓存，再请求后端覆盖（N-08）。 */
  async function load() {
    loading.value = true
    try {
      const cached = await getAll<CachedFavorite>('favorites')
      if (cached.length) {
        keys.value = new Set(cached.map((c) => c.key))
        cachedAt.value = cached[0].cached_at
      }
    } catch (err) {
      logger.warn('app', 'favorites', '读取收藏缓存失败', { error: String(err) })
    }
    try {
      const res = await favApi.listFavorites()
      keys.value = new Set(res.data.map((i) => i.key))
      offline.value = false
      cachedAt.value = new Date().toISOString()
      await clearStore('favorites')
      await putMany<CachedFavorite>(
        'favorites',
        res.data.map((i) => ({
          key: i.key,
          rel_path: i.rel_path,
          media_type: i.media_type,
          playable: i.playable,
          reason: i.reason,
          cached_at: new Date().toISOString(),
        })),
      )
    } catch (err) {
      offline.value = true
      logger.warn('app', 'favorites', '收藏同步失败，使用本地缓存', { error: String(err) })
    } finally {
      loading.value = false
    }
  }

  function has(key: string) {
    return keys.value.has(key)
  }

  /** 收藏/取消收藏：必须等待后端成功后再更新本地状态（F-21）。 */
  async function toggle(item: MediaItem): Promise<boolean> {
    if (keys.value.has(item.key)) {
      await favApi.removeFavorite(item.key)
      const next = new Set(keys.value)
      next.delete(item.key)
      keys.value = next
      logger.info('interaction', 'favorites', '取消收藏', { video_key: item.key })
      return false
    }
    await favApi.addFavorite(item.key)
    const next = new Set(keys.value)
    next.add(item.key)
    keys.value = next
    logger.info('interaction', 'favorites', '添加收藏', { video_key: item.key })
    return true
  }

  function reset() {
    keys.value = new Set()
    cachedAt.value = ''
  }

  return { keys, count, offline, loading, cachedAt, load, has, toggle, reset }
})
