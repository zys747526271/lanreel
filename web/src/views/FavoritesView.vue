<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import MediaGrid from '@/components/media/MediaGrid.vue'
import { batchRemoveFavorites, cleanMissingFavorites, listFavorites, removeFavorite } from '@/api/favorites'
import { fetchAllMediaStatus } from '@/api/media'
import { useAuthStore } from '@/stores/auth'
import { reasonText } from '@/lib/format'
import { logger } from '@/lib/logger'
import { rememberPlayerRoute } from '@/lib/session'
import { getSnapshot, loadProgressMap } from '@/lib/progress'
import { subscribePolicyChanges } from '@/lib/policyEvents'
import type { MediaItem } from '@/api/types'

const router = useRouter()
const auth = useAuthStore()

const items = ref<MediaItem[]>([])
const loading = ref(true)
const error = ref('')
/** 从播放页返回时恢复滚动位置 */
const SCROLL_KEY = 'lanreel.favorites.scroll'

const playableCount = computed(() => items.value.filter((i) => i.playable).length)
/** 文件已丢失（不是管理员屏蔽）：只对这些条目提供一键清理。 */
const missingItems = computed(() => items.value.filter((i) => i.reason === 'file_missing'))
const selectMode = ref(false)
const selectedKeys = ref<string[]>([])
const actionItem = ref<MediaItem | null>(null)
const busy = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [res] = await Promise.all([listFavorites(), loadProgressMap()])
    const snap = getSnapshot()
    // 需求 2：收藏夹网格显示观看进度条。
    items.value = res.data.map((i) => ({ ...i, watchProgress: snap[i.key]?.progress ?? 0 }))
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

function toggleSelect(item: MediaItem) {
  const i = selectedKeys.value.indexOf(item.key)
  if (i >= 0) selectedKeys.value.splice(i, 1)
  else selectedKeys.value.push(item.key)
}

function toggleSelectMode() {
  selectMode.value = !selectMode.value
  if (!selectMode.value) selectedKeys.value = []
}

function selectAll() {
  selectedKeys.value = selectMode.value && selectedKeys.value.length === items.value.length
    ? []
    : items.value.map((i) => i.key)
}

function showError(err: unknown, fallback: string) {
  error.value = err instanceof Error ? err.message : fallback
  window.setTimeout(() => (error.value = ''), 2600)
}

/** 取消单条收藏，可作用于被管理员屏蔽或文件丢失的条目。 */
async function removeOne(item: MediaItem | null) {
  if (!item) return
  actionItem.value = null
  await removeKeys([item.key], '已取消收藏')
}

async function removeKeys(keys: string[], successText: string) {
  if (!keys.length || busy.value) return
  busy.value = true
  try {
    const res = keys.length === 1
      ? await removeFavorite(keys[0]).then(() => ({ data: { removed: 1 } }))
      : await batchRemoveFavorites(keys)
    selectedKeys.value = selectedKeys.value.filter((k) => !keys.includes(k))
    await load()
    error.value = successText + `（${res.data.removed} 条）`
    window.setTimeout(() => (error.value = ''), 2200)
  } catch (err) {
    showError(err, '取消收藏失败')
  } finally {
    busy.value = false
  }
}

async function removeSelected() {
  await removeKeys([...selectedKeys.value], '已批量取消收藏')
}

async function removeMissing() {
  if (!missingItems.value.length || busy.value) return
  busy.value = true
  try {
    const res = await cleanMissingFavorites()
    await load()
    error.value = res.data.removed ? `已清理 ${res.data.removed} 条丢失文件` : '没有需要清理的丢失文件'
    window.setTimeout(() => (error.value = ''), 2200)
  } catch (err) {
    showError(err, '清理丢失文件失败')
  } finally {
    busy.value = false
  }
}

let stopPolicyEvents: (() => void) | null = null
let statusTimer: number | null = null
let statusSyncing = false
/** 后台策略变更后同步网格模糊状态，无需用户重新进入页面。 */
async function syncStatus() {
  if (statusSyncing || !items.value.length) return
  statusSyncing = true
  try {
    const statuses = await fetchAllMediaStatus(items.value.map((i) => i.key))
    const map = new Map(statuses.map((s) => [s.key, s]))
    items.value = items.value.map((i) => {
      const s = map.get(i.key)
      if (!s) return i
      return { ...i, playable: s.playable, reason: s.playable ? undefined : s.reason }
    })
  } catch (err) {
    logger.warn('app', 'policy', '刷新收藏可播放状态失败', { error: err instanceof Error ? err.message : String(err) })
  } finally {
    statusSyncing = false
  }
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
  stopPolicyEvents = subscribePolicyChanges(() => void syncStatus())
  statusTimer = window.setInterval(() => void syncStatus(), 60000)
  restoreScroll()
})

