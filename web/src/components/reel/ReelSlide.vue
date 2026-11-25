<script setup lang="ts">
/**
 * 单条媒体展示组件（9.6：滑动容器与播放器解耦）。
 * 只按 media_type 渲染视频或图片，不关心滑动逻辑。
 *
 * v0.2.0 交互补充：
 *  - 全屏按钮：整条画面进入全屏，视频/图片仍按原始比例（object-contain letterbox）；
 *  - 长按进入倍速（默认 1.5×），上下拖动选档、松手保持；点底部提示恢复原速；
 *  - 上下拖动（触摸/鼠标）切换条目；
 *  - 暴露 togglePlay / seekBy 供键盘控制（空格暂停、长按左右快进回退）。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { withToken } from '@/api/client'
import { logger } from '@/lib/logger'
import { reasonText, formatDuration } from '@/lib/format'
import { RATE_OPTIONS, type PlaybackRate, type EndAction } from '@/lib/prefs'
import { pauseOtherVideos, registerVideo, unregisterVideo } from '@/lib/playerRegistry'
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
  /** 播放结束后的行为（v0.3.0） */
  endAction?: EndAction
  /** 当前条目观看进度比例 0~1（用于列表提示，播放时不需要） */
  watchedProgress?: number
  /** 沉浸模式：隐藏所有页面与播放器 UI，仅保留退出按钮 */
  immersive?: boolean
  /** 播放器音量 0~1（全屏竖滑调节，父级持久化） */
  volume?: number
  /** 首页第一条允许下拉刷新随机流 */
  allowRefresh?: boolean
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
  /** 播放进度上报（节流到约 1 秒）；started 表示确实触发过播放，watchedDelta 只含自然播放推进 */
  (e: 'progress', payload: { key: string; current: number; duration: number; started: boolean; justStarted: boolean; watchedDelta: number }): void
  /** 视频自然播放结束（由页面按 endAction 决定暂停/循环/下一个） */
  (e: 'ended', key: string): void
  /** 进入/退出沉浸模式 */
  (e: 'toggle-immersive'): void
  /** 首页第一条下拉刷新 */
  (e: 'refresh'): void
  /** 全屏竖滑调节音量 */
  (e: 'volume-change', payload: { volume: number; muted: boolean }): void
}>()

const rootEl = ref<HTMLElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)
const playing = ref(false)
const paused = ref(true)
/** 视频已自然结束：此时 <video> 在全屏下可能只保留黑帧，需显示封面/重播层。 */
const videoEnded = ref(false)
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

/** 快进/倍速交互时进入极简模式：只显示进度条与时间（v0.3.0 需求 8）。 */
const minimalUI = ref(false)
/** 快进时显示的时间点 HUD（v0.3.0 需求 4）。 */
const seekHud = ref('')
/** 沉浸模式下快进/倍速时临时显示的进度条（沉浸模式原本隐藏全部控件）。 */
const interactionHud = ref(false)
/** 沉浸模式倍速恢复按钮：开启 3 秒后半透明，与退出沉浸按钮一致。 */
const rateRestoreFaded = ref(false)
let minimalTimer: number | null = null
let seekHudTimer: number | null = null
let interactionTimer: number | null = null
let rateRestoreTimer: number | null = null

/** delay<=0 表示由手势持有；操作结束后再给一个短暂的收起延时。 */
function showInteractionHud(delay = 1000) {
  interactionHud.value = true
  if (interactionTimer !== null) window.clearTimeout(interactionTimer)
  interactionTimer = null
  if (delay <= 0) return
  interactionTimer = window.setTimeout(() => {
    interactionHud.value = false
    interactionTimer = null
  }, delay)
}

function hideInteractionHud() {
  if (interactionTimer !== null) window.clearTimeout(interactionTimer)
  interactionTimer = null
  interactionHud.value = false
}

/** 进入极简模式；delay<=0 表示由手势持有，直到松手时显式退出。 */
function enterMinimal(delay = 1600) {
  minimalUI.value = true
  document.body.classList.add('reel-minimal')
  if (minimalTimer !== null) window.clearTimeout(minimalTimer)
  minimalTimer = null
  if (delay <= 0) return
  minimalTimer = window.setTimeout(() => {
    minimalUI.value = false
    document.body.classList.remove('reel-minimal')
    minimalTimer = null
  }, delay)
}

/** 立即退出极简模式（组件卸载 / 退出时）。 */
function exitMinimal() {
  if (minimalTimer !== null) window.clearTimeout(minimalTimer)
  minimalTimer = null
  minimalUI.value = false
  document.body.classList.remove('reel-minimal')
}

