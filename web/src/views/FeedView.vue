<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import ReelSlide from '@/components/reel/ReelSlide.vue'
import { useFeedStore } from '@/stores/feed'
import { useAuthStore } from '@/stores/auth'
import { useFavoritesStore } from '@/stores/favorites'
import { logger } from '@/lib/logger'
import { reasonText } from '@/lib/format'
import { prefs } from '@/lib/prefs'
import { recordHistory } from '@/lib/history'

const router = useRouter()
const feed = useFeedStore()
const auth = useAuthStore()
const favorites = useFavoritesStore()

const scroller = ref<HTMLElement | null>(null)
const activeIndex = ref(0)
const muted = ref(prefs.defaultMuted)
const imageDuration = ref(5)
const rate = computed(() => prefs.defaultRate)
const toast = ref('')
const routeLoading = ref(true)
let toastTimer: number | null = null

function showToast(message: string) {
  toast.value = message
  if (toastTimer) window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = ''), 2600)
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
    if (index >= feed.items.length - 3) void feed.loadMore(5)
    maybeRecordHistory()
  }
}

let historyTimer: number | null = null
/** 停留在某条超过 1.5 秒才记入观看历史，避免快速滑动污染列表。 */
function maybeRecordHistory() {
  if (!auth.isLoggedIn) return
  if (historyTimer) window.clearTimeout(historyTimer)
  historyTimer = window.setTimeout(() => {
    const item = feed.items[activeIndex.value]
    if (item) void recordHistory(item)
  }, 1500)
}

function next() {
  if (activeIndex.value < feed.items.length - 1) scrollToIndex(activeIndex.value + 1)
  else void feed.loadMore(5)
}

function prev() {
  if (activeIndex.value > 0) scrollToIndex(activeIndex.value - 1)
}

function onKeydown(event: KeyboardEvent) {
  if (['INPUT', 'TEXTAREA'].includes((event.target as HTMLElement)?.tagName)) return
  switch (event.key) {
    case 'ArrowDown':
    case 'PageDown':
    case ' ':
      event.preventDefault()
      next()
      break
    case 'ArrowUp':
    case 'PageUp':
      event.preventDefault()
      prev()
      break
    case 'm':
    case 'M':
      muted.value = !muted.value
      break
    default:
      break
  }
}

async function onToggleFavorite(key: string) {
  if (!auth.isLoggedIn) {
    showToast('登录后可收藏')
    router.push({ path: '/login', query: { redirect: '/' } })
    return
  }
  const item = feed.items.find((i) => i.key === key)
  if (!item) return
  try {
    const nowFavorite = await favorites.toggle(item)
    feed.patchFavorite(key, nowFavorite)
    showToast(nowFavorite ? '已收藏' : '已取消收藏')
  } catch (err) {
    showToast(err instanceof Error ? err.message : '收藏失败')
    logger.error('interaction', 'favorites', '收藏操作失败', { error: String(err) })
  }
}

function onBlocked(reason: string) {
  showToast(reasonText(reason))
}

function openFavorites() {
  if (!auth.isLoggedIn) {
    router.push({ path: '/login', query: { redirect: '/favorites' } })
    return
  }
  router.push('/favorites')
}

async function refresh() {
  feed.reset()
  activeIndex.value = 0
  await feed.loadMore(8)
  await nextTick()
  scrollToIndex(0, false)
  maybeRecordHistory()
}

async function loadConfig() {
  try {
    const res = await fetch('/api/v1/config/public').then((r) => r.json())
    if (res?.data?.image_display_seconds) imageDuration.value = res.data.image_display_seconds
  } catch {
    /* 保留默认值 */
  }
}

async function signOut() {
  await auth.logout()
  favorites.reset()
  router.push('/login')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  // 登录身份变化（游客↔登录、换账号）时重建列表，避免残留上一身份的条目
  const sessionKey = auth.user ? `u${auth.user.id}` : 'guest'
  if (sessionStorage.getItem('lanreel.feed.session') !== sessionKey) {
    feed.reset()
    sessionStorage.setItem('lanreel.feed.session', sessionKey)
  }
  const tasks: Promise<unknown>[] = [loadConfig()]
  if (auth.isLoggedIn) tasks.push(favorites.load())
  await Promise.all(tasks)
  await feed.loadMore(8)
  if (auth.isLoggedIn) feed.applyFavoriteKeys(favorites.keys)
  routeLoading.value = false
  maybeRecordHistory()
  logger.info('app', 'feed', '进入浏览页', { guest: !auth.isLoggedIn })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  if (toastTimer) window.clearTimeout(toastTimer)
  if (historyTimer) window.clearTimeout(historyTimer)
})

