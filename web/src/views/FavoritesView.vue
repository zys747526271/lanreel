<script setup lang="ts">
import { computed, onActivated, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listFavorites } from '@/api/favorites'
import { withToken } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { formatDuration, reasonText } from '@/lib/format'
import { logger } from '@/lib/logger'
import type { MediaItem } from '@/api/types'

const router = useRouter()
const auth = useAuthStore()

const items = ref<MediaItem[]>([])
const loading = ref(true)
const error = ref('')
/** 从播放页返回时恢复滚动位置 */
const SCROLL_KEY = 'lanreel.favorites.scroll'

const playableCount = computed(() => items.value.filter((i) => i.playable).length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listFavorites()
    items.value = res.data
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载收藏失败'
    logger.warn('app', 'favorites', '加载收藏列表失败', { error: error.value })
  } finally {
    loading.value = false
  }
}

function open(item: MediaItem) {
  if (!item.playable) {
    error.value = reasonText(item.reason)
    window.setTimeout(() => (error.value = ''), 2600)
    return
  }
  const el = document.scrollingElement ?? document.documentElement
  sessionStorage.setItem(SCROLL_KEY, String(el.scrollTop))
  router.push({ path: '/favorites/play', query: { key: item.key } })
}

function restoreScroll() {
  const saved = Number(sessionStorage.getItem(SCROLL_KEY) ?? 0)
  if (!saved) return
  requestAnimationFrame(() => window.scrollTo({ top: saved }))
}

onMounted(async () => {
  if (!auth.isLoggedIn) {
    router.replace({ path: '/login', query: { redirect: '/favorites' } })
    return
  }
  await load()
  restoreScroll()
})

// 保留 keep-alive 场景（后续若启用页面缓存）下的刷新能力。
onActivated(() => {
  if (items.value.length) void load()
})
</script>

<template>
  <div class="min-h-[100dvh] bg-[#08080a] pb-10 text-white">
    <header
      class="sticky top-0 z-20 flex items-center justify-between border-b border-white/8 bg-[#08080a]/90 px-4 py-3 backdrop-blur"
    >
      <div class="flex items-center gap-2">
        <button
          class="rounded-full border border-white/12 p-2 transition hover:bg-white/10"
          title="返回浏览"
          @click="router.push('/')"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
        <div>
          <h1 class="text-base font-semibold">收藏夹</h1>
          <p class="text-[11px] text-white/40">
            {{ items.length }} 条 · 可播放 {{ playableCount }} 条
          </p>
        </div>
      </div>
      <button
        class="rounded-full border border-white/12 px-3 py-1.5 text-xs text-white/70 transition hover:bg-white/10"
        @click="load"
      >
        刷新
      </button>
    </header>

    <p v-if="error" class="mx-4 mt-3 rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
      {{ error }}
    </p>

    <div v-if="loading" class="flex justify-center py-16 text-white/50">
      <div class="h-7 w-7 animate-spin rounded-full border-2 border-white/20 border-t-white/80"></div>
    </div>

    <div v-else-if="!items.length" class="flex flex-col items-center gap-3 px-8 py-24 text-center">
      <svg class="h-11 w-11 text-white/25" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4">
        <path d="M12 20s-7-4.4-7-9.2A4.3 4.3 0 0 1 12 7.6a4.3 4.3 0 0 1 7 3.2C19 15.6 12 20 12 20Z" />
      </svg>
      <p class="text-sm text-white/70">还没有收藏</p>
      <p class="max-w-[16rem] text-xs leading-relaxed text-white/40">
        浏览时点右侧的爱心，收藏的内容会出现在这里
      </p>
      <button
        class="mt-1 rounded-full bg-white px-4 py-2 text-sm font-medium text-black"
        @click="router.push('/')"
      >
        去看看
      </button>
    </div>

    <!-- 缩略图网格：点击才进入播放，避免全部加载视频 -->
    <div v-else class="grid grid-cols-3 gap-1.5 p-1.5 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6">
      <button
        v-for="item in items"
        :key="item.key"
        class="group relative aspect-[9/14] overflow-hidden rounded-lg bg-white/5 text-left"
        :title="item.rel_path"
        @click="open(item)"
      >
        <img
          :src="withToken(item.thumb_url)"
          class="h-full w-full object-cover transition duration-200 group-hover:scale-[1.03]"
          :class="item.playable ? '' : 'opacity-40 blur-[2px]'"
          loading="lazy"
          alt=""
          @error="($event.target as HTMLImageElement).style.display = 'none'"
        />
        <div
          v-if="!item.thumb_url"
          class="absolute inset-0 flex items-center justify-center bg-gradient-to-br from-slate-800 to-slate-950"
        >
          <svg class="h-7 w-7 text-white/30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <rect x="3" y="4.5" width="18" height="15" rx="2.5" />
            <path d="m10 9 5 3-5 3V9Z" />
          </svg>
        </div>

        <!-- 视频角标 -->
        <span
          v-if="item.media_type === 'video'"
          class="absolute left-1 top-1 rounded bg-black/60 px-1 py-0.5 text-[10px] text-white/90 backdrop-blur"
        >
          <svg class="inline h-3 w-3 -translate-y-px" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5.5v13l11-6.5z" />
          </svg>
          <span v-if="item.duration" class="ml-0.5">{{ formatDuration(item.duration) }}</span>
        </span>

        <!-- 不可播放 -->
        <span
          v-if="!item.playable"
          class="absolute inset-x-1 bottom-1 rounded bg-black/65 px-1 py-0.5 text-center text-[10px] text-amber-200 backdrop-blur"
        >
          {{ reasonText(item.reason) }}
        </span>

        <span
          class="pointer-events-none absolute inset-0 flex items-center justify-center opacity-0 transition group-hover:opacity-100"
        >
          <span class="rounded-full bg-black/45 p-2.5 backdrop-blur">
            <svg class="h-5 w-5 text-white" viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5.5v13l11-6.5z" />
            </svg>
          </span>
        </span>
      </button>
    </div>
  </div>
</template>
