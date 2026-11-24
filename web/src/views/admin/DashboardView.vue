<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NButton, NCard, NSpin, NSwitch, NTag, useMessage } from 'naive-ui'
import { fetchStats, setRegistration, triggerScan } from '@/api/admin'
import { formatTime } from '@/lib/format'
import type { AdminStats } from '@/api/types'

const message = useMessage()
const stats = ref<AdminStats | null>(null)
const loading = ref(true)
const scanning = ref(false)
const savingRegistration = ref(false)

const cards = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { label: '媒体总数', value: s.media_total, hint: `视频 ${s.videos} · 图片 ${s.images}` },
    { label: '用户', value: s.users, hint: `管理员 ${s.admins}` },
    { label: '收藏记录', value: s.favorites, hint: '全站累计' },
    { label: '策略', value: s.folder_policies + s.file_overrides, hint: `文件夹 ${s.folder_policies} · 文件 ${s.file_overrides}` },
  ]
})

const logCategories = computed(() => Object.entries(stats.value?.log_counts ?? {}).sort((a, b) => b[1] - a[1]))

async function load() {
  loading.value = true
  try {
    stats.value = (await fetchStats()).data
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function runScan() {
  scanning.value = true
  try {
    const res = await triggerScan()
    message.success(`扫描完成，当前共 ${res.data.media_total} 个媒体文件`)
    await load()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '扫描失败')
  } finally {
    scanning.value = false
  }
}

async function toggleRegistration(value: boolean) {
  savingRegistration.value = true
  try {
    await setRegistration(value)
    if (stats.value) stats.value.registration_open = value
    message.success(value ? '已开放注册' : '已关闭注册')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '设置失败')
    await load()
  } finally {
    savingRegistration.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <n-spin :show="loading">
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <n-card v-for="c in cards" :key="c.label" size="small" :bordered="true">
          <p class="text-xs text-slate-500">{{ c.label }}</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums">{{ c.value }}</p>
          <p class="mt-1 text-[11px] text-slate-400">{{ c.hint }}</p>
        </n-card>
      </div>
    </n-spin>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <n-card title="媒体扫描" size="small" class="lg:col-span-2">
        <div v-if="stats" class="space-y-3 text-sm">
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div class="rounded-lg bg-slate-50 p-3">
              <p class="text-xs text-slate-500">最近扫描</p>
              <p class="mt-1 text-sm font-medium">{{ formatTime(stats.scan.finished_at) }}</p>
            </div>
            <div class="rounded-lg bg-slate-50 p-3">
              <p class="text-xs text-slate-500">耗时</p>
              <p class="mt-1 text-sm font-medium tabular-nums">{{ stats.scan.duration_ms }} ms</p>
            </div>
            <div class="rounded-lg bg-slate-50 p-3">
              <p class="text-xs text-slate-500">跳过（非媒体）</p>
              <p class="mt-1 text-sm font-medium tabular-nums">{{ stats.scan.skipped }}</p>
            </div>
            <div class="rounded-lg bg-slate-50 p-3">
              <p class="text-xs text-slate-500">本次新增 / 移除</p>
              <p class="mt-1 text-sm font-medium tabular-nums">{{ stats.scan.added }} / {{ stats.scan.removed }}</p>
            </div>
          </div>
          <div v-if="stats.scan.errors?.length" class="rounded-lg bg-amber-50 p-3 text-xs text-amber-700">
            <p class="font-medium">扫描告警（{{ stats.scan.errors.length }}）</p>
            <ul class="mt-1 list-inside list-disc space-y-0.5">
              <li v-for="(e, i) in stats.scan.errors.slice(0, 5)" :key="i">{{ e }}</li>
            </ul>
          </div>
          <div class="flex gap-2">
            <n-button type="primary" :loading="scanning" @click="runScan">立即重新扫描</n-button>
            <n-button quaternary @click="load">刷新数据</n-button>
          </div>
        </div>
      </n-card>

      <n-card title="运行配置" size="small">
        <div class="space-y-4 text-sm">
          <div class="flex items-center justify-between">
            <div>
              <p class="font-medium">普通用户注册</p>
              <p class="text-xs text-slate-500">关闭后注册接口直接拒绝</p>
            </div>
            <n-switch
              :value="stats?.registration_open ?? false"
              :loading="savingRegistration"
              @update:value="toggleRegistration"
            />
          </div>
          <div v-if="stats" class="rounded-lg bg-slate-50 p-3 text-xs text-slate-600">
            <p>日志保留：{{ stats.logging.retention_days }} 天</p>
            <p class="mt-1">日志级别：{{ stats.logging.level }}</p>
            <p class="mt-1">前端上报：{{ stats.logging.frontend_upload_enabled ? '已开启' : '已关闭' }}</p>
          </div>
          <router-link to="/admin/settings" class="text-xs text-slate-700 underline-offset-2 hover:underline">
            前往系统设置 →
          </router-link>
        </div>
      </n-card>
    </div>

    <n-card title="日志分类分布" size="small">
      <n-spin :show="loading">
        <div v-if="logCategories.length" class="flex flex-wrap gap-2">
          <n-tag v-for="[cat, count] in logCategories" :key="cat" size="small" :bordered="false">
            {{ cat }} · {{ count }}
          </n-tag>
        </div>
        <p v-else class="text-sm text-slate-400">暂无日志</p>
      </n-spin>
    </n-card>
  </div>
</template>
