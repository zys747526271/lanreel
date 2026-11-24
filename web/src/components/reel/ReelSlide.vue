<script setup lang="ts">
/**
 * 单条媒体展示组件（9.6：滑动容器与播放器解耦）。
 * 只按 media_type 渲染视频或图片，不关心滑动逻辑。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { withToken } from '@/api/client'
import { logger } from '@/lib/logger'
import { reasonText, formatDuration } from '@/lib/format'
import type { MediaItem } from '@/api/types'

const props = defineProps<{
  item: MediaItem
  active: boolean
  preload: boolean
  imageDuration: number
  muted: boolean
  /** 播放倍速（1 表示原速） */
  rate?: number
}>()

const emit = defineEmits<{
  (e: 'advance'): void
  (e: 'blocked', reason: string): void
  (e: 'toggle-favorite', key: string): void
  (e: 'player-error', payload: { key: string; message: string }): void
  /** 浏览器阻止有声自动播放，已降级为静音；用户首次交互后可恢复 */
  (e: 'autoplay-muted'): void
}>()

const videoEl = ref<HTMLVideoElement | null>(null)
const playing = ref(false)
const current = ref(0)
const duration = ref(props.item.duration || 0)
const mediaError = ref('')
const imageProgress = ref(0)
const imageLoaded = ref(false)
/** 用户是否手动暂停：为 true 时自动播放逻辑不再抢占控制权 */
const userPaused = ref(false)
/** 浏览器自动播放策略导致的有声播放失败：临时静音，等用户手势后恢复 */
const forcedMute = ref(false)
/** 解除降级静音的那一次点击不再触发暂停（否则用户点一下会同时静音恢复+暂停） */
let suppressNextClick = false
/** 进度条拖动状态（只有按下才跳转，见 F-04 修复） */
const seeking = ref(false)
const seekRatio = ref(0)

let imageTimer: number | null = null
let progressRaf = 0

const src = computed(() => withToken(props.item.stream_url))
const thumb = computed(() => (props.item.thumb_url ? withToken(props.item.thumb_url) : ''))
const blockedText = computed(() => reasonText(props.item.reason))

const canRender = computed(() => props.item.playable && !mediaError.value)

function clearImageTimer() {
  if (imageTimer !== null) {
    window.clearTimeout(imageTimer)
    imageTimer = null
  }
  if (progressRaf) {
    cancelAnimationFrame(progressRaf)
    progressRaf = 0
  }
  imageProgress.value = 0
}

/** 图片按配置时长自动切换（F-03），并用进度条提示停留时间。 */
function startImageTimer() {
  clearImageTimer()
  const total = Math.max(1, props.imageDuration) * 1000
  const started = performance.now()
  const tick = () => {
    const elapsed = performance.now() - started
    imageProgress.value = Math.min(1, elapsed / total)
    if (elapsed >= total) {
      emit('advance')
      return
    }
    progressRaf = requestAnimationFrame(tick)
  }
  progressRaf = requestAnimationFrame(tick)
  imageTimer = window.setTimeout(() => emit('advance'), total)
}

async function tryPlay() {
  const el = videoEl.value
  if (!el) return
  el.muted = props.muted || forcedMute.value
  try {
    await el.play()
    playing.value = true
  } catch {
    // 有声自动播放被浏览器拦截：降级为静音重试，并提示用户点击恢复声音。
    if (!el.muted) {
      forcedMute.value = true
      el.muted = true
      emit('autoplay-muted')
      try {
        await el.play()
        playing.value = true
        return
      } catch {
        playing.value = false
        return
      }
    }
    playing.value = false
  }
}

/** 用户首次交互后解除降级静音（必须在手势回调里执行）。 */
function onFirstGesture() {
  if (!forcedMute.value) return
  forcedMute.value = false
  suppressNextClick = true
  const el = videoEl.value
  if (!el) return
  el.muted = props.muted
  if (props.active && !userPaused.value) {
    void el.play().catch(() => {
      playing.value = false
    })
  }
}

function pause() {
  const el = videoEl.value
  if (!el) return
  el.pause()
  playing.value = false
}

function onTogglePlay(event?: Event) {
  event?.stopPropagation()
  if (suppressNextClick) {
    suppressNextClick = false
    return
  }
  if (!canRender.value || props.item.media_type !== 'video') return
  const el = videoEl.value
  if (!el) return
  if (el.paused) {
    userPaused.value = false
    void tryPlay()
  } else {
    userPaused.value = true
    pause()
  }
}

/** 元数据/首帧就绪后再触发播放，避免加载竞态导致静默失败。 */
function onCanPlay() {
  if (props.active && !userPaused.value && !mediaError.value) void tryPlay()
}

