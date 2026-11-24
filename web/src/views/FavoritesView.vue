<script setup lang="ts">
import { computed, onActivated, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import MediaGrid from '@/components/media/MediaGrid.vue'
import { listFavorites } from '@/api/favorites'
import { useAuthStore } from '@/stores/auth'
import { reasonText } from '@/lib/format'
import { logger } from '@/lib/logger'
import { rememberPlayerRoute } from '@/lib/session'
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
  rememberPlayerRoute('/favorites/play')
  router.push({ path: '/favorites/play', query: { key: item.key } })
}

function restoreScroll() {
  const saved = Number(sessionStorage.getItem(SCROLL_KEY) ?? 0)
  if (!saved) return
  requestAnimationFrame(() => window.scrollTo({ top: saved }))
}

function goBack() {
  router.push('/')
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
      class="sticky top-0 z-20 border-b border-white/10 bg-[#08080a]/95 px-3 pb-2 pt-[max(0.75rem,env(safe-area-inset-top))] backdrop-blur"
    >
      <div class="flex items-center justify-between gap-2">
        <div class="flex min-w-0 items-center gap-2">
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-white/15 transition hover:bg-white/10"
            title="返回浏览"
            aria-label="返回浏览"
            @click="goBack"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M15 18l-6-6 6-6" />
            </svg>
          </button>
          <div class="min-w-0">
            <h1 class="truncate text-base font-semibold">收藏夹</h1>
            <p class="text-[11px] text-white/45">{{ items.length }} 条 · 可播放 {{ playableCount }} 条</p>
          </div>
        </div>
        <button
          class="h-10 shrink-0 rounded-full border border-white/15 px-3 text-xs text-white/75 transition hover:bg-white/10"
          @click="load"
        >
          刷新
        </button>
      </div>
      <!-- 收藏 / 历史互切 -->
      <nav class="mt-2 grid grid-cols-2 gap-1 rounded-xl bg-white/[0.05] p-1" aria-label="列表切换">
        <router-link
          to="/favorites"
          class="flex h-9 items-center justify-center rounded-lg bg-white text-xs font-medium text-black"
        >
          收藏夹
        </router-link>
        <router-link
          to="/history"
          class="flex h-9 items-center justify-center rounded-lg text-xs text-white/65 transition hover:bg-white/10"
        >
          观看历史
        </router-link>
      </nav>
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
        @click="goBack"
      >
        去看看
      </button>
    </div>

    <MediaGrid v-else :items="items" @open="open" />
  </div>
</template>
