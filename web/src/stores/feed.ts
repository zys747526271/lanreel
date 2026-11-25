import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { fetchRandom, type FeedMode, type MediaStatus } from '@/api/media'
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

  /** 策略变更后刷新条目的可播放状态；禁用项保留在列表中但由视图隐藏，开放后可自动恢复。 */
  function patchPolicyStatuses(statuses: MediaStatus[]) {
    const map = new Map(statuses.map((s) => [s.key, s]))
    items.value = items.value.map((i) => {
      const s = map.get(i.key)
      if (!s) return i
      return { ...i, playable: s.playable, reason: s.playable ? undefined : s.reason }
    })
  }

  /**
   * 策略「开放」后把新变为可播放、但尚未进入过随机流的媒体并入列表。
   * 随机流只返回加载时可播放的条目，被管理员禁用的视频根本不在 items 里，
   * 因此只重查已加载 key 的状态无法发现它们；必须主动拉一批当前可播放的媒体。
   * 以全部已知 key 作为 exclude，重复调用会自然收敛，不会无限追加。
   */
  async function mergeAvailable(count = 20): Promise<number> {
    // 状态重查期间用户若滚动触发了 loadMore，先等它结束再取快照，避免并发写列表。
    let waited = 0
    while (loading.value && waited < 40) {
      await new Promise<void>((resolve) => window.setTimeout(resolve, 50))
      waited += 1
    }
    if (loading.value) return 0
    const exclude = items.value.map((i) => i.key)
    loading.value = true
    // 超时中止，避免单个请求挂起导致 loading 长期为 true、阻塞 loadMore 与后续策略同步。
    const controller = new AbortController()
    const timeout = window.setTimeout(() => controller.abort(), 10000)
    try {
      const res = await fetchRandom({ limit: count, exclude, mode: mode.value, signal: controller.signal })
      const known = new Set(items.value.map((i) => i.key))
      const stamp = Date.now()
      const fresh = res.data
        .filter((i) => !known.has(i.key))
        .map((i, idx) => ({ ...i, uid: `${i.key}-${stamp}-${idx}` }))
      if (fresh.length) items.value.push(...fresh)
      logger.debug('app', 'feed', '策略开放后并入可播放媒体', { added: fresh.length })
      return fresh.length
    } finally {
      window.clearTimeout(timeout)
      loading.value = false
    }
  }

  /** 策略开放后恢复继续加载能力：此前可能因空结果被标记为「没有更多」。 */
  function reopenHasMore() {
    hasMore.value = true
    error.value = ''
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

  return {
    items, mode, loading, error, hasMore, isEmpty, loadMore, reset, restore, setMode,
    patchPolicyStatuses, mergeAvailable, reopenHasMore, patchFavorite, applyFavoriteKeys, removeKey,
  }
})
