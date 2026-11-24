<script setup lang="ts">
/**
 * 收藏夹 / 观看历史的封面图。
 *
 * 服务端缩略图优先；没有服务端缩略图时，鼠标悬停会在约 10 秒内快速跳播采样
 * 5 个位置并抽帧（类似流媒体的 hover 预览），抽到的帧缓存到 IndexedDB，
 * 下次打开网格直接作为封面显示，无需再次下载视频。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { withToken } from '@/api/client'
import { getCachedThumb, saveCachedThumb } from '@/lib/thumbCache'
import type { MediaItem } from '@/api/types'

const props = defineProps<{ item: MediaItem }>()

const frame = ref('')
const previewing = ref(false)
const progress = ref(0)
/** 服务端 thumb_url 始终是接口地址，需按图片实际加载结果判断缩略图是否可用。 */
const thumbFailed = ref(false)
const serverThumb = computed(() =>
  props.item.thumb_url && !thumbFailed.value ? withToken(props.item.thumb_url) : '',
)
const displaySrc = computed(() => frame.value || serverThumb.value)

const SAMPLE_RATIOS = [0.08, 0.28, 0.5, 0.72, 0.92]
const STEP_MS = 2000 // 5 帧 × 2 秒 ≈ 10 秒快速跳播

let video: HTMLVideoElement | null = null
let hoverTimer: number | null = null
let stepTimer: number | null = null
let cancelled = false
let saved = false

function cleanupVideo() {
  if (!video) return
  const el = video
  video = null
  el.removeAttribute('src')
  el.load()
  el.remove()
}

function stopPreview() {
  cancelled = true
  if (hoverTimer !== null) window.clearTimeout(hoverTimer)
  if (stepTimer !== null) window.clearTimeout(stepTimer)
  hoverTimer = null
  stepTimer = null
  previewing.value = false
  progress.value = 0
  cleanupVideo()
}

function waitForMetadata(el: HTMLVideoElement): Promise<boolean> {
  return new Promise((resolve) => {
    let done = false
    const finish = (ok: boolean) => {
      if (done) return
      done = true
      resolve(ok)
    }
    el.addEventListener('loadedmetadata', () => finish(true), { once: true })
    el.addEventListener('error', () => finish(false), { once: true })
    window.setTimeout(() => finish(el.readyState >= 1), 6000)
  })
}

async function sampleAt(el: HTMLVideoElement, time: number): Promise<string | null> {
  if (cancelled) return null
  await new Promise<void>((resolve) => {
    let done = false
    const finish = () => {
      if (done) return
      done = true
      resolve()
    }
    el.addEventListener('seeked', finish, { once: true })
    el.addEventListener('error', finish, { once: true })
    try {
      el.currentTime = time
    } catch {
      finish()
    }
    window.setTimeout(finish, 3000)
  })
  if (cancelled || el.readyState < 2 || !el.videoWidth) return null
  try {
    const canvas = document.createElement('canvas')
    const scale = Math.min(1, 320 / el.videoWidth)
    canvas.width = Math.max(1, Math.round(el.videoWidth * scale))
    canvas.height = Math.max(1, Math.round(el.videoHeight * scale))
    canvas.getContext('2d')?.drawImage(el, 0, 0, canvas.width, canvas.height)
    return canvas.toDataURL('image/jpeg', 0.72)
  } catch {
    return null
  }
}

async function startPreview() {
  if (props.item.media_type !== 'video' || !props.item.playable) return
  if (displaySrc.value) return // 已有服务端缩略图或抽帧缓存，无需重复下载
  cancelled = false
  previewing.value = true
  const el = document.createElement('video')
  video = el
  //  detached <video> 在 Chromium 中不会加载元数据，需挂到 DOM（不可见）。
  el.style.cssText = 'position:fixed;left:-9999px;top:0;width:2px;height:2px;opacity:0;pointer-events:none;'
  document.body.appendChild(el)
  el.src = withToken(props.item.stream_url)
  el.muted = true
  el.playsInline = true
  el.preload = 'auto'
  if (!(await waitForMetadata(el)) || cancelled || !Number.isFinite(el.duration) || el.duration <= 0) {
    stopPreview()
    return
  }
  const samples = SAMPLE_RATIOS.map((r) => Math.min(el.duration - 0.05, el.duration * r)).filter((t) => t >= 0)
  let index = 0
  const run = async () => {
    if (cancelled || index >= samples.length) {
      stopPreview()
      return
    }
    progress.value = (index + 1) / samples.length
    const data = await sampleAt(el, samples[index])
    if (data && !frame.value) {
      frame.value = data
      if (!saved) {
        saved = true
        void saveCachedThumb(props.item.key, data)
      }
    }
    index += 1
    stepTimer = window.setTimeout(() => void run(), STEP_MS)
  }
  void run()
}

function onEnter(event: PointerEvent) {
  if (event.pointerType !== 'mouse' || displaySrc.value) return
  if (hoverTimer !== null) window.clearTimeout(hoverTimer)
  hoverTimer = window.setTimeout(() => void startPreview(), 180)
}

onMounted(async () => {
  frame.value = (await getCachedThumb(props.item.key)) ?? ''
  // thumb_url 恒为接口地址；先探测服务端是否真的有缩略图，缺失时启用悬浮抽帧。
  if (props.item.thumb_url) {
    try {
      const res = await fetch(withToken(props.item.thumb_url), { method: 'HEAD' })
      if (!res.ok) thumbFailed.value = true
    } catch {
      thumbFailed.value = true
    }
  } else {
    thumbFailed.value = true
  }
})

onBeforeUnmount(stopPreview)
</script>

<template>
  <div class="absolute inset-0 overflow-hidden" @pointerenter="onEnter" @pointerleave="stopPreview">
    <img
      v-if="displaySrc"
      :src="displaySrc"
      class="h-full w-full object-cover transition duration-200"
      :class="item.playable ? '' : 'opacity-40 blur-[2px]'"
      loading="lazy"
      alt=""
      @error="
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
      title="正在快速跳播并抽取封面"
    >
      <div class="h-full bg-reel-accent transition-all" :style="{ width: `${progress * 100}%` }"></div>
    </div>
  </div>
</template>
