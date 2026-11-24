<script setup lang="ts">
/**
 * 收藏夹 / 观看历史的封面图（参考 YouTube hover preview / peekbar 的做法）：
 *
 *  - 服务端缩略图优先；HEAD 探测失败时自动懒抽首帧并缓存到 IndexedDB，
 *    卡片进入视口即生成，不需要用户触碰；
 *  - 悬停时用隐藏的 <video> 静音连续播放（不碰主播放器），canvas 每 ~120ms
 *    抽一帧显示在卡片上，进度条平滑推进，约 10s  preview 完一段片段；
 *  - 抽到的帧作为封面回退缓存，下次打开网格直接显示。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { withToken } from '@/api/client'
import { getCachedThumb, saveCachedThumb } from '@/lib/thumbCache'
import type { MediaItem } from '@/api/types'

const props = defineProps<{ item: MediaItem }>()

const rootEl = ref<HTMLElement | null>(null)
/** 自动抽取 / 缓存的封面 */
const frame = ref('')
/** 悬停预览期间临时显示的帧（离开后清除，回落到 frame / serverThumb） */
const previewFrame = ref('')
const previewing = ref(false)
const progress = ref(0)
const thumbFailed = ref(false)

const serverThumb = computed(() =>
  props.item.thumb_url && !thumbFailed.value ? withToken(props.item.thumb_url) : '',
)
const displaySrc = computed(() => previewFrame.value || frame.value || serverThumb.value)

/* --------------------------- 自动抽帧队列 --------------------------- */
/** 限制同时解码的视频数，避免打开收藏夹时一次性拉几十个视频。 */
const AUTO_LIMIT = 2
const autoQueue: Array<() => Promise<void>> = []
let autoRunning = 0

function enqueueAuto(task: () => Promise<void>) {
  autoQueue.push(task)
  void drainAuto()
}

async function drainAuto() {
  while (autoRunning < AUTO_LIMIT && autoQueue.length) {
    const task = autoQueue.shift()!
    autoRunning += 1
    try {
      await task()
    } finally {
      autoRunning -= 1
      void drainAuto()
    }
  }
}

/* ------------------------------ 通用工具 ------------------------------ */
let disposed = false
let observer: IntersectionObserver | null = null
let hoverTimer: number | null = null
let previewVideo: HTMLVideoElement | null = null
let previewRaf = 0
let previewTimer: number | null = null
let previewStopped = false
let cachedThisHover = false

/** 仅用于诊断：当前正在悬停预览的组件。 */
let activePreviewStop: (() => void) | null = null

function createHiddenVideo(): HTMLVideoElement {
  const el = document.createElement('video')
  el.style.cssText =
    'position:fixed;left:-9999px;top:0;width:2px;height:2px;opacity:0;pointer-events:none;'
  el.setAttribute('aria-hidden', 'true')
  el.muted = true
  el.playsInline = true
  el.preload = 'auto'
  // detached <video> 在 Chromium 不加载元数据，必须挂到 DOM（不可见）。
  document.body.appendChild(el)
  return el
}

function destroyVideo(el: HTMLVideoElement | null) {
  if (!el) return
  el.pause()
  el.removeAttribute('src')
  el.load()
  el.remove()
}

function waitForMetadata(el: HTMLVideoElement, timeout = 6000): Promise<boolean> {
  return new Promise((resolve) => {
    let done = false
    const finish = (ok: boolean) => {
      if (done) return
      done = true
      window.clearTimeout(timer)
      resolve(ok)
    }
    const timer = window.setTimeout(() => finish(el.readyState >= 1), timeout)
    el.addEventListener('loadedmetadata', () => finish(true), { once: true })
    el.addEventListener('error', () => finish(false), { once: true })
  })
}

function waitForSeek(el: HTMLVideoElement, time: number, timeout = 2500): Promise<boolean> {
  return new Promise((resolve) => {
    if (Math.abs(el.currentTime - time) < 0.06 && el.readyState >= 2) {
      resolve(true)
      return
    }
    let done = false
    const finish = (ok: boolean) => {
      if (done) return
      done = true
      window.clearTimeout(timer)
      el.removeEventListener('seeked', onSeeked)
      resolve(ok)
    }
    const onSeeked = () => finish(true)
    const timer = window.setTimeout(() => finish(el.readyState >= 2), timeout)
    el.addEventListener('seeked', onSeeked)
    try {
      el.currentTime = time
    } catch {
      finish(false)
    }
  })
}

