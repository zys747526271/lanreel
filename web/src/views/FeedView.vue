<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReelSlide from '@/components/reel/ReelSlide.vue'
import { useFeedStore } from '@/stores/feed'
import { useAuthStore } from '@/stores/auth'
import { useFavoritesStore } from '@/stores/favorites'
import { logger } from '@/lib/logger'
import { reasonText } from '@/lib/format'
import { prefs } from '@/lib/prefs'
import { recordHistory } from '@/lib/history'
import { loadFeedSession, rememberPlayerRoute, saveFeedSession } from '@/lib/session'
import { pauseAllVideos } from '@/lib/playerRegistry'

const route = useRoute()
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
/** 从设置/后台返回时恢复的播放位置 */
const positions = ref<Record<string, number>>({})
const pendingResumeTime = ref<number | null>(null)
let toastTimer: number | null = null

type SlideInstance = InstanceType<typeof ReelSlide>
const slideRefs = new Map<number, SlideInstance>()
function setSlideRef(el: unknown, index: number) {
  if (el) slideRefs.set(index, el as SlideInstance)
  else slideRefs.delete(index)
}
const activeSlide = computed(() => slideRefs.get(activeIndex.value))
const currentItem = computed(() => feed.items[activeIndex.value])
const favoriteCount = computed(() => favorites.count)

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
    // 恢复进度只对进入时的那一条生效，切换后不再回跳。
    pendingResumeTime.value = null
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

/* ------------------------- 键盘控制 ------------------------- */
/** 长按 ←/→ 持续快退/快进；首次按下立即响应，随按住时间加速。 */
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
      // 空格 = 暂停/继续（不再切换视频）
      event.preventDefault()
      activeSlide.value?.togglePlay()
      break
    case 'm':
    case 'M':
      muted.value = !muted.value
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
  if (routeLoading.value || !feed.items.length) return
  const item = feed.items[activeIndex.value]
  if (item) {
    const t = activeSlide.value?.getCurrentTime()
    if (t != null) positions.value[item.key] = t
  }
  const sessionKey = auth.user ? `u${auth.user.id}` : 'guest'
  saveFeedSession({
    sessionKey,
    items: feed.items,
    activeIndex: activeIndex.value,
    positions: { ...positions.value },
  })
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

function openList(path: string) {
  if (!auth.isLoggedIn) {
    router.push({ path: '/login', query: { redirect: path } })
    return
  }
  router.push(path)
}

async function refresh() {
  feed.reset()
  activeIndex.value = 0
  positions.value = {}
  pendingResumeTime.value = null
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
  window.addEventListener('keyup', onKeyup)
  window.addEventListener('blur', stopSeek)
  // 注意：刷新由 main.ts 的全局 pagehide → clearAllPlayerSessions 处理；
  // 这里只负责 SPA 内路由离开时的保存（onBeforeUnmount）。
  rememberPlayerRoute(route.fullPath)
  pauseAllVideos()

  const sessionKey = auth.user ? `u${auth.user.id}` : 'guest'
  const saved = loadFeedSession(sessionKey)
  if (saved) {
    feed.restore(saved.items)
    activeIndex.value = Math.min(saved.activeIndex, saved.items.length - 1)
    positions.value = { ...saved.positions }
    pendingResumeTime.value = saved.positions[saved.items[activeIndex.value]?.key ?? ''] ?? null
  } else {
    // 刷新 / 首次进入：始终重新随机，不沿用旧列表。
    feed.reset()
    sessionStorage.setItem('lanreel.feed.session', sessionKey)
  }

  const tasks: Promise<unknown>[] = [loadConfig()]
  if (auth.isLoggedIn) tasks.push(favorites.load())
  await Promise.all(tasks)
  if (!saved) await feed.loadMore(8)
  if (auth.isLoggedIn) feed.applyFavoriteKeys(favorites.keys)
  routeLoading.value = false
  await nextTick()
  scrollToIndex(activeIndex.value, false)
  maybeRecordHistory()
  logger.info('app', 'feed', '进入浏览页', {
    guest: !auth.isLoggedIn,
    restored: !!saved,
    index: activeIndex.value,
  })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('keyup', onKeyup)
  window.removeEventListener('blur', stopSeek)
  stopSeek()
  saveSession()
  pauseAllVideos()
  if (toastTimer) window.clearTimeout(toastTimer)
  if (historyTimer) window.clearTimeout(historyTimer)
})
</script>

