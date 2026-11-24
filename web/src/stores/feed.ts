import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { fetchRandom, type FeedMode } from '@/api/media'
import { logger } from '@/lib/logger'
import type { MediaItem } from '@/api/types'

export const useFeedStore = defineStore('feed', () => {
  const items = ref<MediaItem[]>([])
  const mode = ref<FeedMode>('all')
  const loading = ref(false)
  const error = ref('')
  const hasMore = ref(true)

  const isEmpty = computed(() => !loading.value && items.value.length === 0)

  async function loadMore(count = 5, extraExclude: string[] = []) {
    if (loading.value || !hasMore.value) return
    loading.value = true
    error.value = ''
    const started = performance.now()
    try {
      // 刷新随机流时排除当前整批，避免只拿到同一批视频看起来像「没刷新」。
      // 常规上拉加载只排除末尾几条，保留重复观看的机会。
      const exclude = [...new Set([
        ...(extraExclude.length ? extraExclude : items.value.slice(-6).map((i) => i.key)),
      ])]
      const res = await fetchRandom({ limit: count, exclude, mode: mode.value })
      const stamp = Date.now()
      const fresh = res.data
        .filter((i) => !exclude.includes(i.key) || mode.value === 'favorites')
        .map((i, idx) => ({ ...i, uid: `${i.key}-${stamp}-${idx}` }))
      if (!fresh.length) {
        hasMore.value = false
        logger.info('app', 'feed', '没有更多可播放媒体', { mode: mode.value })
      } else {
        items.value.push(...fresh)
      }
      logger.debug('performance', 'feed', '加载媒体批次', {
        count: fresh.length,
        duration_ms: Math.round(performance.now() - started),
        mode: mode.value,
      })
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载失败'
      logger.error('app', 'feed', '加载随机媒体失败', { error: error.value })
    } finally {
      loading.value = false
    }
  }

  function reset() {
    items.value = []
    hasMore.value = true
    error.value = ''
  }

  /** 从会话缓存恢复上一次的随机流（返回设置/后台时保持同一批视频与顺序）。 */
  function restore(restored: typeof items.value) {
    items.value = restored
    loading.value = false
    hasMore.value = true
    error.value = ''
  }

  async function setMode(next: FeedMode) {
    if (mode.value === next) return
    mode.value = next
    reset()
    logger.info('interaction', 'feed', '切换浏览模式', { mode: next })
    await loadMore(8)
  }

  function patchFavorite(key: string, favorite: boolean) {
    items.value = items.value.map((i) => (i.key === key ? { ...i, favorite } : i))
  }

  function applyFavoriteKeys(keys: Set<string>) {
    items.value = items.value.map((i) => ({ ...i, favorite: keys.has(i.key) }))
  }

  /** 收藏模式下取消收藏后，移除该条目避免残留在流中。 */
  function removeKey(key: string) {
    items.value = items.value.filter((i) => i.key !== key)
  }

  return { items, mode, loading, error, hasMore, isEmpty, loadMore, reset, restore, setMode, patchFavorite, applyFavoriteKeys, removeKey }
})