interface GrabbedFrame {
  data: string
  /** 平均亮度，用于跳过片头的纯黑帧 */
  brightness: number
}

function grabFrame(el: HTMLVideoElement, maxWidth = 320): GrabbedFrame | null {
  if (disposed || el.readyState < 2 || !el.videoWidth) return null
  try {
    const canvas = document.createElement('canvas')
    const scale = Math.min(1, maxWidth / el.videoWidth)
    canvas.width = Math.max(1, Math.round(el.videoWidth * scale))
    canvas.height = Math.max(1, Math.round(el.videoHeight * scale))
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) return null
    ctx.drawImage(el, 0, 0, canvas.width, canvas.height)
    const data = canvas.toDataURL('image/jpeg', 0.72)
    const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data
    let sum = 0
    let count = 0
    const stride = Math.max(4, Math.floor(pixels.length / 4 / 200) * 4)
    for (let i = 0; i < pixels.length; i += stride) {
      sum += (pixels[i] + pixels[i + 1] + pixels[i + 2]) / 3
      count += 1
    }
    return { data, brightness: count ? sum / count : 0 }
  } catch {
    return null
  }
}

function cacheFrame(data: string) {
  if (cachedThisHover) return
  cachedThisHover = true
  void saveCachedThumb(props.item.key, data)
}

/* ------------------------- 自动首帧（无需触碰） ------------------------- */
function queueFirstFrame() {
  if (props.item.media_type !== 'video' || !props.item.playable) return
  enqueueAuto(async () => {
    if (disposed || frame.value || serverThumb.value) return
    const cached = await getCachedThumb(props.item.key)
    if (disposed || cached) {
      if (cached && !disposed) frame.value = cached
      return
    }
    const el = createHiddenVideo()
    try {
      el.src = withToken(props.item.stream_url)
      if (!(await waitForMetadata(el, 5000)) || disposed) return
      if (!Number.isFinite(el.duration) || el.duration <= 0) return
      // 片头常为黑帧：依次尝试 0.1s / 0.8s / 2s / 5%，取第一个足够亮的帧。
      const candidates = [0.1, 0.8, 2, Math.min(5, el.duration * 0.08)]
      let picked: string | null = null
      for (const candidate of candidates) {
        if (disposed) return
        const time = Math.max(0, Math.min(candidate, el.duration - 0.06))
        if (!(await waitForSeek(el, time, 2500))) continue
        const shot = grabFrame(el)
        if (!shot) continue
        if (!picked) picked = shot.data
        if (shot.brightness > 16) {
          picked = shot.data
          break
        }
      }
      if (picked && !disposed && !frame.value && !serverThumb.value) {
        frame.value = picked
        void saveCachedThumb(props.item.key, picked)
      }
    } finally {
      destroyVideo(el)
    }
  })
}

/* --------------------------- 悬停连续预览 --------------------------- */
function stopPreview() {
  if (activePreviewStop === stopPreview) activePreviewStop = null
  previewStopped = true
  if (hoverTimer !== null) window.clearTimeout(hoverTimer)
  if (previewRaf) cancelAnimationFrame(previewRaf)
  if (previewTimer !== null) window.clearInterval(previewTimer)
  hoverTimer = null
  previewRaf = 0
  previewTimer = null
  previewing.value = false
  previewFrame.value = ''
  progress.value = 0
  const el = previewVideo
  previewVideo = null
  destroyVideo(el)
}

