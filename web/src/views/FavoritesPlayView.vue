<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReelSlide from '@/components/reel/ReelSlide.vue'
import { listFavorites, removeFavorite } from '@/api/favorites'
import { useAuthStore } from '@/stores/auth'
import { formatDuration, reasonText } from '@/lib/format'
import { prefs, enterFullscreen, exitFullscreen } from '@/lib/prefs'
import { listHistory, recordHistory } from '@/lib/history'
import { logger } from '@/lib/logger'
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
let toastTimer: number | null = null
let historyTimer: number | null = null

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
    scheduleHistory()
  }
}

function next() {
  if (activeIndex.value < items.value.length - 1) scrollToIndex(activeIndex.value + 1)
}

function prev() {
  if (activeIndex.value > 0) scrollToIndex(activeIndex.value - 1)
}

function onKeydown(event: KeyboardEvent) {
  if (['INPUT', 'TEXTAREA'].includes((event.target as HTMLElement)?.tagName)) return
  if (event.key === 'ArrowDown' || event.key === 'PageDown' || event.key === ' ') {
    event.preventDefault()
    next()
  } else if (event.key === 'ArrowUp' || event.key === 'PageUp') {
    event.preventDefault()
    prev()
  } else if (event.key === 'Escape') {
    back()
  } else if (event.key === 'm' || event.key === 'M') {
    muted.value = !muted.value
  }
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
  router.replace(src === 'history' ? '/settings' : '/favorites')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  if (!auth.isLoggedIn) {
    router.replace({ path: '/login', query: { redirect: '/favorites' } })
    return
  }
  try {
    if (src === 'history') {
      const list = await listHistory(100)
      items.value = list.map((h) => ({
        key: h.key,
        media_type: h.media_type,
        root_index: 0,
        root_name: h.root_name,
        rel_path: h.rel_path,
        folder: h.folder,
        duration: h.duration,
        size: 0,
        playable: h.playable,
        favorite: false,
        stream_url: h.stream_url,
        thumb_url: h.thumb_url,
      }))
    } else {
      const res = await listFavorites()
      items.value = res.data
    }
    const startKey = String(route.query.key ?? '')
    const idx = items.value.findIndex((i) => i.key === startKey)
    activeIndex.value = idx > 0 ? idx : 0
  } catch (err) {
    showToast(err instanceof Error ? err.message : '加载收藏失败')
  } finally {
    loading.value = false
  }
  await nextTick()
  scrollToIndex(activeIndex.value, false)
  scheduleHistory()
  if (prefs.autoFullscreen) void enterFullscreen(prefs.autoRotate)
  logger.info('app', 'favorites', '进入收藏播放', { count: items.value.length })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
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
          class="rounded-full border border-white/15 bg-black/40 p-2 backdrop-blur transition hover:bg-black/60"
          title="返回收藏夹"
          @click="back"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
        <span class="rounded-full bg-black/40 px-2.5 py-1 text-[11px] text-white/75 backdrop-blur">
          {{ src === 'history' ? '历史' : '收藏' }} · {{ activeIndex + 1 }}/{{ items.length }}
        </span>
      </div>
      <div class="pointer-events-auto flex items-center gap-1.5">
        <button
          class="rounded-full border border-white/15 bg-black/40 p-2 backdrop-blur transition hover:bg-black/60"
          :title="muted ? '开启声音' : '静音'"
          @click="muted = !muted"
        >
          <svg v-if="muted" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M11 5 6 9H3v6h3l5 4V5Z" />
            <path d="m16 9 5 6m0-6-5 6" />
          </svg>
          <svg v-else class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M11 5 6 9H3v6h3l5 4V5Z" />
            <path d="M15.5 8.5a5 5 0 0 1 0 7M18 6a8.5 8.5 0 0 1 0 12" />
          </svg>
        </button>
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
          :item="item"
          :active="index === activeIndex"
          :preload="Math.abs(index - activeIndex) <= 1"
          :image-duration="5"
          :muted="muted"
          :rate="rate"
          @autoplay-muted="showToast('已静音播放，点击画面开启声音')"
          @advance="next"
          @blocked="(r) => showToast(reasonText(r))"
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
      <p class="text-sm text-white/70">收藏夹已空</p>
      <button class="rounded-full bg-white px-4 py-2 text-sm font-medium text-black" @click="back">
        返回收藏夹
      </button>
    </div>

    <!-- 右侧操作 -->
    <div v-if="currentItem && !loading" class="absolute bottom-44 right-3 z-30 flex flex-col items-center gap-5">
      <button class="group flex flex-col items-center gap-1" @click="unfavorite">
        <span class="flex h-12 w-12 items-center justify-center rounded-full border border-white/15 bg-black/40 backdrop-blur">
          <svg class="h-6 w-6 text-reel-accent" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.6">
            <path d="M12 20s-7-4.4-7-9.2A4.3 4.3 0 0 1 12 7.6a4.3 4.3 0 0 1 7 3.2C19 15.6 12 20 12 20Z" />
          </svg>
        </span>
        <span class="text-[11px] text-white/70">{{ favoriteCount }}</span>
      </button>
      <button class="flex flex-col items-center gap-1" @click="next">
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/15 bg-black/40 backdrop-blur">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </span>
        <span class="text-[11px] text-white/60">下一个</span>
      </button>
    </div>

    <div v-if="currentItem" class="pointer-events-none absolute bottom-24 left-4 z-20 max-w-[70%] space-y-1">
      <p class="truncate text-sm font-medium text-white/95">{{ currentItem.rel_path.split('/').pop() }}</p>
      <p class="truncate text-[11px] text-white/55">
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
        class="pointer-events-none absolute left-1/2 top-20 z-40 -translate-x-1/2 rounded-full bg-black/75 px-4 py-2 text-sm text-white shadow-lg backdrop-blur"
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