onBeforeUnmount(() => {
  stopPolicyEvents?.()
  stopPolicyEvents = null
  if (statusTimer !== null) window.clearInterval(statusTimer)
  statusTimer = null
})

// 保留 keep-alive 场景（后续若启用页面缓存）下的刷新能力。
onActivated(() => {
  if (items.value.length) void load()
})
</script>

<template>
  <div class="min-h-[100dvh] bg-[#08080a] pb-10 text-white">
    <header
      class="sticky top-0 z-40 border-b border-white/10 bg-[#08080a]/95 px-3 pb-2 pt-[max(0.75rem,env(safe-area-inset-top))] backdrop-blur"
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
        <div class="flex shrink-0 items-center gap-1.5">
          <button
            class="h-10 rounded-full border border-white/15 px-3 text-xs text-white/75 transition hover:bg-white/10"
            @click="load"
          >
            刷新
          </button>
          <button
            class="h-10 rounded-full border px-3 text-xs transition"
            :class="selectMode ? 'border-reel-accent bg-reel-accent/20 text-reel-accent' : 'border-white/15 text-white/75 hover:bg-white/10'"
            @click="toggleSelectMode"
          >
            {{ selectMode ? '退出选择' : '批量管理' }}
          </button>
        </div>
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
      <div v-if="selectMode" class="mt-2 flex flex-wrap items-center gap-2 rounded-xl bg-white/[0.06] px-3 py-2 text-xs">
        <span class="mr-auto text-white/70">已选 {{ selectedKeys.length }} / {{ items.length }}</span>
        <button class="rounded-lg px-2 py-1 text-white/75 hover:bg-white/10" @click="selectAll">
          {{ selectedKeys.length === items.length ? '取消全选' : '全选' }}
        </button>
        <!-- 批量管理内直接提供一键清理入口，避免窄屏顶部按钮被挤出可视区 -->
        <button
          v-if="missingItems.length"
          class="rounded-lg border border-amber-400/40 bg-amber-500/10 px-2.5 py-1 text-amber-200 transition hover:bg-amber-500/20 disabled:opacity-40"
          :disabled="busy"
          @click="removeMissing"
        >
          一键清除失效视频{{ missingItems.length ? `（${missingItems.length}）` : '' }}
        </button>
        <button
          class="rounded-lg bg-red-500/85 px-3 py-1 font-medium text-white disabled:opacity-40"
          :disabled="!selectedKeys.length || busy"
          @click="removeSelected"
        >
          取消所选收藏
        </button>
      </div>
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

    <MediaGrid
      v-else
      :items="items"
      :selectable="selectMode"
      :selected-keys="selectedKeys"
      @open="open"
      @longpress="actionItem = $event"
      @toggle-select="toggleSelect"
    />

    <!-- 长按收藏条目的操作面板：被管理员屏蔽的条目也能从这里取消收藏 -->
    <transition name="fade">
      <div
        v-if="actionItem"
        class="fixed inset-0 z-50 flex items-end justify-center bg-black/60"
        @click.self="actionItem = null"
      >
        <div class="w-full max-w-md rounded-t-2xl border-t border-white/10 bg-[#16161a] p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          <p class="mb-3 truncate text-sm text-white/85">{{ actionItem.rel_path }}</p>
          <p class="mb-4 text-[11px] text-white/45">
            {{ actionItem.playable ? '可正常播放' : '当前不可播放，但仍可取消收藏' }}
          </p>
          <button
            class="mb-2 w-full rounded-xl bg-red-500/90 py-3 text-sm font-medium text-white disabled:opacity-40"
            :disabled="busy"
            @click="removeOne(actionItem)"
          >
            取消收藏
          </button>
          <button
            class="w-full rounded-xl border border-white/15 py-3 text-sm text-white/75"
            @click="actionItem = null"
          >
            取消
          </button>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
