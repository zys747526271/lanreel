<script setup lang="ts">
/** 收藏 / 历史的缩略图网格；封面支持悬停快速跳播抽帧（MediaThumb）。 */
import MediaThumb from './MediaThumb.vue'
import { formatDuration, reasonText } from '@/lib/format'
import type { MediaItem } from '@/api/types'

defineProps<{
  items: MediaItem[]
  removable?: boolean
}>()

const emit = defineEmits<{
  (e: 'open', item: MediaItem): void
  (e: 'remove', item: MediaItem): void
}>()
</script>

<template>
  <div class="grid grid-cols-3 gap-1.5 p-1.5 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6">
    <div
      v-for="item in items"
      :key="item.key"
      class="group relative aspect-[9/14] cursor-pointer overflow-hidden rounded-lg bg-white/5 text-left"
      :title="item.rel_path"
      @click="emit('open', item)"
    >
      <MediaThumb :item="item" />

      <!-- 视频角标 -->
      <span
        v-if="item.media_type === 'video'"
        class="pointer-events-none absolute left-1 top-1 rounded bg-black/65 px-1 py-0.5 text-[10px] text-white/90 backdrop-blur"
      >
        <svg class="inline h-3 w-3 -translate-y-px" viewBox="0 0 24 24" fill="currentColor">
          <path d="M8 5.5v13l11-6.5z" />
        </svg>
        <span v-if="item.duration" class="ml-0.5">{{ formatDuration(item.duration) }}</span>
      </span>

      <!-- 不可播放 -->
      <span
        v-if="!item.playable"
        class="pointer-events-none absolute inset-x-1 bottom-1 rounded bg-black/65 px-1 py-0.5 text-center text-[10px] text-amber-200 backdrop-blur"
      >
        {{ reasonText(item.reason) }}
      </span>

      <span
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
