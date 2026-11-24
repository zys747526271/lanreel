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
}>()

const emit = defineEmits<{
  (e: 'advance'): void
  (e: 'blocked', reason: string): void
  (e: 'toggle-favorite', key: string): void
  (e: 'player-error', payload: { key: string; message: string }): void
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
  try {
    await el.play()
    playing.value = true
  } catch {
    // 浏览器自动播放策略限制：保持静音状态后可重试，不视为错误
    playing.value = false
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
  if (!el) return
  current.value = el.currentTime
  if (el.duration && Number.isFinite(el.duration)) duration.value = el.duration
}

function seekByRatio(ratio: number) {
  const el = videoEl.value
  if (!el || !duration.value) return
  el.currentTime = Math.max(0, Math.min(1, ratio)) * duration.value
}

function onProgressPointer(event: PointerEvent) {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  const ratio = (event.clientX - rect.left) / rect.width
  seekByRatio(ratio)
  if (event.type === 'pointermove' && event.buttons !== 1) return
  const move = (e: PointerEvent) => onProgressPointer(e as PointerEvent)
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  if (event.type === 'pointerdown') {
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
}

function onLoadedMetadata() {
  const el = videoEl.value
  if (el && Number.isFinite(el.duration)) duration.value = el.duration
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
    el.muted = props.muted
    if (props.active) void tryPlay()
  }
})

watch(
  () => props.muted,
  (muted) => {
    if (videoEl.value) videoEl.value.muted = muted
  },
)

onBeforeUnmount(() => {
  clearImageTimer()
  pause()
})
</script>

<template>
  <div class="relative h-full w-full overflow-hidden bg-black">
    <!-- 不可播放：模糊预览 + 原因说明（F-16） -->
    <div v-if="!item.playable" class="absolute inset-0">
      <img
        v-if="item.media_type === 'image' && thumb"
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

      <!-- 进度条（可拖拽，F-04） -->
      <div class="absolute inset-x-0 bottom-0 z-10 px-4 pb-3 pt-6">
        <div
          class="group relative h-6 cursor-pointer touch-none select-none"
          @pointerdown="onProgressPointer"
          @pointermove="onProgressPointer"
        >
          <div class="absolute top-1/2 h-[3px] w-full -translate-y-1/2 rounded-full bg-white/25">
            <div
              class="h-full rounded-full bg-white/90"
              :style="{ width: duration ? `${Math.min(100, (current / duration) * 100)}%` : '0%' }"
            ></div>
          </div>
        </div>
        <div class="mt-1 flex items-center justify-between text-[11px] text-white/70">
          <span>{{ formatDuration(current) }}</span>
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