async function startPreview() {
  if (disposed || props.item.media_type !== 'video' || !props.item.playable) return
  if (activePreviewStop) activePreviewStop()
  previewStopped = false
  cachedThisHover = false
  previewing.value = true
  activePreviewStop = stopPreview

  const el = createHiddenVideo()
  previewVideo = el
  try {
    el.src = withToken(props.item.stream_url)
    if (!(await waitForMetadata(el, 6000)) || previewStopped || disposed) {
      stopPreview()
      return
    }
    const duration = el.duration
    if (!Number.isFinite(duration) || duration <= 0) {
      stopPreview()
      return
    }
    // 约 10s 预览一段：长视频取 48s 片段，短视频按 1× 播完，速度不突兀。
    const span = Math.min(Math.max(6, duration * 0.6), 48)
    const rate = Math.max(1, Math.min(8, span / 10))
    const start = Math.min(0.2, duration * 0.02)
    el.currentTime = start
    el.playbackRate = rate
    const startedAt = performance.now()
    let lastDraw = 0

    const tick = (now: number) => {
      if (previewStopped || disposed) return
      if (now - lastDraw >= 120 && el.readyState >= 2 && el.videoWidth) {
        lastDraw = now
        const shot = grabFrame(el, 240)
        if (shot) {
          previewFrame.value = shot.data
          if (!frame.value && !serverThumb.value) {
            frame.value = shot.data
            cacheFrame(shot.data)
          }
        }
        progress.value = Math.min(1, Math.max(0, (el.currentTime - start) / span))
      }
      if (el.currentTime >= start + span || now - startedAt > 12000) {
        stopPreview()
        return
      }
      previewRaf = requestAnimationFrame(tick)
    }

    try {
      await el.play()
      if (!previewStopped && !disposed) previewRaf = requestAnimationFrame(tick)
    } catch {
      // 自动播放失败时退化为细步长 seek 预览（0.9s 一帧，比原来的 2s 更平滑）。
      let step = 0
      const total = 9
      previewTimer = window.setInterval(() => {
        if (previewStopped || disposed) return
        const t = Math.min(duration - 0.06, start + (span * (step + 1)) / total)
        void waitForSeek(el, t, 1200).then((ok) => {
          if (!ok || previewStopped || disposed) return
          const shot = grabFrame(el, 240)
          if (shot) {
            previewFrame.value = shot.data
            if (!frame.value && !serverThumb.value) {
              frame.value = shot.data
              cacheFrame(shot.data)
            }
          }
          progress.value = (step + 1) / total
        })
        step += 1
        if (step >= total) stopPreview()
      }, 900)
    }
  } catch {
    stopPreview()
  }
}

function onEnter(event: PointerEvent) {
  if (event.pointerType !== 'mouse' || disposed) return
  if (previewing.value) return
  if (hoverTimer !== null) window.clearTimeout(hoverTimer)
  hoverTimer = window.setTimeout(() => void startPreview(), 180)
}

onMounted(async () => {
  frame.value = (await getCachedThumb(props.item.key)) ?? ''
  // thumb_url 恒为接口地址：HEAD 探测实际是否真有缩略图。
  if (props.item.thumb_url) {
    try {
      const res = await fetch(withToken(props.item.thumb_url), { method: 'HEAD' })
      const type = res.headers.get('content-type') ?? ''
      if (!res.ok || (type && !type.startsWith('image/'))) thumbFailed.value = true
    } catch {
      thumbFailed.value = true
    }
  } else {
    thumbFailed.value = true
  }
  if (disposed || displaySrc.value || !rootEl.value) return
  observer = new IntersectionObserver(
    (entries) => {
      if (!entries.some((e) => e.isIntersecting)) return
      observer?.disconnect()
      observer = null
      queueFirstFrame()
    },
    { rootMargin: '200px' },
  )
  observer.observe(rootEl.value)
})

onBeforeUnmount(() => {
  disposed = true
  observer?.disconnect()
  observer = null
  stopPreview()
})
</script>

<template>
  <div ref="rootEl" class="absolute inset-0 overflow-hidden" @pointerenter="onEnter" @pointerleave="stopPreview">
    <img
      v-if="displaySrc"
      :src="displaySrc"
      class="h-full w-full object-cover transition duration-200"
      :class="item.playable ? '' : 'opacity-40 blur-[2px]'"
      loading="lazy"
      alt=""
      @error="
        previewFrame = '';
        thumbFailed = true;
        ($event.target as HTMLImageElement).style.display = 'none'
      "
    />
    <div
      v-else
      class="pointer-events-none absolute inset-0 flex items-center justify-center bg-gradient-to-br from-slate-800 to-slate-950"
    >
      <svg class="h-7 w-7 text-white/25" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <rect x="3" y="4.5" width="18" height="15" rx="2.5" />
        <path d="m10 9 5 3-5 3V9Z" />
      </svg>
    </div>
    <div
      v-if="previewing"
      class="pointer-events-none absolute inset-x-0 bottom-0 h-1 bg-white/20"
      title="正在静音预览并抽取封面"
    >
      <div class="h-full bg-reel-accent transition-all" :style="{ width: `${progress * 100}%` }"></div>
    </div>
  </div>
</template>