/** 显示快进/调速的时间提示（当前 / 总时长）。 */
function showSeekHud(text: string, delay = 1400) {
  seekHud.value = text
  if (seekHudTimer !== null) window.clearTimeout(seekHudTimer)
  seekHudTimer = window.setTimeout(() => {
    seekHud.value = ''
    seekHudTimer = null
  }, delay)
}

/** 全屏 / 沉浸手势的居中提示（不被沉浸模式隐藏）。 */
function showGestureHud(text: string, delay = 1200) {
  gestureHud.value = text
  if (gestureHudTimer !== null) window.clearTimeout(gestureHudTimer)
  gestureHudTimer = window.setTimeout(() => {
    gestureHud.value = ''
    gestureHudTimer = null
  }, delay)
}

/** 倍速模式：非 null 表示手势倍速已生效（底部提示按钮可退出恢复原速）。 */
const rateOverride = ref<number | null>(null)
/** 中央倍速面板：仅按住调速期间显示，松手即收起，倍速保持生效（Kazumi 模式）。 */
const rateSelecting = ref(false)
/** 当前指针是否正按在面板上纵向调速。 */
const rateDragging = ref(false)
const isFullscreen = ref(false)
/** 移动端用 CSS 铺满替代原生全屏：旋转屏幕不会触发 WebView 重建/页面重载。 */
const cssFullscreen = ref(false)
const immersiveExitFaded = ref(false)
let immersiveTimer: number | null = null
const isTouchDevice = typeof window !== 'undefined' && !!window.matchMedia?.('(pointer: coarse)').matches
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

/**
 * 手势方向锁（参考 ZFPlayer / TigerVideoPlayer）：
 * - 全屏 / 沉浸：横向拖动快进，全屏纵向拖动调音量；
 * - 普通模式：纵向拖动切视频，首页第一条下拉刷新。
 * 三种模式互斥，避免全屏切换视频导致黑屏串音。
 */
type GestureMode = 'none' | 'seek' | 'volume'
let gestureMode: GestureMode = 'none'
let gestureStartY = 0
let gestureStartCurrent = 0
let gestureStartVolume = 1
const gestureHud = ref('')
let gestureHudTimer: number | null = null

let imageTimer: number | null = null
let progressRaf = 0
let lastProgressEmit = 0
/** 真实播放判定与自然推进时长（预加载/拖动进度条不计入）。 */
let hasStarted = false
let lastObservedTime: number | null = null
let lastTickAt = 0
let pendingWatched = 0

/** 跳转后丢弃这一段 currentTime 变化，避免把拖动进度条算作观看。 */
function resetWatchObservation() {
  lastObservedTime = null
  lastTickAt = performance.now()
}

const src = computed(() => withToken(props.item.stream_url))
const thumb = computed(() => (props.item.thumb_url ? withToken(props.item.thumb_url) : ''))
const blockedText = computed(() => reasonText(props.item.reason))
const effectiveRate = computed(() => rateOverride.value ?? props.rate ?? 1)
const canRender = computed(() => props.item.playable && !mediaError.value)
const canInteract = computed(() => props.active && canRender.value)
const rateHint = computed(() => `${effectiveRate.value}×`)

