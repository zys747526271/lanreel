<script setup lang="ts">
/**
 * 单条媒体展示组件（9.6：滑动容器与播放器解耦）。
 * 只按 media_type 渲染视频或图片，不关心滑动逻辑。
 *
 * v0.2.0 交互补充：
 *  - 全屏按钮：整条画面进入全屏，视频/图片仍按原始比例（object-contain letterbox）；
 *  - 长按进入倍速模式，上下拖动选档；点击倍速提示按钮恢复 1× 并退出；
 *  - 上下拖动（触摸/鼠标）切换条目；
 *  - 暴露 togglePlay / seekBy 供键盘控制（空格暂停、长按左右快进回退）。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { withToken } from '@/api/client'
import { logger } from '@/lib/logger'
import { reasonText, formatDuration } from '@/lib/format'
import { RATE_OPTIONS, type PlaybackRate } from '@/lib/prefs'
import type { MediaItem } from '@/api/types'

const props = defineProps<{
  item: MediaItem
  active: boolean
  preload: boolean
  imageDuration: number
  muted: boolean
  /** 播放倍速（1 表示原速） */
  rate?: number
  /** 从设置/后台返回时恢复的播放秒数（每条仅消费一次） */
  resumeTime?: number | null
}>()

const emit = defineEmits<{
  (e: 'advance'): void
  (e: 'retreat'): void
  (e: 'blocked', reason: string): void
  (e: 'toggle-favorite', key: string): void
  (e: 'player-error', payload: { key: string; message: string }): void
  /** 浏览器阻止有声自动播放，已降级为静音；用户首次交互后可恢复 */
  (e: 'autoplay-muted'): void
  /** 底部控制条的静音按钮 */
  (e: 'toggle-mute'): void
  /** 播放进度上报（节流到约 1 秒），供页面保存会话 */
  (e: 'progress', payload: { key: string; current: number; duration: number }): void
}>()

const rootEl = ref<HTMLElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)
const playing = ref(false)
const paused = ref(true)
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

/** 倍速模式：非 null 表示正在使用手势倍速（点击提示按钮即可退出恢复 1×）。 */
const rateOverride = ref<number | null>(null)
const rateDragging = ref(false)
const isFullscreen = ref(false)
/** 恢复进度的待处理值，等元数据就绪后再 seek；只消费一次。 */
let pendingResume = props.resumeTime ?? null
let resumeDone = false

/** 手势状态 */
let pointerId: number | null = null
let pressTimer: number | null = null
let startX = 0
let startY = 0
let lastX = 0
let lastY = 0
let longPressed = false
let swiping = false
let rateStartY = 0
let rateStartIndex = 0

let imageTimer: number | null = null
let progressRaf = 0
let lastProgressEmit = 0

const src = computed(() => withToken(props.item.stream_url))
const thumb = computed(() => (props.item.thumb_url ? withToken(props.item.thumb_url) : ''))
const blockedText = computed(() => reasonText(props.item.reason))
const effectiveRate = computed(() => rateOverride.value ?? props.rate ?? 1)
const canRender = computed(() => props.item.playable && !mediaError.value)
const canInteract = computed(() => props.active && canRender.value)
const rateHint = computed(() => `${effectiveRate.value}×`)

function emitProgress(force = false) {
  const el = videoEl.value
  if (!el || props.item.media_type !== 'video') return
  const now = performance.now()
  if (!force && now - lastProgressEmit < 1000) return
  lastProgressEmit = now
  emit('progress', {
    key: props.item.key,
    current: Number(el.currentTime.toFixed(2)),
    duration: Number((Number.isFinite(el.duration) ? el.duration : duration.value).toFixed(2)),
  })
}

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
    paused.value = false
  } catch {
    // 有声自动播放被浏览器拦截：降级为静音重试，并提示用户点击恢复声音。
    if (!el.muted) {
      forcedMute.value = true
      el.muted = true
      emit('autoplay-muted')
      try {
        await el.play()
        playing.value = true
        paused.value = false
        return
      } catch {
        playing.value = false
        paused.value = true
        return
      }
    }
    playing.value = false
    paused.value = true
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
      paused.value = true
    })
  }
}

function pause() {
  const el = videoEl.value
  if (!el) return
  el.pause()
  playing.value = false
  paused.value = true
  emitProgress(true)
}

