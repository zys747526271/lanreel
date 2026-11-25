<script setup lang="ts">
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReelSlide from '@/components/reel/ReelSlide.vue'
import { listFavorites, removeFavorite } from '@/api/favorites'
import { useAuthStore } from '@/stores/auth'
import { useFavoritesStore } from '@/stores/favorites'
import { formatDuration, reasonText } from '@/lib/format'
import { prefs, exitFullscreen } from '@/lib/prefs'
import { logger } from '@/lib/logger'
import { pauseAllVideos } from '@/lib/playerRegistry'
import {
  loadPlaySession,
  rememberPlayerRoute,
  savePlaySession,
} from '@/lib/session'
import {
  flush as flushProgress,
  loadProgressMap,
  progressOf,
  restart as restartProgress,
  resumePosition,
  trackProgress,
} from '@/lib/progress'
import { listHistory } from '@/api/watch'
import type { HistoryEntryView, MediaItem, ProgressView } from '@/api/types'

// KeepAlive 按组件名缓存；名称必须与 App.vue 的 KEPT_VIEWS 一致。
defineOptions({ name: 'FavoritesPlayView' })

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const favorites = useFavoritesStore()
/** 播放列表来源：收藏夹或观看历史（KeepAlive 下必须响应 query 变化） */
const src = computed<'history' | 'favorites'>(() => (route.query.src === 'history' ? 'history' : 'favorites'))

const scroller = ref<HTMLElement | null>(null)
const items = ref<MediaItem[]>([])
const activeIndex = ref(0)
const loading = ref(true)
const muted = ref(prefs.defaultMuted)
const volume = ref(1)
const rate = computed(() => prefs.defaultRate)
const endAction = computed(() => prefs.endAction)
const toast = ref('')
const positions = ref<Record<string, number>>({})
const pendingResumeTime = ref<number | null>(null)
/** 当前条目是否有可续播进度（用于显示「从头开始」按钮）。 */
const canRestart = ref(false)
const immersive = ref(false)
/** KeepAlive 切走后为 false，阻止后台页面自动播放（v0.3.1）。 */
const viewActive = ref(true)
/** KeepAlive 组件只在首次挂载，后续 query 变化需手动重新聚焦条目。 */
let routeReady = false
let lastRouteSignature = ''
let toastTimer: number | null = null

type SlideInstance = InstanceType<typeof ReelSlide>
const slideRefs = new Map<string, SlideInstance>()
function setSlideRef(el: unknown, key: string) {
  if (el) slideRefs.set(key, el as SlideInstance)
  else slideRefs.delete(key)
}
const currentItem = computed(() => items.value[activeIndex.value])
const activeSlide = computed(() => slideRefs.get(currentItem.value?.key ?? ''))
/** 需求 7：收藏按钮显示当前视频收藏数。 */
const favoriteCount = computed(() => currentItem.value?.favorite_count ?? 0)

/** 视频信息条显示实际文件夹名；根目录文件显示媒体源名称。 */
function folderLabel(item: { folder?: string; root_name?: string }) {
  if (item.folder) return item.folder.split('/').filter(Boolean).pop() ?? item.folder
  return item.root_name || '根目录'
}

function setImmersive(on: boolean) {
  immersive.value = on
  document.body.classList.toggle('reel-immersive', on)
}
function toggleImmersive() {
  setImmersive(!immersive.value)
}

function onVolumeChange(payload: { volume: number; muted: boolean }) {
  volume.value = payload.volume
  muted.value = payload.muted
}

function toggleMute() {
  muted.value = !muted.value
  if (!muted.value && volume.value <= 0) volume.value = 1
}

function showToast(message: string) {
  toast.value = message
  if (toastTimer) window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = ''), 2200)
}

function slideHeight() {
  return scroller.value?.clientHeight || window.innerHeight
}

function scrollToIndex(index: number, smooth = true) {
  const el = scroller.value
  if (!el) return
  el.scrollTo({ top: index * slideHeight(), behavior: smooth ? 'smooth' : 'auto' })
}