watch(
  () => favorites.keys,
  (keys) => feed.applyFavoriteKeys(keys),
  { deep: true },
)

const currentItem = computed(() => feed.items[activeIndex.value])
const favoriteCount = computed(() => favorites.count)
</script>

<template>
  <div class="fixed inset-0 flex flex-col bg-black text-white">
    <!-- 顶部栏 -->
    <header class="pointer-events-none absolute inset-x-0 top-0 z-30 flex items-start justify-between p-3 pt-[max(0.75rem,env(safe-area-inset-top))]">
      <div class="pointer-events-auto flex items-center gap-2">
        <span class="select-none text-lg font-semibold tracking-tight drop-shadow">LanReel</span>
        <span
          v-if="favorites.offline"
          class="rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] text-amber-200"
          title="网络不可达，正在使用本地收藏缓存"
        >
          离线
        </span>
        <span
          v-if="!auth.isLoggedIn"
          class="rounded-full border border-white/20 bg-black/40 px-2 py-0.5 text-[10px] text-white/70 backdrop-blur"
        >
          游客
        </span>
        <button
          class="rounded-full border border-white/15 bg-black/40 px-3 py-1 text-xs text-white/80 backdrop-blur transition hover:bg-black/60"
          @click="openFavorites"
        >
          收藏夹
        </button>
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
        <router-link
          v-if="auth.isAdmin"
          to="/admin"
          class="rounded-full border border-white/15 bg-black/40 px-3 py-2 text-xs backdrop-blur transition hover:bg-black/60"
        >
          后台
        </router-link>
        <router-link
          to="/settings"
          class="rounded-full border border-white/15 bg-black/40 p-2 backdrop-blur transition hover:bg-black/60"
          title="设置与日志"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2V21a2 2 0 1 1-4 0v-.1A1.7 1.7 0 0 0 7 19.4a1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 3 15a1.7 1.7 0 0 0-1.6-1H1.3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 3 9a1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 9 3a1.7 1.7 0 0 0 1-1.6V1.3a2 2 0 1 1 4 0v.1A1.7 1.7 0 0 0 15 3c.6.3 1.3.2 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1A1.7 1.7 0 0 0 21 9v.1c.3.6.9 1 1.6 1h.1a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.6 1Z" />
          </svg>
        </router-link>
        <router-link
          v-if="!auth.isLoggedIn"
          to="/login"
          class="rounded-full bg-white px-3 py-2 text-xs font-medium text-black transition hover:bg-white/90"
        >
          登录
        </router-link>
        <button
          v-else
          class="rounded-full border border-white/15 bg-black/40 p-2 backdrop-blur transition hover:bg-black/60"
          title="退出登录"
          @click="signOut"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M15 17l5-5-5-5M20 12H9M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h7" />
          </svg>
        </button>
      </div>
    </header>

    <!-- 滑动容器 -->
    <div
      ref="scroller"
      class="no-scrollbar h-full w-full snap-y snap-mandatory overflow-y-scroll overscroll-none"
      @scroll.passive="onScroll"
    >
      <section
        v-for="(item, index) in feed.items"
        :key="item.uid"
        class="relative h-full w-full snap-start snap-always"
      >
        <ReelSlide
          :item="item"
          :active="index === activeIndex"
          :preload="Math.abs(index - activeIndex) <= 1"
          :image-duration="imageDuration"
          :muted="muted"
          :rate="rate"
          @autoplay-muted="showToast('已静音播放，点击画面开启声音')"
          @advance="next"
          @blocked="onBlocked"
          @toggle-favorite="onToggleFavorite"
          @player-error="(p) => showToast(p.message)"
        />
      </section>

      <section v-if="feed.loading" class="flex h-full w-full snap-start items-center justify-center">
        <div class="flex flex-col items-center gap-3 text-white/60">
          <div class="h-8 w-8 animate-spin rounded-full border-2 border-white/25 border-t-white/80"></div>
          <span class="text-sm">正在加载更多…</span>
        </div>
      </section>
    </div>

    <!-- 空状态 -->
    <div
      v-if="!routeLoading && feed.isEmpty"
      class="absolute inset-0 z-20 flex flex-col items-center justify-center gap-4 bg-black/92 px-8 text-center"
    >
      <svg class="h-12 w-12 text-white/35" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4">
        <rect x="2.5" y="4.5" width="19" height="15" rx="3" />
        <path d="m9 9 6 3-6 3V9Z" />
      </svg>
      <p class="text-base font-medium">{{ auth.isLoggedIn ? '媒体库暂无可播放内容' : '游客暂无可看内容' }}</p>
      <p class="max-w-xs text-sm leading-relaxed text-white/50">
        {{
          auth.isLoggedIn
            ? '请确认服务端媒体目录已配置正确，或当前内容被管理员策略限制'
            : '管理员尚未对游客开放内容，登录后可以看更多'
        }}
      </p>
      <div class="flex gap-2">
        <button
          class="rounded-full bg-white px-4 py-2 text-sm font-medium text-black transition hover:bg-white/90"
          @click="refresh"
        >
          重新加载
        </button>
        <router-link
          v-if="!auth.isLoggedIn"
          to="/login"
          class="rounded-full border border-white/20 px-4 py-2 text-sm text-white/80 transition hover:bg-white/10"
        >
          登录 / 注册
        </router-link>
      </div>
    </div>

    <!-- 右侧操作栏 -->
    <div
      v-if="currentItem && !feed.isEmpty"
      class="absolute bottom-44 right-3 z-30 flex flex-col items-center gap-5"
    >
      <button
        v-if="auth.isLoggedIn"
        class="group flex flex-col items-center gap-1"
        data-action="favorite"
        @click="onToggleFavorite(currentItem.key)"
      >
        <span
          class="flex h-12 w-12 items-center justify-center rounded-full border border-white/15 bg-black/40 backdrop-blur transition group-active:scale-90"
        >
          <svg
            class="h-6 w-6 transition"
            :class="currentItem.favorite ? 'scale-110 text-reel-accent' : 'text-white'"
            viewBox="0 0 24 24"
            :fill="currentItem.favorite ? 'currentColor' : 'none'"
            stroke="currentColor"
            stroke-width="1.8"
          >
            <path d="M12 20s-7-4.4-7-9.2A4.3 4.3 0 0 1 12 7.6a4.3 4.3 0 0 1 7 3.2C19 15.6 12 20 12 20Z" />
          </svg>
        </span>
        <span class="text-[11px] text-white/70" data-role="favorite-count">{{ favoriteCount }}</span>
      </button>
      <router-link
        :to="auth.isLoggedIn ? '/settings' : '/login'"
        class="flex flex-col items-center gap-1"
      >
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/15 bg-black/40 backdrop-blur">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11v5m0-8.2v.1" />
          </svg>
        </span>
        <span class="text-[11px] text-white/60">信息</span>
      </router-link>
      <button class="flex flex-col items-center gap-1" @click="next">
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/15 bg-black/40 backdrop-blur">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </span>
        <span class="text-[11px] text-white/60">下一个</span>
      </button>
    </div>

    <!-- 信息条 -->
    <div
      v-if="currentItem"
      class="pointer-events-none absolute bottom-24 left-4 z-20 max-w-[70%] space-y-1"
    >
      <p class="truncate text-sm font-medium text-white/95">
        {{ currentItem.rel_path.split('/').pop() }}
      </p>
      <p class="truncate text-[11px] text-white/55">
        {{ currentItem.folder || '根目录' }} · {{ currentItem.media_type === 'video' ? '视频' : '图片' }}
      </p>
      <p v-if="!currentItem.playable" class="text-[11px] text-amber-300/90">
        {{ reasonText(currentItem.reason) }}
      </p>
    </div>

    <!-- 轻提示 -->
    <transition name="toast">
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
.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translate(-50%, -6px);
}
</style>