/** 供父级键盘空格调用；行为与点击画面一致。 */
function togglePlay() {
  if (!canInteract.value || props.item.media_type !== 'video') return
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

/** 供父级长按左右方向键调用：正数为快进，负数为回退。 */
function seekBy(seconds: number) {
  const el = videoEl.value
  if (!el || !Number.isFinite(el.duration)) return
  el.currentTime = Math.max(0, Math.min(el.duration - 0.05, el.currentTime + seconds))
  current.value = el.currentTime
  emitProgress(true)
}

function getCurrentTime() {
  return Number((videoEl.value?.currentTime ?? 0).toFixed(2))
}

function isPaused() {
  return paused.value
}

/** 元数据/首帧就绪后再触发播放，避免加载竞态导致静默失败。 */
function onCanPlay() {
  if (props.active && !userPaused.value && !mediaError.value) void tryPlay()
}

function applyResume() {
  if (resumeDone || pendingResume == null || pendingResume <= 0) return
  const el = videoEl.value
  if (!el || !Number.isFinite(el.duration) || el.duration <= 0) return
  const target = Math.min(pendingResume, Math.max(0, el.duration - 0.3))
  if (target > 0.3) {
    el.currentTime = target
    current.value = target
  }
  resumeDone = true
  pendingResume = null
}

function onTimeUpdate() {
  const el = videoEl.value
  if (!el || seeking.value) return
  current.value = el.currentTime
  if (el.duration && Number.isFinite(el.duration)) duration.value = el.duration
  emitProgress()
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
  event.stopPropagation()
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
  emitProgress(true)
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
  el.playbackRate = effectiveRate.value
  applyResume()
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

/** 成为当前条目：按恢复进度或从头播放，图片启动停留计时。 */
function activate() {
  if (!props.item.playable) return
  userPaused.value = false
  if (props.item.media_type === 'video') {
    const el = videoEl.value
    if (!el) return
    el.playbackRate = effectiveRate.value
    applyResume()
    if (!resumeDone && el.duration && el.currentTime >= el.duration - 0.2) el.currentTime = 0
    void tryPlay()
  } else {
    startImageTimer()
  }
}

function deactivate() {
  emitProgress(true)
  pause()
  clearImageTimer()
  endFullscreenIfOwned()
}

watch(
  () => props.active,
  (active) => (active ? activate() : deactivate()),
)

// 首屏首个条目在组件挂载时即为 active，需要在 DOM 就绪后触发播放。
onMounted(() => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  if (props.active) activate()
})

// <video> 元素在 v-if 渲染完成后才可用，元素出现时补一次播放尝试。
watch(videoEl, (el) => {
  if (el) {
    el.muted = props.muted || forcedMute.value
    el.playbackRate = effectiveRate.value
    applyResume()
    if (props.active) void tryPlay()
  }
})

watch(
  () => props.muted,
  (muted) => {
    if (videoEl.value) videoEl.value.muted = muted || forcedMute.value
  },
)

watch(effectiveRate, (rate) => {
  if (videoEl.value) videoEl.value.playbackRate = rate
})

watch(
  () => props.resumeTime,
  (value) => {
    if (value != null && !resumeDone) {
      pendingResume = value
      applyResume()
    }
  },
)

/* ------------------------------ 全屏 ------------------------------ */

function onFullscreenChange() {
  isFullscreen.value = document.fullscreenElement === rootEl.value
}

function endFullscreenIfOwned() {
  if (rootEl.value && document.fullscreenElement === rootEl.value) {
    void document.exitFullscreen().catch(() => undefined)
  }
}

async function toggleFullscreen(rotate = false) {
  if (document.fullscreenElement === rootEl.value) {
    await document.exitFullscreen().catch(() => undefined)
    return
  }
  if (document.fullscreenElement) await document.exitFullscreen().catch(() => undefined)
  try {
    await rootEl.value?.requestFullscreen?.()
    if (rotate && document.fullscreenElement === rootEl.value) {
      const orientation = screen.orientation as ScreenOrientation & { lock?: (o: string) => Promise<void> }
      await orientation?.lock?.('landscape').catch(() => undefined)
    }
  } catch {
    // iOS Safari 的 <video> 私有全屏作为回退（按视频比例显示）
    const el = videoEl.value as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | null
    el?.webkitEnterFullscreen?.()
  }
}

/* --------------------------- 倍速 / 手势 --------------------------- */

function clearPressTimer() {
  if (pressTimer !== null) {
    window.clearTimeout(pressTimer)
    pressTimer = null
  }
}

/** 长按 500ms 进入倍速模式。 */
function beginRateMode() {
  if (!props.active || !canRender.value) return
  longPressed = true
  swiping = false
  rateDragging.value = true
  rateStartY = lastY
  const currentIndex = Math.max(
    0,
    RATE_OPTIONS.findIndex((r) => r.value === effectiveRate.value),
  )
  rateStartIndex = currentIndex >= 0 ? currentIndex : 1
  if (rateOverride.value === null) rateOverride.value = effectiveRate.value
  if (navigator.vibrate) navigator.vibrate(15)
}

function applyRateDrag(clientY: number) {
  if (!rateDragging.value) return
  const steps = Math.round((rateStartY - clientY) / 34)
  const index = Math.max(0, Math.min(RATE_OPTIONS.length - 1, rateStartIndex + steps))
  const next = RATE_OPTIONS[index].value
  if (next !== rateOverride.value) {
    rateOverride.value = next
    if (navigator.vibrate) navigator.vibrate(10)
  }
}

/** 单击倍速提示按钮：退出倍速模式，恢复 1×（保持素材默认值由设置页决定）。 */
function exitRateMode() {
  rateOverride.value = null
  rateDragging.value = false
  if (videoEl.value) videoEl.value.playbackRate = props.rate ?? 1
  logger.info('interaction', 'player', '退出倍速模式', { video_key: props.item.key })
}

function onGestureDown(event: PointerEvent) {
  if (!canInteract.value) return
  if (event.button !== undefined && event.button !== 0) return
  pointerId = event.pointerId
  startX = lastX = event.clientX
  startY = lastY = event.clientY
  longPressed = false
  swiping = false
  rateDragging.value = false
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  clearPressTimer()
  pressTimer = window.setTimeout(beginRateMode, 500)
}

function onGestureMove(event: PointerEvent) {
  if (pointerId !== event.pointerId || !canInteract.value) return
  lastX = event.clientX
  lastY = event.clientY
  if (rateDragging.value) {
    event.preventDefault()
    applyRateDrag(event.clientY)
    return
  }
  if (!longPressed && Math.abs(event.clientY - startY) > 14) {
    // 判定为切换手势：取消长按，避免与倍速模式冲突。
    clearPressTimer()
    swiping = true
  }
}

function onGestureUp(event: PointerEvent) {
  clearPressTimer()
  if (pointerId !== event.pointerId) return
  pointerId = null
  const finishRate = rateDragging.value
  rateDragging.value = false
  ;(event.currentTarget as HTMLElement).releasePointerCapture?.(event.pointerId)
  if (finishRate) {
    suppressNextClick = true
    return
  }
  if (swiping) {
    const dy = event.clientY - startY
    suppressNextClick = true
    if (dy < -40) emit('advance')
    else if (dy > 40) emit('retreat')
    return
  }
  if (event.type === 'pointercancel') return
  // 解除降级静音的那次点击不能同时暂停/播放
  if (suppressNextClick) {
    suppressNextClick = false
    return
  }
  togglePlay()
}

function onGestureCancel(event: PointerEvent) {
  clearPressTimer()
  if (pointerId !== event.pointerId) return
  pointerId = null
  rateDragging.value = false
  swiping = false
}

onBeforeUnmount(() => {
  clearImageTimer()
  emitProgress(true)
  pause()
  clearPressTimer()
  document.removeEventListener('pointerdown', onFirstGesture)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  endFullscreenIfOwned()
})

// 降级静音后，任意点击/触摸都可恢复声音（浏览器要求在手势回调中调用 play）。
watch(forcedMute, (v) => {
  if (v) document.addEventListener('pointerdown', onFirstGesture, { passive: true })
  else document.removeEventListener('pointerdown', onFirstGesture)
})

defineExpose({ togglePlay, seekBy, getCurrentTime, isPaused, getRate: () => effectiveRate.value, toggleFullscreen })
</script>

<template>
  <div ref="rootEl" class="relative h-full w-full overflow-hidden bg-black">
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
        @play="playing = true; paused = false"
        @pause="playing = false; paused = true"
        @timeupdate="onTimeUpdate"
        @loadedmetadata="onLoadedMetadata"
        @canplay="onCanPlay"
        @error="onVideoError"
      ></video>
      <div v-else class="flex h-full w-full flex-col items-center justify-center gap-2 px-8 text-center">
        <svg class="h-10 w-10 text-white/50" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M12 9v4m0 4h.01M10.3 3.9 2.6 17a2 2 0 0 0 1.7 3h15.4a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" />
        </svg>
        <p class="text-sm text-white/80">{{ mediaError }}</p>
        <p class="text-xs text-white/45">{{ item.rel_path }}</p>
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
    </template>

    <!-- 手势层：触摸/鼠标上下拖动切换，长按进入倍速模式（控件在上层，不受影响） -->
    <div
      v-if="active && item.playable"
      class="absolute inset-0 z-[2] touch-none"
      @pointerdown="onGestureDown"
      @pointermove="onGestureMove"
      @pointerup="onGestureUp"
      @pointercancel="onGestureCancel"
    ></div>

    <!-- 播放/暂停提示 -->
    <transition name="fade">
      <div
        v-if="active && item.media_type === 'video' && !playing && !mediaError"
        class="pointer-events-none absolute inset-0 z-[3] flex items-center justify-center"
      >
        <div class="rounded-full bg-black/40 p-5 backdrop-blur">
          <svg class="h-10 w-10 text-white" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5.5v13l11-6.5z" />
          </svg>
        </div>
      </div>
    </transition>

    <!-- 倍速模式提示按钮：单击即退出倍速模式并恢复 1× -->
    <button
      v-if="active && rateOverride !== null"
      class="absolute left-1/2 top-1/2 z-20 -translate-x-1/2 -translate-y-1/2 rounded-full border border-white/25 bg-black/55 px-5 py-3 text-center text-white shadow-xl backdrop-blur-md transition active:scale-95"
      :title="`当前 ${rateHint}，单击恢复 1×`"
      @pointerdown.stop
      @click.stop="exitRateMode"
    >
      <span class="block text-xl font-semibold tracking-wide">{{ rateHint }}</span>
      <span class="mt-0.5 block text-[10px] text-white/60">上下滑动调速 · 点击恢复 1×</span>
    </button>

    <!-- 底部控制条：进度 / 时间 / 播放 / 静音 / 全屏 -->
    <div
      v-if="active && item.playable"
      class="absolute inset-x-0 bottom-0 z-10 px-4 pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-8"
      @pointerdown.stop
    >
      <!-- 进度条：按下才跳转、按住可拖动（F-04） -->
      <div
        v-if="item.media_type === 'video'"
        class="group relative h-8 cursor-pointer touch-none select-none"
        @pointerdown.prevent.stop="onProgressDown"
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
      <div v-else class="h-[3px] w-full overflow-hidden rounded-full bg-white/20">
        <div class="h-full bg-white/90" :style="{ width: `${imageProgress * 100}%` }"></div>
      </div>

      <div class="mt-1 flex items-center justify-between gap-2 text-[11px] text-white/80">
        <div class="flex items-center gap-2">
          <button
            v-if="item.media_type === 'video'"
            class="flex h-9 w-9 items-center justify-center rounded-full border border-white/15 bg-black/45 backdrop-blur transition hover:bg-black/65 active:scale-90"
            :title="paused ? '播放' : '暂停'"
            @pointerdown.stop
            @click.stop="togglePlay"
          >
            <svg v-if="paused" class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5.5v13l11-6.5z" />
            </svg>
            <svg v-else class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
              <path d="M7 5h3.5v14H7zM13.5 5H17v14h-3.5z" />
            </svg>
          </button>
          <span v-if="item.media_type === 'video'" class="tabular-nums">
            {{ formatDuration(seeking ? seekRatio * duration : current) }} / {{ formatDuration(duration) }}
          </span>
          <span v-else class="tabular-nums">图片 · {{ imageDuration }}s</span>
        </div>
        <div class="flex items-center gap-1.5">
          <span
            v-if="rateOverride !== null"
            class="rounded-full border border-white/20 bg-black/45 px-2 py-1 text-[10px] text-white/80 backdrop-blur"
          >{{ rateHint }}</span>
          <button
            class="flex h-9 w-9 items-center justify-center rounded-full border border-white/15 bg-black/45 backdrop-blur transition hover:bg-black/65 active:scale-90"
            :title="muted ? '开启声音' : '静音'"
            @pointerdown.stop
            @click.stop="emit('toggle-mute')"
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
          <button
            class="flex h-9 w-9 items-center justify-center rounded-full border border-white/15 bg-black/45 backdrop-blur transition hover:bg-black/65 active:scale-90"
            :title="isFullscreen ? '退出全屏' : '全屏（按原始比例）'"
            @pointerdown.stop
            @click.stop="void toggleFullscreen()"
          >
            <svg v-if="!isFullscreen" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M8 3H4a1 1 0 0 0-1 1v4M16 3h4a1 1 0 0 1 1 1v4M8 21H4a1 1 0 0 1-1-1v-4M16 21h4a1 1 0 0 0 1-1v-4" />
            </svg>
            <svg v-else class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M3 8h4a1 1 0 0 0 1-1V3M21 8h-4a1 1 0 0 1-1-1V3M3 16h4a1 1 0 0 1 1 1v4M21 16h-4a1 1 0 0 0-1 1v4" />
            </svg>
          </button>
        </div>
      </div>
    </div>
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
:fullscreen {
  background: #000;
}
:fullscreen video,
:fullscreen img {
  object-fit: contain;
}
</style>