function onTimeUpdate() {
  const el = videoEl.value
  if (!el || seeking.value) return
  current.value = el.currentTime
  if (el.duration && Number.isFinite(el.duration)) duration.value = el.duration
}

function seekByRatio(ratio: number) {
  const el = videoEl.value
  if (!el || !duration.value) return
  el.currentTime = Math.max(0, Math.min(1, ratio)) * duration.value
}

function ratioFromEvent(event: PointerEvent): number {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  if (rect.width <= 0) return 0
  return Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
}

/**
 * 进度条交互（F-04）：
 * - 只有按下（pointerdown）才跳转，鼠标悬停/触摸滑过都不改变播放位置；
 * - 按住拖动实时更新，松手保持；
 * - 拖动期间暂停 timeupdate 覆盖进度显示，避免抖动。
 */
function onProgressDown(event: PointerEvent) {
  if (!canRender.value) return
  const ratio = ratioFromEvent(event)
  seeking.value = true
  seekRatio.value = ratio
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  seekByRatio(ratio)
}

function onProgressMove(event: PointerEvent) {
  if (!seeking.value) return
  const ratio = ratioFromEvent(event)
  seekRatio.value = ratio
  seekByRatio(ratio)
}

function onProgressUp(event: PointerEvent) {
  if (!seeking.value) return
  seekByRatio(ratioFromEvent(event))
  seeking.value = false
  ;(event.currentTarget as HTMLElement).releasePointerCapture?.(event.pointerId)
}

const displayRatio = computed(() => {
  if (seeking.value) return seekRatio.value
  return duration.value ? Math.min(1, current.value / duration.value) : 0
})

const seekPreviewText = computed(() => formatDuration(seekRatio.value * duration.value))

function onLoadedMetadata() {
  const el = videoEl.value
  if (!el) return
  if (Number.isFinite(el.duration)) duration.value = el.duration
  el.playbackRate = props.rate ?? 1
}

function onVideoError() {
  const el = videoEl.value
  const code = el?.error?.code
  const map: Record<number, string> = {
    1: '播放被中止',
    2: '网络错误，无法加载视频',
    3: '视频解码失败（浏览器可能不支持该编码）',
    4: '视频格式不受支持（建议使用 MP4/H.264）',
  }
  mediaError.value = map[code ?? 0] ?? '视频无法播放'
  logger.error('player', 'video', '视频播放失败', {
    video_key: props.item.key,
    rel_path: props.item.rel_path,
    code,
  })
  emit('player-error', { key: props.item.key, message: mediaError.value })
}

function onBlockedClick() {
  emit('blocked', props.item.reason ?? 'unknown')
}

/** 成为当前条目：视频从头播放，图片启动停留计时。 */
function activate() {
  if (!props.item.playable) return
  userPaused.value = false
  if (props.item.media_type === 'video') {
    const el = videoEl.value
    if (!el) return
    if (el.duration && el.currentTime >= el.duration - 0.2) el.currentTime = 0
    void tryPlay()
  } else {
    startImageTimer()
  }
}

function deactivate() {
  pause()
  clearImageTimer()
}

watch(
  () => props.active,
  (active) => (active ? activate() : deactivate()),
)

// 首屏首个条目在组件挂载时即为 active，需要在 DOM 就绪后触发播放。
onMounted(() => {
  if (props.active) activate()
})

// <video> 元素在 v-if 渲染完成后才可用，元素出现时补一次播放尝试。
watch(videoEl, (el) => {
  if (el) {
    el.muted = props.muted || forcedMute.value
    el.playbackRate = props.rate ?? 1
    if (props.active) void tryPlay()
  }
})

watch(
  () => props.muted,
  (muted) => {
    if (videoEl.value) videoEl.value.muted = muted || forcedMute.value
  },
)

watch(
  () => props.rate,
  (rate) => {
    if (videoEl.value) videoEl.value.playbackRate = rate ?? 1
  },
)

onBeforeUnmount(() => {
  clearImageTimer()
  pause()
  document.removeEventListener('pointerdown', onFirstGesture)
})

// 降级静音后，任意点击/触摸都可恢复声音（浏览器要求在手势回调中调用 play）。
watch(forcedMute, (v) => {
  if (v) document.addEventListener('pointerdown', onFirstGesture, { passive: true })
  else document.removeEventListener('pointerdown', onFirstGesture)
})
</script>