/** 进入某条：计算续播位置与「从头开始」按钮。 */
function applyResume(key: string) {
  const p: ProgressView | undefined = progressOf(key)
  // 看完的从头；未看完且有进度则续播，并显示「从头开始」。
  const resume = resumePosition(key)
  pendingResumeTime.value = resume || null
  canRestart.value = !!p && !p.completed && p.progress > 0.02 && p.position > 1
}

function onScroll() {
  const el = scroller.value
  if (!el) return
  const index = Math.round(el.scrollTop / slideHeight())
  if (index !== activeIndex.value) {
    flushCurrent()
    activeIndex.value = index
    const item = items.value[index]
    if (item) applyResume(item.key)
  }
}

function next() {
  if (activeIndex.value < items.value.length - 1) scrollToIndex(activeIndex.value + 1)
}

function prev() {
  if (activeIndex.value > 0) scrollToIndex(activeIndex.value - 1)
}

function flushCurrent() {
  const item = currentItem.value
  if (!item || !auth.isLoggedIn) return
  const t = activeSlide.value?.getCurrentTime()
  if (t != null) {
    positions.value[item.key] = t
    void flushProgress(item.key, t, item.duration || 0, item)
  }
}

let seekTimer: number | null = null
let seekElapsed = 0
function stopSeek() {
  if (seekTimer !== null) window.clearInterval(seekTimer)
  seekTimer = null
  seekElapsed = 0
}
function startSeek(dir: 1 | -1) {
  stopSeek()
  activeSlide.value?.seekBy(dir * 3)
  seekTimer = window.setInterval(() => {
    seekElapsed += 120
    const step = seekElapsed > 1200 ? 8 : seekElapsed > 600 ? 5 : 3
    activeSlide.value?.seekBy(dir * step)
  }, 120)
}

function onKeydown(event: KeyboardEvent) {
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes((event.target as HTMLElement)?.tagName)) return
  switch (event.key) {
    case 'ArrowDown':
    case 'PageDown':
      event.preventDefault()
      next()
      break
    case 'ArrowUp':
    case 'PageUp':
      event.preventDefault()
      prev()
      break
    case 'ArrowRight':
      event.preventDefault()
      if (!event.repeat) startSeek(1)
      break
    case 'ArrowLeft':
      event.preventDefault()
      if (!event.repeat) startSeek(-1)
      break
    case ' ':
      event.preventDefault()
      activeSlide.value?.togglePlay()
      break
    case 'Escape':
      if (immersive.value) setImmersive(false)
      else back()
      break
    default:
      break
  }
}

function onKeyup(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') stopSeek()
}

function onProgress(payload: {
  key: string
  current: number
  duration: number
  started: boolean
  justStarted: boolean
  watchedDelta: number
}) {
  positions.value[payload.key] = payload.current
  // 预加载邻居不写历史；只统计当前真实播放的条目。
  if (auth.isLoggedIn && payload.key === items.value[activeIndex.value]?.key) {
    trackProgress(payload, items.value.find((i) => i.key === payload.key))
  }
}

/** 从头开始：清零服务端进度并 seek 到 0（需求 2）。 */
async function restartFromBeginning() {
  const item = currentItem.value
  if (!item) return
  await restartProgress(item.key)
  positions.value[item.key] = 0
  pendingResumeTime.value = 0
  canRestart.value = false
  activeSlide.value?.seekTo(0)
  showToast('已从头开始播放')
}

/** 视频自然结束且 endAction=next 时切到下一条；最后一条重播，避免全屏黑屏。 */
function onEnded(_key: string) {
  if (prefs.endAction !== 'next') return
  if (activeIndex.value < items.value.length - 1) next()
  else activeSlide.value?.replayFromStart()
}

function saveSession() {
  if (loading.value || !items.value.length) return
  flushCurrent()
  savePlaySession({
    src: src.value,
    items: items.value,
    activeIndex: activeIndex.value,
    positions: { ...positions.value },
  })
}

async function unfavorite() {
  const item = currentItem.value
  if (!item) return
  try {
    await removeFavorite(item.key)
    void favorites.load()
    const removedIndex = activeIndex.value
    items.value = items.value.filter((i) => i.key !== item.key)
    if (!items.value.length) {
      back()
      return
    }
    await nextTick()
    const target = Math.min(removedIndex, items.value.length - 1)
    activeIndex.value = target
    scrollToIndex(target, false)
    showToast('已取消收藏')
  } catch (err) {
    showToast(err instanceof Error ? err.message : '操作失败')
  }
}

