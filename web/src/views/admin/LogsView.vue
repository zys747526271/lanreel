<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NButton,
  NCard,
  NDrawer,
  NDrawerContent,
  NInput,
  NSpin,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import { listLogs, purgeLogs } from '@/api/admin'
import { formatTime } from '@/lib/format'
import type { SystemLogEntry } from '@/api/types'

const message = useMessage()
const dialog = useDialog()

const categories = [
  'system', 'scan', 'auth', 'access', 'api', 'business', 'policy',
  'media', 'player', 'interaction', 'performance', 'error', 'frontend',
]

const logs = ref<SystemLogEntry[]>([])
const loading = ref(true)
const page = ref(1)
const pageSize = 50
const total = ref(0)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const filters = ref({ category: '', level: '', keyword: '', request_id: '', from: '', to: '' })

const detail = ref<SystemLogEntry | null>(null)
const drawerOpen = ref(false)

const levelType: Record<string, 'default' | 'info' | 'warning' | 'error'> = {
  debug: 'default',
  info: 'info',
  warn: 'warning',
  error: 'error',
}

async function load() {
  loading.value = true
  try {
    const res = await listLogs({
      page: page.value,
      page_size: pageSize,
      category: filters.value.category || undefined,
      level: filters.value.level || undefined,
      keyword: filters.value.keyword || undefined,
      request_id: filters.value.request_id || undefined,
      from: filters.value.from || undefined,
      to: filters.value.to || undefined,
    })
    logs.value = res.data
    total.value = res.meta.total
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  void load()
}

function reset() {
  filters.value = { category: '', level: '', keyword: '', request_id: '', from: '', to: '' }
  search()
}

function changePage(next: number) {
  if (next < 1 || next > totalPages.value) return
  page.value = next
  void load()
}

function openDetail(row: SystemLogEntry) {
  detail.value = row
  drawerOpen.value = true
}

function prettifyContext(raw: string) {
  if (!raw) return '—'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function confirmPurge() {
  dialog.warning({
    title: '清理运行日志',
    content: '将按当前保留策略（天数 / 最大条数）删除超限日志，审计日志不受影响。',
    positiveText: '执行清理',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const res = await purgeLogs()
        message.success(`已清理 ${res.data.removed} 条日志`)
        await load()
      } catch (err) {
        message.error(err instanceof Error ? err.message : '清理失败')
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <n-card size="small">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-6">
        <div>
          <p class="mb-1 text-xs text-slate-500">分类</p>
          <select v-model="filters.category" class="h-[34px] w-full rounded-md border border-slate-300 bg-white px-2 text-sm" @change="search">
            <option value="">全部分类</option>
            <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
          </select>
        </div>
        <div>
          <p class="mb-1 text-xs text-slate-500">级别</p>
          <select v-model="filters.level" class="h-[34px] w-full rounded-md border border-slate-300 bg-white px-2 text-sm" @change="search">
            <option value="">全部级别</option>
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
        </div>
        <div>
          <p class="mb-1 text-xs text-slate-500">关键字</p>
          <n-input v-model:value="filters.keyword" placeholder="消息 / 上下文 / 模块" clearable @keyup.enter="search" />
        </div>
        <div>
          <p class="mb-1 text-xs text-slate-500">请求 ID</p>
          <n-input v-model:value="filters.request_id" placeholder="按 request_id 串联" clearable @keyup.enter="search" />
        </div>
        <div>
          <p class="mb-1 text-xs text-slate-500">开始时间</p>
          <input v-model="filters.from" type="datetime-local" class="h-[34px] w-full rounded-md border border-slate-300 px-2 text-xs" />
        </div>
        <div>
          <p class="mb-1 text-xs text-slate-500">结束时间</p>
          <input v-model="filters.to" type="datetime-local" class="h-[34px] w-full rounded-md border border-slate-300 px-2 text-xs" />
        </div>
      </div>
      <div class="mt-3 flex flex-wrap items-center gap-2">
        <n-button type="primary" size="small" @click="search">查询</n-button>
        <n-button size="small" @click="reset">重置</n-button>
        <n-button size="small" @click="load">刷新</n-button>
        <n-button size="small" type="warning" ghost @click="confirmPurge">按保留策略清理</n-button>
        <span class="ml-auto text-xs text-slate-400">共 {{ total }} 条 · 索引：(ts, category, level)、(request_id)</span>
      </div>
    </n-card>

    <n-spin :show="loading">
      <div class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <table class="w-full min-w-[980px] text-sm">
          <thead class="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th class="px-3 py-2.5 text-left font-medium">时间</th>
              <th class="px-3 py-2.5 text-left font-medium">级别</th>
              <th class="px-3 py-2.5 text-left font-medium">分类</th>
              <th class="px-3 py-2.5 text-left font-medium">模块</th>
              <th class="px-3 py-2.5 text-left font-medium">消息</th>
              <th class="px-3 py-2.5 text-left font-medium">请求 ID</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="log in logs" :key="log.id" class="cursor-pointer hover:bg-slate-50/80" @click="openDetail(log)">
              <td class="whitespace-nowrap px-3 py-2 text-xs text-slate-500">{{ formatTime(log.ts) }}</td>
              <td class="px-3 py-2">
                <n-tag size="tiny" :type="levelType[log.level] ?? 'default'" :bordered="false">{{ log.level }}</n-tag>
              </td>
              <td class="px-3 py-2 font-mono text-xs text-slate-600">{{ log.category }}</td>
              <td class="px-3 py-2 font-mono text-xs text-slate-500">{{ log.module }}</td>
              <td class="max-w-[380px] px-3 py-2">
                <p class="truncate text-xs text-slate-800" :title="log.message">{{ log.message }}</p>
              </td>
              <td class="px-3 py-2 font-mono text-[10px] text-slate-400">{{ log.request_id || '—' }}</td>
            </tr>
            <tr v-if="!logs.length && !loading">
              <td colspan="6" class="px-4 py-10 text-center text-sm text-slate-400">没有匹配的日志</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="mt-3 flex items-center justify-between text-xs text-slate-500">
        <span>第 {{ page }} / {{ totalPages }} 页</span>
        <div class="flex gap-2">
          <n-button size="small" :disabled="page <= 1" @click="changePage(page - 1)">上一页</n-button>
          <n-button size="small" :disabled="page >= totalPages" @click="changePage(page + 1)">下一页</n-button>
        </div>
      </div>
    </n-spin>

    <n-drawer v-model:show="drawerOpen" :width="520" placement="right">
      <n-drawer-content title="日志详情" closable>
        <div v-if="detail" class="space-y-3 text-sm">
          <div class="grid grid-cols-2 gap-2 text-xs">
            <div><p class="text-slate-400">时间</p><p class="mt-0.5">{{ formatTime(detail.ts) }}</p></div>
            <div><p class="text-slate-400">级别</p><p class="mt-0.5">{{ detail.level }}</p></div>
            <div><p class="text-slate-400">分类</p><p class="mt-0.5 font-mono">{{ detail.category }}</p></div>
            <div><p class="text-slate-400">模块</p><p class="mt-0.5 font-mono">{{ detail.module }}</p></div>
            <div class="col-span-2"><p class="text-slate-400">请求 ID</p><p class="mt-0.5 font-mono break-all">{{ detail.request_id || '—' }}</p></div>
            <div class="col-span-2"><p class="text-slate-400">用户 ID</p><p class="mt-0.5">{{ detail.user_id ?? '—' }}</p></div>
          </div>
          <div>
            <p class="text-xs text-slate-400">消息</p>
            <p class="mt-1 rounded-lg bg-slate-50 px-3 py-2 text-sm">{{ detail.message }}</p>
          </div>
          <div>
            <p class="text-xs text-slate-400">上下文（已脱敏，白名单字段）</p>
            <pre class="mt-1 max-h-[400px] overflow-auto rounded-lg bg-slate-900 p-3 text-[11px] leading-relaxed text-slate-100">{{ prettifyContext(detail.context_json) }}</pre>
          </div>
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>
