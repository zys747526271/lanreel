<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReelSlide from '@/components/reel/ReelSlide.vue'
import { listFavorites, removeFavorite } from '@/api/favorites'
import { useAuthStore } from '@/stores/auth'
import { formatDuration, reasonText } from '@/lib/format'
import { prefs, exitFullscreen } from '@/lib/prefs'
import { listHistory, recordHistory, type HistoryEntry } from '@/lib/history'
import { logger } from '@/lib/logger'
import { pauseAllVideos } from '@/lib/playerRegistry'
import {
  loadPlaySession,
  rememberPlayerRoute,
  savePlaySession,
} from '@/lib/session'
import type { MediaItem } from '@/api/types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
/** 播放列表来源：收藏夹或观看历史 */
const src = route.query.src === 'history' ? 'history' : 'favorites'

const scroller = ref<HTMLElement | null>(null)
const items = ref<MediaItem[]>([])
const activeIndex = ref(0)
const loading = ref(true)
const muted = ref(prefs.defaultMuted)
const rate = computed(() => prefs.defaultRate)
const toast = ref('')
const positions = ref<Record<string, number>>({})
const pendingResumeTime = ref<number | null>(null)
let toastTimer: number | null = null
let historyTimer: number | null = null

type SlideInstance = InstanceType<typeof ReelSlide>
const slideRefs = new Map<number, SlideInstance>()
function setSlideRef(el: unknown, index: number) {
  if (el) slideRefs.set(index, el as SlideInstance)
  else slideRefs.delete(index)
}
const activeSlide = computed(() => slideRefs.get(activeIndex.value))
const currentItem = computed(() => items.value[activeIndex.value])
const favoriteCount = computed(() => items.value.filter((i) => i.playable).length)

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

function onScroll() {
  const el = scroller.value
  if (!el) return
  const index = Math.round(el.scrollTop / slideHeight())
  if (index !== activeIndex.value) {
    activeIndex.value = index
    pendingResumeTime.value = null
    scheduleHistory()
  }
}

function next() {
  if (activeIndex.value < items.value.length - 1) scrollToIndex(activeIndex.value + 1)
}

function prev() {
  if (activeIndex.value > 0) scrollToIndex(activeIndex.value - 1)
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
      back()
      break
    default:
      break
  }
}

function onKeyup(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') stopSeek()
}

function onProgress(payload: { key: string; current: number }) {
  positions.value[payload.key] = payload.current
}

function saveSession() {
  if (loading.value || !items.value.length) return
  const item = items.value[activeIndex.value]
  if (item) {
    const t = activeSlide.value?.getCurrentTime()
    if (t != null) positions.value[item.key] = t
  }
  savePlaySession({
    src,
    items: items.value,
    activeIndex: activeIndex.value,
    positions: { ...positions.value },
  })
}

function scheduleHistory() {
  if (!auth.isLoggedIn) return
  if (historyTimer) window.clearTimeout(historyTimer)
  historyTimer = window.setTimeout(() => {
    const item = items.value[activeIndex.value]
    if (item) void recordHistory(item)
  }, 1500)
}

async function unfavorite() {
  const item = currentItem.value
  if (!item) return
  try {
    await removeFavorite(item.key)
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

function back() {
  void exitFullscreen()
  router.replace(src === 'history' ? '/history' : '/favorites')
}

function toMedia(h: HistoryEntry): MediaItem {
  return {
    key: h.key,
    media_type: h.media_type,
    root_index: 0,
    root_name: h.root_name,
    rel_path: h.rel_path,
    folder: h.folder,
    duration: h.duration,
    size: 0,
    playable: h.playable,
    reason: h.playable ? undefined : 'unavailable',
    favorite: false,
    stream_url: h.stream_url,
    thumb_url: h.thumb_url,
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('keyup', onKeyup)
  window.addEventListener('blur', stopSeek)
  // 刷新清会话由 main.ts 全局处理；这里仅在 SPA 路由离开时保存（onBeforeUnmount）。
  rememberPlayerRoute(route.fullPath)
  pauseAllVideos()
  if (!auth.isLoggedIn) {
    router.replace({ path: '/login', query: { redirect: '/favorites' } })
    return
  }
  const saved = loadPlaySession(src)
  try {
    if (src === 'history') {
      const list = await listHistory(100)
      items.value = list.map(toMedia)
    } else {
      const res = await listFavorites()
      items.value = res.data
    }
    const startKey = String(route.query.key ?? '')
    const idx = startKey ? items.value.findIndex((i) => i.key === startKey) : -1
    if (idx > 0 || (idx === 0 && !saved)) {
      activeIndex.value = idx
    } else if (saved) {
      activeIndex.value = Math.min(saved.activeIndex, Math.max(0, items.value.length - 1))
      positions.value = { ...saved.positions }
      pendingResumeTime.value = saved.positions[items.value[activeIndex.value]?.key ?? ''] ?? null
    }
  } catch (err) {
    showToast(err instanceof Error ? err.message : '加载列表失败')
  } finally {
    loading.value = false
  }
  await nextTick()
  scrollToIndex(activeIndex.value, false)
  scheduleHistory()
  // 沿用设置页的「从收藏/历史播放时自动全屏」偏好（浏览器可能因非手势拒绝，手动按钮始终可用）
  if (prefs.autoFullscreen) void activeSlide.value?.toggleFullscreen(prefs.autoRotate)
  logger.info('app', 'player', '进入列表播放', { src, count: items.value.length, restored: !!pendingResumeTime.value })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('keyup', onKeyup)
  window.removeEventListener('blur', stopSeek)
  stopSeek()
  saveSession()
  pauseAllVideos()
  void exitFullscreen()
  if (toastTimer) window.clearTimeout(toastTimer)
  if (historyTimer) window.clearTimeout(historyTimer)
})
</script>

<template>
  <div class="fixed inset-0 flex flex-col bg-black text-white">
    <header
      class="pointer-events-none absolute inset-x-0 top-0 z-30 flex items-start justify-between p-3 pt-[max(0.75rem,env(safe-area-inset-top))]"
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
          :ref="(el) => setSlideRef(el, index)"
          :item="item"
          :active="index === activeIndex"
          :preload="Math.abs(index - activeIndex) <= 1"
          :image-duration="5"
          :muted="muted"
          :rate="rate"
          :resume-time="index === activeIndex ? pendingResumeTime : null"
          @autoplay-muted="showToast('已静音播放，点击画面开启声音')"
          @advance="next"
          @retreat="prev"
          @blocked="(r) => showToast(reasonText(r))"
          @toggle-mute="muted = !muted"
          @progress="onProgress"
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

    <!-- 右侧操作：上一个 / 取消收藏 / 下一个 -->
    <div v-if="currentItem && !loading" class="absolute bottom-[7.5rem] right-3 z-30 flex flex-col items-center gap-3.5">
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

    <div v-if="currentItem" class="pointer-events-none absolute bottom-[6.2rem] left-4 z-20 max-w-[60%] space-y-0.5">
      <p class="truncate text-sm font-medium text-white/95 drop-shadow">{{ currentItem.rel_path.split('/').pop() }}</p>
      <p class="truncate text-[11px] text-white/65 drop-shadow">
        {{ currentItem.folder || '根目录' }} ·
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
