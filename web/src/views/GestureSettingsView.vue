<script setup lang="ts">
/** v0.3.1：手势操作二级设置页，按普通 / 沉浸 / 全屏三类场景分别配置。 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  END_ACTION_OPTIONS,
  GESTURE_CONTEXTS,
  SINGLE_TAP_OPTIONS,
  DOUBLE_TAP_OPTIONS,
  VERTICAL_OPTIONS,
  HORIZONTAL_OPTIONS,
  prefs,
  resetGestureContext,
  type EndAction,
  type GestureContext,
  type GestureConfig,
} from '@/lib/prefs'

const router = useRouter()
const context = ref<GestureContext>('normal')
const cfg = computed<GestureConfig>(() => prefs.gestures[context.value])
const meta = computed(() => GESTURE_CONTEXTS.find((c) => c.value === context.value))

function set<K extends keyof GestureConfig>(key: K, value: GestureConfig[K]) {
  ;(cfg.value as unknown as Record<string, unknown>)[key] = value
}

function setEndAction(value: EndAction) {
  prefs.endAction = value
}
</script>

<template>
  <div class="min-h-[100dvh] bg-[#08080a] px-4 py-5 text-white">
    <div class="mx-auto w-full max-w-2xl space-y-5">
      <header class="flex items-center gap-2">
        <button
          class="rounded-full border border-white/12 p-2 transition hover:bg-white/10"
          title="返回设置"
          aria-label="返回设置"
          @click="router.push('/settings')"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
        <div>
          <h1 class="text-base font-semibold">手势与播放操作</h1>
          <p class="mt-0.5 text-[11px] text-white/45">按场景分别定义短视频操作逻辑</p>
        </div>
      </header>

      <!-- 通用播放行为 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <h2 class="mb-3 text-sm font-medium text-white/80">通用播放行为</h2>
        <p class="mb-2 text-xs text-white/45">播放结束后</p>
        <div class="grid grid-cols-3 gap-1.5">
          <button
            v-for="opt in END_ACTION_OPTIONS"
            :key="opt.value"
            class="rounded-lg border px-2 py-2 text-center text-xs transition"
            :class="prefs.endAction === opt.value
              ? 'border-white bg-white text-black'
              : 'border-white/15 text-white/70 hover:bg-white/10'"
            :title="opt.desc"
            @click="setEndAction(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>
        <p class="mt-1.5 text-[11px] text-white/35">
          {{ END_ACTION_OPTIONS.find((o) => o.value === prefs.endAction)?.desc }}
        </p>
      </section>

      <!-- 场景切换 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-medium text-white/80">按场景配置</h2>
          <button class="text-[11px] text-white/40 hover:text-white/70" @click="resetGestureContext(context)">
            恢复该场景默认
          </button>
        </div>
        <div class="grid grid-cols-3 gap-1.5">
          <button
            v-for="c in GESTURE_CONTEXTS"
            :key="c.value"
            class="rounded-lg border px-2 py-2 text-xs transition"
            :class="context === c.value
              ? 'border-white bg-white text-black'
              : 'border-white/15 text-white/70 hover:bg-white/10'"
            @click="context = c.value"
          >
            {{ c.label }}
          </button>
        </div>
        <p class="mt-2 text-[11px] leading-relaxed text-white/40">{{ meta?.desc }}</p>

        <div class="mt-4 space-y-4">
          <div>
            <p class="mb-2 text-xs text-white/75">单击画面</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="o in SINGLE_TAP_OPTIONS"
                :key="o.value"
                class="rounded-lg border px-2.5 py-1.5 text-[11px] transition"
                :class="cfg.singleTap === o.value
                  ? 'border-white bg-white text-black'
                  : 'border-white/15 text-white/70 hover:bg-white/10'"
                @click="set('singleTap', o.value)"
              >
                {{ o.label }}
              </button>
            </div>
          </div>

          <div>
            <p class="mb-2 text-xs text-white/75">双击画面</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="o in DOUBLE_TAP_OPTIONS"
                :key="o.value"
                class="rounded-lg border px-2.5 py-1.5 text-[11px] transition"
                :class="cfg.doubleTap === o.value
                  ? 'border-white bg-white text-black'
                  : 'border-white/15 text-white/70 hover:bg-white/10'"
                @click="set('doubleTap', o.value)"
              >
                {{ o.label }}
              </button>
            </div>
          </div>

          <div>
            <p class="mb-2 text-xs text-white/75">上下滑动</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="o in VERTICAL_OPTIONS"
                :key="o.value"
                class="rounded-lg border px-2.5 py-1.5 text-[11px] transition"
                :class="cfg.vertical === o.value
                  ? 'border-white bg-white text-black'
                  : 'border-white/15 text-white/70 hover:bg-white/10'"
                @click="set('vertical', o.value)"
              >
                {{ o.label }}
              </button>
            </div>
          </div>

          <div>
            <p class="mb-2 text-xs text-white/75">左右滑动</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="o in HORIZONTAL_OPTIONS"
                :key="o.value"
                class="rounded-lg border px-2.5 py-1.5 text-[11px] transition"
                :class="cfg.horizontal === o.value
                  ? 'border-white bg-white text-black'
                  : 'border-white/15 text-white/70 hover:bg-white/10'"
                @click="set('horizontal', o.value)"
              >
                {{ o.label }}
              </button>
            </div>
          </div>

          <div class="flex items-center justify-between gap-4 border-t border-white/8 pt-3">
            <div>
              <p class="text-xs text-white/75">长按进入倍速</p>
              <p class="mt-0.5 text-[11px] text-white/40">长按 500ms 后上下滑动调速，松手保持</p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition"
              :class="cfg.longPressRate ? 'bg-white' : 'bg-white/15'"
              @click="cfg.longPressRate = !cfg.longPressRate"
            >
              <span
                class="absolute top-0.5 h-5 w-5 rounded-full bg-black transition"
                :class="cfg.longPressRate ? 'left-[1.375rem]' : 'left-0.5 bg-white/70'"
              ></span>
            </button>
          </div>
        </div>
      </section>

      <p class="pb-2 text-center text-[11px] text-white/25">
        普通：上下切视频 / 单击暂停 / 双击点赞 / 长按倍速<br />
        沉浸与全屏默认：上下音量 / 左右快进 / 单击暂停 / 双击点赞 / 长按倍速
      </p>
    </div>
  </div>
</template>