/** 双击点赞：收藏/取消收藏，收藏来源下取消会同步移出播放列表（v0.3.1）。 */
async function onToggleFavorite(key: string) {
  const item = items.value.find((i) => i.key === key)
  if (!item) return
  try {
    const nowFavorite = await favorites.toggle(item)
    item.favorite = nowFavorite
    item.favorite_count = Math.max(0, (item.favorite_count ?? 0) + (nowFavorite ? 1 : -1))
    if (src.value === 'favorites' && !nowFavorite) {
      const removedIndex = activeIndex.value
      items.value = items.value.filter((i) => i.key !== key)
      if (!items.value.length) {
        back()
        return
      }
      await nextTick()
      const target = Math.min(removedIndex, items.value.length - 1)
      activeIndex.value = target
      scrollToIndex(target, false)
    }
    showToast(nowFavorite ? '已收藏' : '已取消收藏')
  } catch (err) {
    showToast(err instanceof Error ? err.message : '收藏失败')
  }
}

function back() {
  activeSlide.value?.exitFullscreenOwned()
  void exitFullscreen()
  router.replace(src.value === 'history' ? '/history' : '/favorites')
}

function toMedia(h: HistoryEntryView): MediaItem {
  return {
    key: h.key,
    media_type: h.media_type,
    root_index: h.root_index ?? 0,
    root_name: h.root_name,
    rel_path: h.rel_path,
    folder: h.folder,
    duration: h.duration,
    size: 0,
    playable: h.playable,
    reason: h.playable ? undefined : h.reason ?? 'unavailable',
    favorite: favorites.has(h.key),
    favorite_count: h.favorite_count,
    stream_url: h.stream_url,
    thumb_url: h.thumb_url,
  }
}

/** 拉取当前来源的最新列表（历史需要实时反映刚播放的条目）。 */
async function fetchList(): Promise<MediaItem[]> {
  await loadProgressMap()
  if (src.value === 'history') {
    const res = await listHistory(100)
    return (res.data as HistoryEntryView[]).map(toMedia)
  }
  const res = await listFavorites()
  return res.data as MediaItem[]
}

function currentRouteSignature() {
  return `${src.value}:${String(route.query.key ?? '')}`
}

/** KeepAlive 下 query 变化：重新加载列表并聚焦到新 key，避免继续播放旧视频。 */
async function focusRouteKey() {
  const key = String(route.query.key ?? '')
  if (!key) return
  flushCurrent()
  setImmersive(false)
  pauseAllVideos()
  loading.value = true
  try {
    items.value = await fetchList()
    const idx = items.value.findIndex((i) => i.key === key)
    if (idx >= 0) {
      activeIndex.value = idx
      applyResume(key)
    }
  } catch (err) {
    showToast(err instanceof Error ? err.message : '加载列表失败')
  } finally {
    loading.value = false
    await nextTick()
    scrollToIndex(activeIndex.value, false)
    if (!activeSlide.value?.isPaused()) activeSlide.value?.resume()
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('keyup', onKeyup)
  window.addEventListener('blur', stopSeek)
  window.addEventListener('resize', onResize)
  rememberPlayerRoute(route.fullPath)
  pauseAllVideos()
  if (!auth.isLoggedIn) {
    router.replace({ path: '/login', query: { redirect: '/favorites' } })
    return
  }
  if (!favorites.keys.size && !favorites.loading) void favorites.load()
  const saved = loadPlaySession(src.value)
  try {
    items.value = await fetchList()
    const startKey = String(route.query.key ?? '')
    const idx = startKey ? items.value.findIndex((i) => i.key === startKey) : -1
    if (idx >= 0) {
      activeIndex.value = idx
    } else if (saved) {
      activeIndex.value = Math.min(saved.activeIndex, Math.max(0, items.value.length - 1))
      positions.value = { ...saved.positions }
    }
    const item = items.value[activeIndex.value]
    if (item) applyResume(item.key)
  } catch (err) {
    showToast(err instanceof Error ? err.message : '加载列表失败')
  } finally {
    loading.value = false
  }
  await nextTick()
  scrollToIndex(activeIndex.value, false)
  if (prefs.autoFullscreen) void activeSlide.value?.toggleFullscreen(prefs.autoRotate)
  routeReady = true
  lastRouteSignature = currentRouteSignature()
  logger.info('app', 'player', '进入列表播放', { src: src.value, count: items.value.length, resume: pendingResumeTime.value })
})