<template>
  <div class="relative h-full w-full overflow-hidden bg-black">
    <!-- 不可播放：模糊预览 + 原因说明（F-16） -->
    <div v-if="!item.playable" class="absolute inset-0">
      <img
        v-if="thumb"
        :src="thumb"
        class="h-full w-full scale-110 object-cover opacity-60 blur-2xl"
        alt=""
      />
      <div v-else class="h-full w-full bg-gradient-to-br from-slate-900 via-slate-800 to-black"></div>
      <div class="absolute inset-0 flex items-center justify-center px-8">
        <button
          class="flex max-w-sm flex-col items-center gap-3 rounded-2xl bg-black/45 px-8 py-7 text-center backdrop-blur-md transition hover:bg-black/55"
          @click="onBlockedClick"
        >
          <svg class="h-9 w-9 text-white/85" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6">
            <rect x="4" y="10.5" width="16" height="10" rx="2.5" />
            <path d="M8 10.5V8a4 4 0 0 1 8 0v2.5" />
          </svg>
          <p class="text-base font-medium text-white">{{ blockedText }}</p>
          <p class="text-xs leading-relaxed text-white/60">
            条目仍保留在你的收藏中，恢复可播放后可继续观看
          </p>
        </button>
      </div>
    </div>

    <!-- 视频 -->
    <template v-else-if="item.media_type === 'video'">
      <video
        v-if="!mediaError"
        ref="videoEl"
        class="h-full w-full object-contain"
        :src="src"
        :preload="preload ? 'auto' : 'metadata'"
        :muted="muted"
        loop
        playsinline
        webkit-playsinline
        @timeupdate="onTimeUpdate"
        @loadedmetadata="onLoadedMetadata"
        @canplay="onCanPlay"
        @error="onVideoError"
        @click="onTogglePlay"
      ></video>
      <div v-else class="flex h-full w-full flex-col items-center justify-center gap-2 px-8 text-center">
        <svg class="h-10 w-10 text-white/50" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M12 9v4m0 4h.01M10.3 3.9 2.6 17a2 2 0 0 0 1.7 3h15.4a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" />
        </svg>
        <p class="text-sm text-white/80">{{ mediaError }}</p>
        <p class="text-xs text-white/45">{{ item.rel_path }}</p>
      </div>

      <!-- 播放/暂停提示 -->
      <transition name="fade">
        <div
          v-if="!playing && !mediaError"
          class="pointer-events-none absolute inset-0 flex items-center justify-center"
        >
          <div class="rounded-full bg-black/40 p-5 backdrop-blur">
            <svg class="h-10 w-10 text-white" viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5.5v13l11-6.5z" />
            </svg>
          </div>
        </div>
      </transition>

      <!-- 进度条：按下才跳转、按住可拖动（F-04） -->
      <div class="absolute inset-x-0 bottom-0 z-10 px-4 pb-3 pt-6">
        <div
          class="group relative h-8 cursor-pointer touch-none select-none"
          @pointerdown.prevent="onProgressDown"
          @pointermove="onProgressMove"
          @pointerup="onProgressUp"
          @pointercancel="onProgressUp"
        >
          <div class="absolute top-1/2 h-[3px] w-full -translate-y-1/2 rounded-full bg-white/25">
            <div
              class="h-full rounded-full bg-white/90"
              :style="{ width: `${displayRatio * 100}%` }"
            ></div>
          </div>
          <!-- 拖动把手：平时隐藏，按下或悬浮时出现 -->
          <div
            class="absolute top-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full bg-white shadow transition-opacity"
            :class="seeking ? 'opacity-100' : 'opacity-0 group-hover:opacity-80'"
            :style="{ left: `${displayRatio * 100}%` }"
          ></div>
          <div
            v-if="seeking"
            class="pointer-events-none absolute -top-7 -translate-x-1/2 rounded bg-black/75 px-1.5 py-0.5 text-[11px] text-white"
            :style="{ left: `${displayRatio * 100}%` }"
          >
            {{ seekPreviewText }}
          </div>
        </div>
        <div class="mt-0.5 flex items-center justify-between text-[11px] text-white/70">
          <span>{{ formatDuration(seeking ? seekRatio * duration : current) }}</span>
          <span>{{ formatDuration(duration) }}</span>
        </div>
      </div>
    </template>

    <!-- 图片 -->
    <template v-else>
      <img
        v-if="thumb"
        :src="thumb"
        class="absolute inset-0 h-full w-full scale-105 object-cover blur-xl opacity-40"
        alt=""
        aria-hidden="true"
      />
      <img
        :src="src"
        class="relative z-[1] h-full w-full object-contain transition-opacity duration-300"
        :class="imageLoaded ? 'opacity-100' : 'opacity-0'"
        alt=""
        @load="imageLoaded = true"
        @error="
          mediaError = '图片加载失败';
          logger.error('player', 'image', '图片加载失败', { video_key: item.key })
        "
      />
      <div class="absolute inset-x-0 bottom-0 z-10 px-4 pb-3">
        <div class="h-[3px] w-full overflow-hidden rounded-full bg-white/20">
          <div class="h-full bg-white/90" :style="{ width: `${imageProgress * 100}%` }"></div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
