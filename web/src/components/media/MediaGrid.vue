<script setup lang="ts">
/** 收藏 / 历史的缩略图网格；封面支持悬停快速跳播抽帧（MediaThumb）。 */
import { ref } from 'vue'
import MediaThumb from './MediaThumb.vue'
import { formatDuration, reasonText } from '@/lib/format'
import type { MediaItem } from '@/api/types'

const props = defineProps<{
  items: MediaItem[]
  removable?: boolean
  /** 批量勾选模式（收藏夹） */
  selectable?: boolean
  selectedKeys?: string[]
}>()

const emit = defineEmits<{
  (e: 'open', item: MediaItem): void
  (e: 'remove', item: MediaItem): void
  (e: 'longpress', item: MediaItem): void
  (e: 'toggle-select', item: MediaItem): void
}>()

/** 长按 550ms 弹出操作（管理员屏蔽的条目也能取消收藏）。 */
let pressTimer: number | null = null
const longPressFired = ref(false)

function startPress(item: MediaItem) {
  longPressFired.value = false
  if (pressTimer !== null) window.clearTimeout(pressTimer)
  pressTimer = window.setTimeout(() => {
    longPressFired.value = true
    pressTimer = null
    emit('longpress', item)
  }, 550)
}

function cancelPress() {
  if (pressTimer !== null) window.clearTimeout(pressTimer)
  pressTimer = null
}

function onClick(item: MediaItem) {
  if (longPressFired.value) {
    longPressFired.value = false
    return
  }
  if (props.selectable) {
    emit('toggle-select', item)
    return
  }
  emit('open', item)
}
</script>

<template>
  <div class="grid grid-cols-3 gap-1.5 p-1.5 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6">
    <div
      v-for="item in items"
      :key="item.key"
      class="group relative isolate aspect-[9/14] cursor-pointer overflow-hidden rounded-lg bg-white/5 text-left"
      :title="item.rel_path"
      @pointerdown.passive="startPress(item)"
      @pointerup.passive="cancelPress"
      @pointercancel.passive="cancelPress"
      @pointerleave.passive="cancelPress"
      @contextmenu.prevent="emit('longpress', item)"
      @click="onClick(item)"
    >
      <MediaThumb :item="item" />

      <!-- 批量勾选 -->
      <span
        v-if="selectable"
        class="absolute left-1 top-1 z-10 flex h-5 w-5 items-center justify-center rounded-md border-2 backdrop-blur"
        :class="selectedKeys?.includes(item.key)
          ? 'border-reel-accent bg-reel-accent text-black'
          : 'border-white/70 bg-black/45 text-transparent'"
      >
        <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <path d="m5 12 4.5 4.5L19 7" />
        </svg>
      </span>

      <!-- 视频角标 -->
      <span
        v-if="item.media_type === 'video' && item.playable && !selectable"
        class="pointer-events-none absolute left-1 top-1 rounded bg-black/65 px-1 py-0.5 text-[10px] text-white/90 backdrop-blur"
      >
        <svg class="inline h-3 w-3 -translate-y-px" viewBox="0 0 24 24" fill="currentColor">
          <path d="M8 5.5v13l11-6.5z" />
        </svg>
        <span v-if="item.duration" class="ml-0.5">{{ formatDuration(item.duration) }}</span>
      </span>

      <!-- 观看进度条（v0.3.0 需求 2）：显示已看比例，看完则不显示 -->
      <div
        v-if="item.playable && item.watchProgress && item.watchProgress > 0.005 && item.watchProgress < 0.995"
        class="pointer-events-none absolute inset-x-0 bottom-0 h-[3px] bg-black/55"
      >
        <div
          class="h-full bg-reel-accent"
          :style="{ width: `${Math.min(100, item.watchProgress * 100)}%` }"
        ></div>
      </div>
      <span
        v-if="item.playable && item.watchProgress && item.watchProgress >= 0.995"
        class="pointer-events-none absolute bottom-1 left-1 rounded bg-black/70 px-1 py-0.5 text-[9px] text-emerald-300"
      >已看完</span>

      <!-- 不可播放：高斯模糊封面 + 暗色遮罩 + 居中提示（隐私遮挡） -->
      <template v-if="!item.playable">
        <div class="pointer-events-none absolute inset-0 bg-black/45"></div>
        <div
          class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-1.5 px-2 text-center"
        >
          <svg
            class="h-6 w-6 shrink-0 text-white/75"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
          >
            <rect x="4" y="10.5" width="16" height="10" rx="2.5" />
            <path d="M8 10.5V8a4 4 0 0 1 8 0v2.5" />
          </svg>
          <span class="text-[11px] font-medium leading-tight text-white/95">
            {{ reasonText(item.reason) }}
          </span>
        </div>
      </template>

      <span
        v-if="item.playable"
        class="pointer-events-none absolute inset-0 flex items-center justify-center opacity-0 transition group-hover:opacity-100"
      >
        <span class="rounded-full bg-black/50 p-2.5 backdrop-blur">
          <svg class="h-5 w-5 text-white" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5.5v13l11-6.5z" />
          </svg>
        </span>
      </span>

      <button
        v-if="removable"
        class="absolute right-1 top-1 hidden rounded-full bg-black/75 p-1.5 text-white/85 backdrop-blur group-hover:block"
        title="从历史中移除"
        @click.stop="emit('remove', item)"
      >
        <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 6l12 12M18 6 6 18" />
        </svg>
      </button>
    </div>
  </div>
</template>