/** 监听 query 变化（KeepAlive 组件不会重新 mount）。 */
watch(currentRouteSignature, (sig, old) => {
  if (!routeReady || sig === old) return
  lastRouteSignature = sig
  rememberPlayerRoute(route.fullPath)
  void focusRouteKey()
})

/** 横竖屏切换后重新对齐当前条目。 */
let resizeTimer: number | null = null
function onResize() {
  if (resizeTimer !== null) window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(() => {
    scrollToIndex(activeIndex.value, false)
    resizeTimer = null
  }, 120)
}

/** KeepAlive：切走只暂停、切回复用同一个 video 续播。 */
let resumeOnActivate = false
function onViewDeactivated() {
  viewActive.value = false
  resumeOnActivate = !activeSlide.value?.isPaused()
  setImmersive(false)
  saveSession()
  pauseAllVideos()
}
function onViewActivated() {
  viewActive.value = true
  rememberPlayerRoute(route.fullPath)
  if (routeReady && lastRouteSignature !== currentRouteSignature()) {
    lastRouteSignature = currentRouteSignature()
    void focusRouteKey()
    return
  }
  void nextTick(() => {
    scrollToIndex(activeIndex.value, false)
    if (resumeOnActivate) activeSlide.value?.resume()
    resumeOnActivate = false
  })
}
onDeactivated(onViewDeactivated)
onActivated(onViewActivated)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('keyup', onKeyup)
  window.removeEventListener('blur', stopSeek)
  window.removeEventListener('resize', onResize)
  if (resizeTimer !== null) window.clearTimeout(resizeTimer)
  stopSeek()
  setImmersive(false)
  saveSession()
  pauseAllVideos()
  void exitFullscreen()
  if (toastTimer) window.clearTimeout(toastTimer)
})
</script>

