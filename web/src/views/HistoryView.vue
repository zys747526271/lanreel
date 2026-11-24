<script setup lang="ts">
/** 独立的观看历史页：与收藏夹同款网格，支持互切、移除与清空。 */
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import MediaGrid from '@/components/media/MediaGrid.vue'
import { fetchMediaStatus } from '@/api/media'
import { useAuthStore } from '@/stores/auth'
import { reasonText } from '@/lib/format'
import { logger } from '@/lib/logger'
import { clearHistory, listHistory, removeHistory, type HistoryEntry } from '@/lib/history'
import { removeCachedThumb } from '@/lib/thumbCache'
import { rememberPlayerRoute } from '@/lib/session'
import type { MediaItem } from '@/api/types'

const router = useRouter()
const auth = useAuthStore()

const entries = ref<HistoryEntry[]>([])
const loading = ref(true)
const error = ref('')
const SCROLL_KEY = 'lanreel.history.scroll'

const items = computed<MediaItem[]>(() =>
  entries.value.map((h) => ({
    key: h.key,
    media_type: h.media_type,
    root_index: 0,
    root_name: h.root_name,
    rel_path: h.rel_path,
    folder: h.folder,
    duration: h.duration,
    size: 0,
    playable: h.playable,
    reason: h.playable ? undefined : h.reason ?? 'unavailable',
    favorite: false,
    stream_url: h.stream_url,
    thumb_url: h.thumb_url,
  })),
)
const playableCount = computed(() => entries.value.filter((h) => h.playable).length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const saved = await listHistory(100)
    // 历史是本机快照：条目后来被管理员禁用 / 时段变化 / 文件丢失时，
    // 必须以服务端实时判定刷新，才能对无权限内容做模糊遮挡和原因提示。
    const keys = saved.map((h) => h.key).filter(Boolean)
    if (keys.length) {
      try {
        const res = await fetchMediaStatus(keys)
        const live = new Map(res.data.map((s) => [s.key, s]))
        entries.value = saved.map((h) => {
          const s = live.get(h.key)
          return s ? { ...h, playable: s.playable, reason: s.reason } : h
        })
      } catch {
        // 接口不可用时沿用本地快照，不阻塞历史页展示
        entries.value = saved
      }
    } else {
      entries.value = saved
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '读取观看历史失败'
    logger.warn('app', 'history', '读取观看历史失败', { error: error.value })
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
  sessionStorage.setItem(SCROLL_KEY, String(document.scrollingElement?.scrollTop ?? 0))
  rememberPlayerRoute('/play')
  router.push({ path: '/play', query: { src: 'history', key: item.key } })
}

async function drop(item: MediaItem) {
  await removeHistory(item.key)
  await removeCachedThumb(item.key)
  entries.value = entries.value.filter((h) => h.key !== item.key)
}

async function clearAll() {
  if (!window.confirm('确认清空观看历史？该操作仅影响本机记录。')) return
  await clearHistory()
  entries.value = []
}

function goBack() {
  router.push('/')
}

onMounted(async () => {
  if (!auth.isLoggedIn) {
    router.replace({ path: '/login', query: { redirect: '/history' } })
    return
  }
  await load()
  const saved = Number(sessionStorage.getItem(SCROLL_KEY) ?? 0)
  if (saved) requestAnimationFrame(() => window.scrollTo({ top: saved }))
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
            <h1 class="truncate text-base font-semibold">观看历史</h1>
            <p class="text-[11px] text-white/45">{{ items.length }} 条 · 可播放 {{ playableCount }} 条</p>
          </div>
        </div>
        <button
          v-if="entries.length"
          class="h-10 shrink-0 rounded-full border border-red-400/30 px-3 text-xs text-red-300 transition hover:bg-red-500/10"
          @click="clearAll"
        >
          清空
        </button>
      </div>
      <nav class="mt-2 grid grid-cols-2 gap-1 rounded-xl bg-white/[0.05] p-1" aria-label="列表切换">
        <router-link
          to="/favorites"
          class="flex h-9 items-center justify-center rounded-lg text-xs text-white/65 transition hover:bg-white/10"
        >
          收藏夹
        </router-link>
        <router-link
          to="/history"
          class="flex h-9 items-center justify-center rounded-lg bg-white text-xs font-medium text-black"
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
        <circle cx="12" cy="12" r="9" />
        <path d="M12 7v5l3 2" />
      </svg>
      <p class="text-sm text-white/70">还没有观看记录</p>
      <p class="max-w-[16rem] text-xs leading-relaxed text-white/40">
        停留观看超过 1.5 秒的内容会出现在这里，仅保存在本机
      </p>
      <button class="mt-1 rounded-full bg-white px-4 py-2 text-sm font-medium text-black" @click="goBack">
        去看看
      </button>
    </div>

    <MediaGrid v-else :items="items" removable @open="open" @remove="drop" />
  </div>
</template>