function emitProgress(force = false, justStarted = false) {
  const el = videoEl.value
  if (!el || props.item.media_type !== 'video') return
  const now = performance.now()
  if (!force && now - lastProgressEmit < 1000) return
  lastProgressEmit = now
  const watchedDelta = pendingWatched
  pendingWatched = 0
  emit('progress', {
    key: props.item.key,
    current: Number(el.currentTime.toFixed(2)),
    duration: Number((Number.isFinite(el.duration) ? el.duration : duration.value).toFixed(2)),
    started: hasStarted,
    justStarted,
    watchedDelta: Number(watchedDelta.toFixed(2)),
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
  if (!el || !props.active) return
  // 会话恢复 / 快速切换时，避免首条与恢复条目同时出声。
  pauseOtherVideos(el)
  // ended 状态下再次 play 浏览器虽会自动回到开头，但原生全屏常先显示黑帧；
  // 这里主动归零并清掉结束态，让画面立刻恢复第一帧。
  if (videoEnded.value || el.ended) {
    videoEnded.value = false
    el.currentTime = 0
    current.value = 0
    resetWatchObservation()
  }
  el.muted = props.muted || forcedMute.value
  try {
    await el.play()
    // play() 是异步的：等待期间可能已经切走，此时必须立刻让位。
    if (!props.active || userPaused.value) {
      el.pause()
      playing.value = false
      paused.value = true
      return
    }
    pauseOtherVideos(el)
    playing.value = true
    paused.value = false
  } catch {
    if (!props.active || userPaused.value) {
      playing.value = false
      paused.value = true
      return
    }
    // 有声自动播放被浏览器拦截：降级为静音重试，并提示用户点击恢复声音。
    if (!el.muted) {
      forcedMute.value = true
      el.muted = true
      emit('autoplay-muted')
      try {
        await el.play()
        if (!props.active || userPaused.value) {
          el.pause()
          playing.value = false
          paused.value = true
          return
        }
        pauseOtherVideos(el)
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
  if (videoEnded.value || el.ended) {
    replayFromStart()
    return
  }
  if (el.paused) {
    userPaused.value = false
    void tryPlay()
  } else {
    userPaused.value = true
    pause()
  }
}

/** KeepAlive 切回父级时调用：恢复播放（不改变当前时间）。 */
function resume() {
  if (!props.active || !canInteract.value || props.item.media_type !== 'video') return
  const el = videoEl.value
  if (!el || !el.paused) return
  userPaused.value = false
  void tryPlay()
}

/** 供父级长按左右方向键调用：正数为快进，负数为回退。 */
function seekBy(seconds: number) {
  const el = videoEl.value
  if (!el || !Number.isFinite(el.duration)) return
  el.currentTime = Math.max(0, Math.min(el.duration - 0.05, el.currentTime + seconds))
  videoEnded.value = false
  current.value = el.currentTime
  resetWatchObservation()
  enterMinimal()
  showSeekHud(`${formatDuration(el.currentTime)} / ${formatDuration(el.duration)}`)
  showInteractionHud(1200)
  emitProgress(true)
}

/** 绝对路径跳转（从头开始按钮使用）。 */
function seekTo(seconds: number) {
  const el = videoEl.value
  if (!el) return
  el.currentTime = Math.max(0, seconds)
  videoEnded.value = false
  current.value = el.currentTime
  resetWatchObservation()
  resumeDone = true
  pendingResume = null
  userPaused.value = false
  void tryPlay()
  emitProgress(true)
}

/** 播放结束后的重播（含全屏/沉浸下由封面层点击触发）。 */
function replayFromStart() {
  const el = videoEl.value
  if (!el) return
  videoEnded.value = false
  userPaused.value = false
  try {
    el.currentTime = 0
  } catch {
    /* 元数据未就绪时忽略，play 后会自动从头 */
  }
  current.value = 0
  resetWatchObservation()
  showInteractionHud(900)
  void tryPlay()
}

function getCurrentTime() {
  return Number((videoEl.value?.currentTime ?? 0).toFixed(2))
}

function isPaused() {
  // 直接读元素状态：pause() 的 pause 事件是异步派发的，ref 可能还是旧值。
  return videoEl.value?.paused ?? paused.value
}

/** 元数据/首帧就绪后再触发播放，避免加载竞态导致静默失败。 */
function onCanPlay() {
  // 结束态回退到最后一帧时浏览器可能再派发 canplay；此时不能自动重播，
  // 否则全屏下会先闪黑再从头播放，看起来像「播完黑屏」。
  if (props.active && !userPaused.value && !mediaError.value && !videoEnded.value) void tryPlay()
  else if (!props.active && !videoEl.value?.paused) videoEl.value?.pause()
}

function applyResume() {
  if (resumeDone || pendingResume == null || pendingResume <= 0) return
  const el = videoEl.value
  if (!el || !Number.isFinite(el.duration) || el.duration <= 0) return
  const target = Math.min(pendingResume, Math.max(0, el.duration - 0.3))
  if (target > 0.3) {
    el.currentTime = target
    current.value = target
    resetWatchObservation()
  }
  resumeDone = true
  pendingResume = null
}

/** 只有 video 真正开始播放才算「开始观看」；预加载不会触发该事件。 */
function onPlay() {
  hasStarted = true
  lastObservedTime = videoEl.value?.currentTime ?? null
  lastTickAt = performance.now()
  // 立即通知页面写入历史，避免 5 秒节流导致刚播放的视频不出现在历史里。
  emitProgress(true, true)
}

function onPause() {
  lastObservedTime = null
  emitProgress(true)
}

function onTimeUpdate() {
  const el = videoEl.value
  if (!el || seeking.value) return
  const nowTs = performance.now()
  // 只累计自然播放推进：跳播后 lastObservedTime 被清空，这一帧不计入。
  if (!el.paused && props.active && lastObservedTime !== null) {
    const advanced = el.currentTime - lastObservedTime
    const wall = Math.max(0.01, (nowTs - lastTickAt) / 1000)
    const maxNatural = Math.max(0.75, wall * effectiveRate.value * 1.6)
    if (advanced > 0 && advanced <= maxNatural) pendingWatched += Math.min(advanced, 2)
  }
  lastObservedTime = el.currentTime
  lastTickAt = nowTs
  current.value = el.currentTime
  if (el.duration && Number.isFinite(el.duration)) duration.value = el.duration
  emitProgress()
}

function seekByRatio(ratio: number) {
  const el = videoEl.value
  if (!el || !duration.value) return
  el.currentTime = Math.max(0, Math.min(1, ratio)) * duration.value
  resetWatchObservation()
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

/**
 * 视频自然结束：交由页面按 endAction 处理（pause 停住 / loop 重播 / next 切换）。
 * 全屏下 ended 的 <video> 常常只剩黑帧，因此统一进入「结束态」显示封面 + 重播层；
 * 即使父级没有可切换的下一条，也不会停在黑屏。
 */
function onVideoEnded() {
  videoEnded.value = true
  playing.value = false
  paused.value = true
  hideInteractionHud()
  emitProgress(true)
  if (props.endAction === 'loop') replayFromStart()
  else if (props.endAction === 'next') emit('ended', props.item.key)
  else {
    const el = videoEl.value
    // 停在可解码的最后一帧，避免部分浏览器在 ended 后直接清成黑帧。
    if (el && Number.isFinite(el.duration) && el.duration > 0) {
      try {
        el.currentTime = Math.max(0, el.duration - 0.08)
      } catch {
        /* ignore */
      }
      resetWatchObservation()
    }
  }
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
    videoEnded.value = false
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
  exitMinimal()
  endFullscreenIfOwned()
}

watch(
  () => props.active,
  (active) => (active ? activate() : deactivate()),
)

/** 从后台 / bfcache 返回时恢复当前条目，避免黑屏有声或停在旧画面。 */
function onVisibilityChange() {
  if (!props.active) return
  if (document.visibilityState === 'visible') {
    const el = videoEl.value
    // 已停在其他标签页期间结束的视频不要自动重播，保留封面 + 重播提示即可。
    if (videoEnded.value || el?.ended) return
    if (el && el.paused && !userPaused.value && canRender.value) void tryPlay()
  } else {
    emitProgress(true)
  }
}

// 首屏首个条目在组件挂载时即为 active，需要在 DOM 就绪后触发播放。
onMounted(() => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('webkitendfullscreen', onWebkitEndFullscreen)
  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('orientationchange', onOrientationChange)
  if (props.active) activate()
})

// <video> 元素在 v-if 渲染完成后才可用，元素出现时补一次播放尝试。
watch(videoEl, (el, oldEl) => {
  if (oldEl) unregisterVideo(oldEl)
  if (el) {
    registerVideo(el)
    el.muted = props.muted || forcedMute.value
    el.volume = Math.max(0, Math.min(1, props.volume ?? 1))
    el.playbackRate = effectiveRate.value
    applyResume()
    if (props.active) void tryPlay()
    else if (!el.paused) el.pause()
  }
})

watch(
  () => props.muted,
  (muted) => {
    if (videoEl.value) videoEl.value.muted = muted || forcedMute.value
  },
)

watch(
  () => props.volume,
  (volume) => {
    if (videoEl.value) videoEl.value.volume = Math.max(0, Math.min(1, volume ?? 1))
  },
  { immediate: true },
)

watch(effectiveRate, (rate) => {
  if (videoEl.value) videoEl.value.playbackRate = rate
})

/** 沉浸模式退出按钮：开启后 3 秒转为半透明，hover/触摸时恢复清晰。 */
watch(
  () => props.immersive,
  (on) => {
    if (immersiveTimer !== null) window.clearTimeout(immersiveTimer)
    immersiveTimer = null
    immersiveExitFaded.value = false
    if (on) {
      immersiveTimer = window.setTimeout(() => {
        immersiveExitFaded.value = true
        immersiveTimer = null
      }, 3000)
    }
  },
  { immediate: true },
)

/** 沉浸模式倍速恢复按钮：与退出沉浸按钮同样 3 秒后转半透明，hover/触摸恢复。 */
watch(
  [rateOverride, () => props.immersive],
  ([rate, immersive]) => {
    if (rateRestoreTimer !== null) window.clearTimeout(rateRestoreTimer)
    rateRestoreTimer = null
    rateRestoreFaded.value = false
    if (immersive && rate !== null && rate !== 1) {
      rateRestoreTimer = window.setTimeout(() => {
        rateRestoreFaded.value = true
        rateRestoreTimer = null
      }, 3000)
    }
  },
  { immediate: true },
)

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
  isFullscreen.value = cssFullscreen.value || document.fullscreenElement === rootEl.value
}

/** iOS <video> 私有全屏退出（不触发标准 fullscreenchange）也要同步状态。 */
function onWebkitEndFullscreen() {
  isFullscreen.value = cssFullscreen.value
}

function orientationApi() {
  return screen.orientation as ScreenOrientation & {
    lock?: (o: string) => Promise<void>
    unlock?: () => Promise<void>
  }
}

async function lockLandscape() {
  try {
    await orientationApi().lock?.('landscape')
  } catch {
    /* 浏览器/系统不支持锁定时保持当前方向 */
  }
}

function endFullscreenIfOwned() {
  if (cssFullscreen.value) {
    cssFullscreen.value = false
    document.documentElement.classList.remove('reel-css-fullscreen-host')
  }
  if (rootEl.value && document.fullscreenElement === rootEl.value) {
    void document.exitFullscreen().catch(() => undefined)
  }
  try {
    const orientation = orientationApi()
    if (orientation.unlock) void Promise.resolve(orientation.unlock()).catch(() => undefined)
  } catch {
    /* ignore */
  }
  isFullscreen.value = false
}

/** 横竖屏变化后让父级重新对齐当前条目，避免全屏旋转后滚动错位。 */
function onOrientationChange() {
  window.dispatchEvent(new Event('resize'))
}

async function toggleFullscreen(rotate = false) {
  if (isFullscreen.value) {
    endFullscreenIfOwned()
    return
  }
  if (isTouchDevice) {
    // 手机端先尝试真正的原生全屏；iOS 等不支持时退回 CSS 铺满。
    try {
      await rootEl.value?.requestFullscreen?.()
      isFullscreen.value = true
      if (rotate) await lockLandscape()
      return
    } catch {
      /* 原生全屏不可用，使用 CSS 全屏 */
    }
    cssFullscreen.value = true
    document.documentElement.classList.add('reel-css-fullscreen-host')
    isFullscreen.value = true
    if (rotate) await lockLandscape()
    return
  }
  if (document.fullscreenElement) await document.exitFullscreen().catch(() => undefined)
  try {
    await rootEl.value?.requestFullscreen?.()
    if (rotate) await lockLandscape()
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

/**
 * 长按进入倍速（参考 Kazumi）：
 * - 首次长按默认 1.5×，不需要先右滑确认；
 * - 已在倍速模式时再次长按则从当前倍速继续调节；
 * - 中央面板只在按住期间显示，松手收起但倍速保持。
 */
function beginRateMode(adjustCurrent = false) {
  if (!props.active || !canRender.value) return
  longPressed = true
  swiping = false
  rateSelecting.value = true
  rateDragging.value = true
  // 长按加速时立即隐藏其他 UI（极简模式由手势持有，松手才恢复）。
  enterMinimal(0)
  showInteractionHud(0)
  rateStartY = lastY
  if (adjustCurrent && rateOverride.value !== null) {
    const index = RATE_OPTIONS.findIndex((r) => r.value === rateOverride.value)
    rateStartIndex = index >= 0 ? index : 3
  } else {
    // Kazumi 默认：长按即 1.5×。
    rateOverride.value = 1.5
    rateStartIndex = Math.max(0, RATE_OPTIONS.findIndex((r) => r.value === 1.5))
  }
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
  // 倍速调整期间刷新时间点；极简模式已由长按进入，松手时统一退出。
  const el = videoEl.value
  if (el) showSeekHud(`${formatDuration(el.currentTime)} / ${formatDuration(el.duration)}`, 1600)
  showInteractionHud(0)
}

/** 点击底部提示：退出倍速模式，恢复设置页默认倍速（默认 1×）。 */
function exitRateMode() {
  rateOverride.value = null
  rateSelecting.value = false
  rateDragging.value = false
  exitMinimal()
  showInteractionHud(900)
  if (videoEl.value) videoEl.value.playbackRate = props.rate ?? 1
  logger.info('interaction', 'player', '退出倍速模式', { video_key: props.item.key })
}

/** 手势拖动只改变 currentTime，不触发自动播放/续播逻辑。 */
function previewSeek(target: number) {
  const el = videoEl.value
  if (!el || !Number.isFinite(el.duration) || el.duration <= 0) return
  const next = Math.max(0, Math.min(el.duration - 0.05, target))
  el.currentTime = next
  videoEnded.value = false
  current.value = next
  resetWatchObservation()
  showGestureHud(`${formatDuration(next)} / ${formatDuration(el.duration)}`)
  // 沉浸模式没有底部控制条，这里临时弹出进度条，松手后再自动收起。
  showInteractionHud(0)
}

/** 全屏竖滑调音量：整屏高度对应 0~1，松手后由父级保存。 */
function applyVolumeDrag(clientY: number) {
  const el = videoEl.value
  if (!el) return
  const height = Math.max(1, rootEl.value?.clientHeight || window.innerHeight)
  const volume = Math.max(0, Math.min(1, gestureStartVolume + (gestureStartY - clientY) / height))
  el.volume = volume
  emit('volume-change', { volume, muted: volume <= 0.001 })
  showGestureHud(`音量 ${Math.round(volume * 100)}%`)
}

function resetGestureMode() {
  gestureMode = 'none'
}

function onGestureDown(event: PointerEvent) {
  if (!canInteract.value) return
  if (event.button !== undefined && event.button !== 0) return
  pointerId = event.pointerId
  startX = lastX = event.clientX
  startY = lastY = event.clientY
  gestureStartY = event.clientY
  gestureStartCurrent = videoEl.value?.currentTime ?? 0
  gestureStartVolume = props.volume ?? videoEl.value?.volume ?? 1
  gestureMode = 'none'
  clearPressTimer()
  // 单击始终是暂停/播放；再次长按才进入调速（从当前倍速起调）。
  longPressed = false
  swiping = false
  rateDragging.value = false
  const adjustCurrent = rateOverride.value !== null
  pressTimer = window.setTimeout(() => beginRateMode(adjustCurrent), 500)
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
}

function onGestureMove(event: PointerEvent) {
  if (pointerId !== event.pointerId || !canInteract.value) return
  lastX = event.clientX
  lastY = event.clientY
  if (rateSelecting.value && rateDragging.value) {
    // 长按倍速期间竖滑选档；与快进/音量手势互斥。
    event.preventDefault()
    applyRateDrag(event.clientY)
    return
  }

  const dx = event.clientX - startX
  const dy = event.clientY - startY
  const gesturePlayer = isFullscreen.value || props.immersive === true

  if (gestureMode === 'seek') {
    event.preventDefault()
    const el = videoEl.value
    if (el && Number.isFinite(el.duration) && el.duration > 0) {
      const scale = el.duration / Math.max(1, rootEl.value?.clientWidth || window.innerWidth)
      previewSeek(gestureStartCurrent + dx * scale)
    }
    return
  }
  if (gestureMode === 'volume') {
    event.preventDefault()
    applyVolumeDrag(event.clientY)
    return
  }
  if (longPressed) return

  if (gesturePlayer && Math.abs(dx) > 12 && Math.abs(dx) > Math.abs(dy)) {
    // 方向锁定为横向：全屏/沉浸下拖动快进。
    clearPressTimer()
    gestureMode = 'seek'
    swiping = false
    event.preventDefault()
    return
  }
  if (isFullscreen.value && Math.abs(dy) > 12 && Math.abs(dy) > Math.abs(dx)) {
    // 方向锁定为纵向：全屏下调节音量，绝不切换视频。
    clearPressTimer()
    gestureMode = 'volume'
    swiping = false
    event.preventDefault()
    applyVolumeDrag(event.clientY)
    return
  }
  if (!isFullscreen.value && Math.abs(dy) > 14) {
    // 普通模式与非全屏沉浸保留纵向切换；首页第一条下拉刷新。
    // 全屏已由上面的音量手势接管，不会误切视频。
    clearPressTimer()
    swiping = true
  }
}

function onGestureUp(event: PointerEvent) {
  clearPressTimer()
  if (pointerId !== event.pointerId) return
  pointerId = null
  const inRateSelection = rateSelecting.value || rateDragging.value
  const finishedMode = gestureMode
  rateDragging.value = false
  rateSelecting.value = false
  resetGestureMode()
  ;(event.currentTarget as HTMLElement).releasePointerCapture?.(event.pointerId)

  if (finishedMode === 'seek') {
    emitProgress(true)
    showInteractionHud(900)
    suppressNextClick = true
    return
  }
  if (finishedMode === 'volume') {
    suppressNextClick = true
    return
  }
  if (inRateSelection) {
    // Kazumi：松手即保持所选倍速，中央面板收起并立即恢复完整 UI。
    exitMinimal()
    showInteractionHud(900)
    suppressNextClick = true
    return
  }
  if (swiping) {
    const dy = event.clientY - startY
    suppressNextClick = true
    if (dy > 40) {
      // 首页第一条下拉刷新（抖音式）；全屏竖滑已被音量占用，不会误触发。
      if (props.allowRefresh && !isFullscreen.value) emit('refresh')
      else emit('retreat')
    } else if (dy < -40) {
      emit('advance')
    }
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
  rateSelecting.value = false
  swiping = false
  resetGestureMode()
  exitMinimal()
  hideInteractionHud()
}

onBeforeUnmount(() => {
  clearImageTimer()
  exitMinimal()
  hideInteractionHud()
  if (seekHudTimer !== null) window.clearTimeout(seekHudTimer)
  if (immersiveTimer !== null) window.clearTimeout(immersiveTimer)
  if (interactionTimer !== null) window.clearTimeout(interactionTimer)
  if (rateRestoreTimer !== null) window.clearTimeout(rateRestoreTimer)
  if (gestureHudTimer !== null) window.clearTimeout(gestureHudTimer)
  emitProgress(true)
  pause()
  unregisterVideo(videoEl.value)
  clearPressTimer()
  document.removeEventListener('pointerdown', onFirstGesture)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('webkitendfullscreen', onWebkitEndFullscreen)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  window.removeEventListener('orientationchange', onOrientationChange)
  endFullscreenIfOwned()
})

// 降级静音后，任意点击/触摸都可恢复声音（浏览器要求在手势回调中调用 play）。
watch(forcedMute, (v) => {
  if (v) document.addEventListener('pointerdown', onFirstGesture, { passive: true })
  else document.removeEventListener('pointerdown', onFirstGesture)
})

defineExpose({
  togglePlay,
  resume,
  seekBy,
  seekTo,
  replayFromStart,
  getCurrentTime,
  isPaused,
  getRate: () => effectiveRate.value,
  toggleFullscreen,
})
</script>

<template>
  <div
    ref="rootEl"
    class="relative h-full w-full overflow-hidden bg-black"
    :class="{ 'reel-css-fullscreen': cssFullscreen }"
  >
    <!-- 不可播放：模糊预览 + 原因说明（F-16） -->
    <div v-if="!item.playable" class="reel-player-ui absolute inset-0">
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
        :loop="endAction === 'loop'"
        playsinline
        webkit-playsinline
        @play="onPlay(); playing = true; paused = false"
        @pause="onPause(); playing = false; paused = true"
        @ended="onVideoEnded"
        @timeupdate="onTimeUpdate"
        @loadedmetadata="onLoadedMetadata"
        @canplay="onCanPlay"
        @error="onVideoError"
      ></video>
      <!-- 播放结束态：原生全屏下 ended 的 video 常只显示黑帧，这里盖一层封面与重播提示。
           不带 reel-player-ui 类，沉浸模式 / 极简模式下同样可见。 -->
      <div
        v-if="videoEnded && !mediaError"
        data-role="ended-overlay"
        class="absolute inset-0 z-[4] flex cursor-pointer items-center justify-center"
        title="点击重播"
        @pointerdown.stop
        @click.stop="replayFromStart"
      >
        <img
          v-if="thumb"
          :src="thumb"
          class="absolute inset-0 h-full w-full object-contain opacity-85"
          alt=""
        />
        <div class="absolute inset-0 bg-black/35"></div>
        <div class="relative flex flex-col items-center gap-2 rounded-full bg-black/55 px-7 py-5 backdrop-blur-md">
          <svg class="h-9 w-9 text-white" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5.5v13l11-6.5z" />
          </svg>
          <span class="text-xs text-white/85">播放完毕 · 点击重播</span>
        </div>
      </div>
      <div v-else-if="mediaError" class="flex h-full w-full flex-col items-center justify-center gap-2 px-8 text-center">
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
        v-if="active && item.media_type === 'video' && !playing && !mediaError && !minimalUI"
        class="reel-player-ui pointer-events-none absolute inset-0 z-[3] flex items-center justify-center"
      >
        <div class="rounded-full bg-black/40 p-5 backdrop-blur">
          <svg class="h-10 w-10 text-white" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5.5v13l11-6.5z" />
          </svg>
        </div>
      </div>
    </transition>

    <!-- 倍速选择面板：按住期间居中半透明显示，松手即收起但倍速保持（参考 Kazumi） -->
    <div
      v-if="active && rateSelecting"
      class="reel-player-ui pointer-events-none absolute left-1/2 top-1/2 z-20 -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-white/25 bg-black/45 px-7 py-5 text-center text-white shadow-xl backdrop-blur-md"
    >
      <div class="text-3xl font-semibold tracking-wide">{{ rateHint }}</div>
      <div class="mt-1.5 text-[11px] text-white/70">上下滑动调速 · 松开保持</div>
    </div>

    <!-- 全屏 / 沉浸手势反馈：不受沉浸模式隐藏 -->
    <transition name="fade">
      <div
        v-if="gestureHud"
        class="pointer-events-none absolute left-1/2 top-1/2 z-[65] -translate-x-1/2 -translate-y-1/2 rounded-xl bg-black/65 px-4 py-2 text-sm font-medium tabular-nums text-white shadow-xl backdrop-blur"
      >
        {{ gestureHud }}
      </div>
    </transition>

    <!-- 沉浸模式的临时进度反馈：快进/调速时弹出，操作结束 0.9 秒后收起。
         不使用 reel-player-ui 类，否则会被沉浸模式的全局隐藏规则一起隐藏。 -->
    <div
      v-if="active && immersive && interactionHud && item.playable"
      data-role="immersive-hud"
      class="pointer-events-none absolute inset-x-0 bottom-0 z-[60] px-5 pb-[max(1.25rem,env(safe-area-inset-bottom))]"
    >
      <div class="relative h-1 rounded-full bg-white/25">
        <div class="h-full rounded-full bg-white/90" :style="{ width: `${displayRatio * 100}%` }"></div>
      </div>
      <div class="mt-2 flex items-center justify-between text-[11px] tabular-nums text-white/90 drop-shadow">
        <span>{{ formatDuration(current) }} / {{ formatDuration(duration) }}</span>
        <span v-if="rateOverride !== null">{{ rateHint }} 倍速</span>
      </div>
    </div>

    <!-- 沉浸模式倍速恢复按钮：3 秒后与退出沉浸按钮一样半透明，触摸/hover 恢复 -->
    <button
      v-if="active && immersive && rateOverride !== null && rateOverride !== 1 && !rateSelecting"
      data-role="rate-restore"
      class="absolute bottom-[max(4.5rem,env(safe-area-inset-bottom))] right-4 z-[60] flex h-8 items-center gap-1 rounded-full border border-white/25 bg-black/55 px-3 text-[11px] text-white backdrop-blur transition-opacity duration-700 hover:opacity-100"
      :class="rateRestoreFaded ? 'opacity-25' : 'opacity-100'"
      @pointerdown.stop
      @click.stop="exitRateMode"
    >
      <span class="font-semibold">{{ rateHint }}</span>
      <span class="text-white/60">恢复原速</span>
    </button>

    <!-- 底部控制条：进度 / 时间 / 播放 / 静音 / 全屏 -->
    <div
      v-if="active && item.playable"
      class="reel-player-ui absolute inset-x-0 bottom-0 z-10 px-4 pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-8"
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

      <!-- 快进/倍速时的居中时间 HUD（v0.3.0 需求 4） -->
      <transition name="fade">
        <div
          v-if="minimalUI && seekHud"
          class="pointer-events-none absolute inset-x-0 -top-1 flex justify-center"
        >
          <span class="rounded-lg bg-black/70 px-3 py-1 text-sm font-medium tabular-nums text-white backdrop-blur">
            {{ seekHud }}
          </span>
        </div>
      </transition>

      <div class="mt-1 flex items-center justify-between gap-2 text-[11px] text-white/80">
        <div class="flex items-center gap-2">
          <button
            v-if="item.media_type === 'video' && !minimalUI"
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
        <div v-show="!minimalUI" class="flex items-center gap-1.5">
          <!-- 松手后倍速保持，底部半透明提示单击即恢复原速 -->
          <button
            v-if="rateOverride !== null && rateOverride !== 1 && !rateSelecting"
            class="flex h-7 items-center gap-1 rounded-full border border-white/25 bg-black/40 px-2.5 text-[11px] text-white/90 backdrop-blur transition hover:bg-black/60 active:scale-95"
            :title="`当前 ${rateHint}，单击退出倍速模式并恢复原速`"
            @pointerdown.stop
            @click.stop="exitRateMode"
          >
            <span class="font-semibold">{{ rateHint }}</span>
            <span class="text-[10px] text-white/55">恢复原速</span>
          </button>
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
            class="flex h-9 w-12 items-center justify-center rounded-full border border-white/15 bg-black/45 text-[11px] text-white/90 backdrop-blur transition hover:bg-black/65 active:scale-90"
            :title="immersive ? '退出沉浸模式' : '沉浸模式（隐藏全部 UI）'"
            @pointerdown.stop
            @click.stop="emit('toggle-immersive')"
          >
            沉浸
          </button>
          <button
            class="flex h-9 w-9 items-center justify-center rounded-full border border-white/15 bg-black/45 backdrop-blur transition hover:bg-black/65 active:scale-90"
            :title="isFullscreen ? '退出全屏' : '全屏（手机端横屏）'"
            @pointerdown.stop
            @click.stop="void toggleFullscreen(true)"
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

    <!-- 沉浸模式唯一保留的 UI：开启 3 秒后半透明，hover/触摸恢复清晰 -->
    <button
      v-if="active && immersive"
      class="absolute right-4 top-[max(1rem,env(safe-area-inset-top))] z-[70] flex h-10 items-center gap-1.5 rounded-full border border-white/25 bg-black/55 px-3 text-xs text-white backdrop-blur transition-opacity duration-700 hover:opacity-100"
      :class="immersiveExitFaded ? 'opacity-25' : 'opacity-100'"
      title="退出沉浸模式（Esc）"
      @pointerdown.stop
      @click.stop="emit('toggle-immersive')"
    >
      <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9">
        <path d="M9 9H4V4m11 0h5v5M9 15H4v5m11 0h5v-5" />
      </svg>
      退出沉浸
    </button>
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
/* 移动端 CSS 全屏：不调用原生 Fullscreen API，旋转屏幕不会重载页面 */
.reel-css-fullscreen {
  position: fixed !important;
  inset: 0;
  z-index: 9999;
  width: 100vw;
  height: 100vh;
  height: 100dvh;
}
:fullscreen {
  background: #000;
}
:fullscreen video,
:fullscreen img {
  object-fit: contain;
}
</style>