<template>
  <div class="fixed inset-0 flex flex-col bg-black text-white">
    <!-- 顶部栏：加大触控区域，移动端常显 -->
    <header
      class="pointer-events-none absolute inset-x-0 top-0 z-30 flex items-start justify-between gap-2 p-3 pt-[max(0.75rem,env(safe-area-inset-top))]"
    >
      <div class="pointer-events-auto flex min-w-0 items-center gap-1.5">
        <span class="hidden select-none text-lg font-semibold tracking-tight drop-shadow sm:inline">LanReel</span>
        <span
          v-if="favorites.offline"
          class="rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] text-amber-200"
          title="网络不可达，正在使用本地收藏缓存"
        >
          离线
        </span>
        <span
          v-if="!auth.isLoggedIn"
          class="hidden rounded-full border border-white/20 bg-black/50 px-2 py-0.5 text-[10px] text-white/70 backdrop-blur sm:inline"
        >
          游客
        </span>
        <button
          class="flex h-9 items-center gap-1 rounded-full border border-white/20 bg-black/55 px-3 text-xs text-white/90 backdrop-blur transition hover:bg-black/75 active:scale-95"
          @click="openList('/favorites')"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9">
            <path d="M12 20s-7-4.4-7-9.2A4.3 4.3 0 0 1 12 7.6a4.3 4.3 0 0 1 7 3.2C19 15.6 12 20 12 20Z" />
          </svg>
          收藏
        </button>
        <button
          class="flex h-9 items-center gap-1 rounded-full border border-white/20 bg-black/55 px-3 text-xs text-white/90 backdrop-blur transition hover:bg-black/75 active:scale-95"
          @click="openList('/history')"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 7v5l3 2" />
          </svg>
          历史
        </button>
      </div>
      <div class="pointer-events-auto flex shrink-0 items-center gap-1.5">
        <router-link
          v-if="auth.isAdmin"
          to="/admin"
          class="flex h-9 items-center rounded-full border border-white/20 bg-black/55 px-3 text-xs text-white/90 backdrop-blur transition hover:bg-black/75"
        >
          后台
        </router-link>
        <router-link
          v-if="!auth.isLoggedIn"
          to="/login"
          class="flex h-9 items-center rounded-full bg-white px-3 text-xs font-medium text-black transition hover:bg-white/90"
        >
          登录
        </router-link>
        <button
          v-else
          class="flex h-9 w-9 items-center justify-center rounded-full border border-white/20 bg-black/55 backdrop-blur transition hover:bg-black/75"
          title="退出登录"
          aria-label="退出登录"
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
        :key="item.uid ?? item.key"
        class="relative h-full w-full snap-start snap-always"
      >
        <ReelSlide
          :ref="(el) => setSlideRef(el, index)"
          :item="item"
          :active="index === activeIndex"
          :preload="Math.abs(index - activeIndex) <= 1"
          :image-duration="imageDuration"
          :muted="muted"
          :rate="rate"
          :resume-time="index === activeIndex ? pendingResumeTime : null"
          @autoplay-muted="showToast('已静音播放，点击画面开启声音')"
          @advance="next"
          @retreat="prev"
          @blocked="onBlocked"
          @toggle-favorite="onToggleFavorite"
          @toggle-mute="muted = !muted"
          @progress="onProgress"
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

    <!-- 右侧操作栏：上下切换 / 收藏 / 信息（放在控制条上方，避免遮挡） -->
    <div
      v-if="currentItem && !feed.isEmpty"
      class="absolute bottom-[7.5rem] right-3 z-30 flex flex-col items-center gap-3.5"
    >
      <button class="flex flex-col items-center gap-0.5" title="上一个（↑）" @click="prev">
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur transition hover:bg-black/70 active:scale-90">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="m6 15 6-6 6 6" />
          </svg>
        </span>
        <span class="text-[10px] text-white/65">上一个</span>
      </button>
      <button
        v-if="auth.isLoggedIn"
        class="group flex flex-col items-center gap-0.5"
        data-action="favorite"
        @click="onToggleFavorite(currentItem.key)"
      >
        <span
          class="flex h-12 w-12 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur transition active:scale-90"
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
        <span class="text-[10px] text-white/65" data-role="favorite-count">{{ favoriteCount }}</span>
      </button>
      <button class="flex flex-col items-center gap-0.5" title="下一个（↓）" @click="next">
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur transition hover:bg-black/70 active:scale-90">
          <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </span>
        <span class="text-[10px] text-white/65">下一个</span>
      </button>
    </div>

    <!-- 设置入口移到左侧偏中，避免右侧操作栏堆叠过多按钮 -->
    <router-link
      v-if="currentItem && !feed.isEmpty"
      :to="auth.isLoggedIn ? '/settings' : '/login'"
      class="absolute left-3 top-[42%] z-30 flex flex-col items-center gap-0.5"
      title="信息 / 设置"
    >
      <span class="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/50 backdrop-blur transition hover:bg-black/70 active:scale-90">
        <svg class="h-5 w-5 text-white/90" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7">
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2V21a2 2 0 1 1-4 0v-.1A1.7 1.7 0 0 0 7 19.4a1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 3 15a1.7 1.7 0 0 0-1.6-1H1.3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 3 9a1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 9 3a1.7 1.7 0 0 0 1-1.6V1.3a2 2 0 1 1 4 0v.1A1.7 1.7 0 0 0 15 3c.6.3 1.3.2 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1A1.7 1.7 0 0 0 21 9v.1c.3.6.9 1 1.6 1h.1a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.6 1Z" />
        </svg>
      </span>
      <span class="text-[10px] text-white/65">设置</span>
    </router-link>

    <!-- 信息条：抬高到控制条上方 -->
    <div
      v-if="currentItem"
      class="pointer-events-none absolute bottom-[6.2rem] left-4 z-20 max-w-[60%] space-y-0.5"
    >
      <p class="truncate text-sm font-medium text-white/95 drop-shadow">
        {{ currentItem.rel_path.split('/').pop() }}
      </p>
      <p class="truncate text-[11px] text-white/65 drop-shadow">
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