<template>
  <div class="fixed inset-0 flex flex-col bg-black text-white">
    <header
      class="player-chrome pointer-events-none absolute inset-x-0 top-0 z-30 flex items-start justify-between p-3 pt-[max(0.75rem,env(safe-area-inset-top))]"
    >
      <div class="pointer-events-auto flex items-center gap-2">
        <button
          class="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/55 backdrop-blur transition hover:bg-black/75"
          :title="src === 'history' ? '返回观看历史' : '返回收藏夹'"
          aria-label="返回列表"
          @click="back"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
        <span class="rounded-full bg-black/50 px-2.5 py-1 text-[11px] text-white/80 backdrop-blur">
          {{ src === 'history' ? '历史' : '收藏' }} · {{ activeIndex + 1 }}/{{ items.length }}
        </span>
      </div>
      <div class="pointer-events-auto flex items-center gap-1.5">
        <router-link
          :to="src === 'history' ? '/favorites' : '/history'"
          class="flex h-9 items-center rounded-full border border-white/20 bg-black/55 px-3 text-xs text-white/85 backdrop-blur transition hover:bg-black/75"
        >
          {{ src === 'history' ? '收藏夹' : '观看历史' }}
        </router-link>
      </div>
    </header>

    <div
      ref="scroller"
      class="no-scrollbar h-full w-full snap-y snap-mandatory overflow-y-scroll overscroll-none"
      @scroll.passive="onScroll"
    >
      <section
        v-for="(item, index) in items"
        :key="item.key"
        class="relative h-full w-full snap-start snap-always"
      >
        <ReelSlide
          :ref="(el) => setSlideRef(el, item.key)"
          :item="item"
          :active="index === activeIndex"
          :preload="Math.abs(index - activeIndex) <= 1"
          :image-duration="5"
          :muted="muted"
          :volume="volume"
          :rate="rate"
          :resume-time="index === activeIndex ? pendingResumeTime : null"
          :end-action="endAction"
          :immersive="immersive"
          :view-active="viewActive"
          @toggle-immersive="toggleImmersive"
          @toggle-favorite="onToggleFavorite"
          @volume-change="onVolumeChange"
          @autoplay-muted="showToast('已静音播放，点击画面开启声音')"
          @advance="next"
          @retreat="prev"
          @blocked="(r) => showToast(reasonText(r))"
          @toggle-mute="toggleMute"
          @progress="onProgress"
          @ended="onEnded"
        />
      </section>
    </div>

    <div v-if="loading" class="absolute inset-0 z-20 flex items-center justify-center bg-black/80">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-white/25 border-t-white/80"></div>
    </div>

    <div
      v-if="!loading && !items.length"
      class="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 bg-black/92 px-8 text-center"
    >
      <p class="text-sm text-white/70">{{ src === 'history' ? '观看历史已空' : '收藏夹已空' }}</p>
      <button class="rounded-full bg-white px-4 py-2 text-sm font-medium text-black" @click="back">
        返回列表
      </button>
    </div>

    <!-- 从头开始提示（有续播进度时出现，需求 2） -->
    <transition name="fade">
      <button
        v-if="canRestart && currentItem && !loading"
        class="player-chrome absolute left-1/2 top-20 z-30 flex -translate-x-1/2 items-center gap-1.5 rounded-full border border-white/20 bg-black/70 px-3.5 py-1.5 text-xs text-white/90 backdrop-blur transition hover:bg-black/85"
        @click="restartFromBeginning"
      >
        <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9">
          <path d="M3 12a9 9 0 1 0 3-6.7M3 4v5h5" />
        </svg>
        从头开始
      </button>
    </transition>

    <!-- 右侧操作：上一个 / 取消收藏 / 下一个 -->
    <div v-if="currentItem && !loading" class="player-chrome absolute bottom-[7.5rem] right-3 z-30 flex flex-col items-center gap-3.5">
      <button class="flex flex-col items-center gap-0.5" title="上一个（↑）" @click="prev">
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur active:scale-90">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="m6 15 6-6 6 6" />
          </svg>
        </span>
        <span class="text-[10px] text-white/65">上一个</span>
      </button>
      <button v-if="src === 'favorites'" class="group flex flex-col items-center gap-0.5" @click="unfavorite">
        <span class="flex h-12 w-12 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur">
          <svg class="h-6 w-6 text-reel-accent" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.6">
            <path d="M12 20s-7-4.4-7-9.2A4.3 4.3 0 0 1 12 7.6a4.3 4.3 0 0 1 7 3.2C19 15.6 12 20 12 20Z" />
          </svg>
        </span>
        <span class="text-[10px] text-white/65">{{ favoriteCount }}</span>
      </button>
      <button class="flex flex-col items-center gap-0.5" title="下一个（↓）" @click="next">
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur active:scale-90">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </span>
        <span class="text-[10px] text-white/65">下一个</span>
      </button>
    </div>

    <div v-if="currentItem" class="player-chrome pointer-events-none absolute bottom-[6.2rem] left-4 z-20 max-w-[60%] space-y-0.5">
      <p class="truncate text-sm font-medium text-white/95 drop-shadow">{{ currentItem.rel_path.split('/').pop() }}</p>
      <p class="truncate text-[11px] text-white/65 drop-shadow">
        {{ folderLabel(currentItem) }} ·
        {{ currentItem.media_type === 'video' ? `视频 ${formatDuration(currentItem.duration)}` : '图片' }}
      </p>
      <p v-if="!currentItem.playable" class="text-[11px] text-amber-300/90">
        {{ reasonText(currentItem.reason) }}
      </p>
    </div>

    <transition name="fade">
      <div
        v-if="toast"
        class="pointer-events-none absolute left-1/2 top-20 z-40 -translate-x-1/2 rounded-full bg-black/80 px-4 py-2 text-sm text-white shadow-lg backdrop-blur"
      >
        {{ toast }}
      </div>
    </transition>
  </div>
</template>

<style scoped>
.no-scrollbar {
  scrollbar-width: none;
  -ms-overflow-style: none;
}
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
