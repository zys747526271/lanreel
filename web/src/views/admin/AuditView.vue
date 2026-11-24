<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NButton, NCard, NInput, NSpin, NTag, useMessage } from 'naive-ui'
import { listAuditLogs } from '@/api/admin'
import { formatTime } from '@/lib/format'
import type { AuditLog } from '@/api/types'

const message = useMessage()

const logs = ref<AuditLog[]>([])
const loading = ref(true)
const page = ref(1)
const pageSize = 20
const total = ref(0)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const filters = ref({ target_type: '', keyword: '', from: '', to: '' })

async function load() {
  loading.value = true
  try {
    const res = await listAuditLogs({
      page: page.value,
      page_size: pageSize,
      target_type: filters.value.target_type || undefined,
      keyword: filters.value.keyword || undefined,
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

function changePage(next: number) {
  if (next < 1 || next > totalPages.value) return
  page.value = next
  void load()
}

function preview(raw: string) {
  if (!raw || raw === 'null') return '—'
  try {
    const obj = JSON.parse(raw)
    const text = JSON.stringify(obj)
    return text.length > 120 ? text.slice(0, 120) + '…' : text
  } catch {
    return raw.slice(0, 120)
  }
}

const typeLabel: Record<string, string> = {
  user: '用户',
  folder: '文件夹',
  file: '文件',
  setting: '设置',
  log: '日志',
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <n-card size="small">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <div>
          <p class="mb-1 text-xs text-slate-500">对象类型</p>
          <select v-model="filters.target_type" class="h-[34px] w-full rounded-md border border-slate-300 bg-white px-2 text-sm" @change="search">
            <option value="">全部</option>
            <option value="user">用户</option>
            <option value="folder">文件夹</option>
            <option value="file">文件</option>
            <option value="setting">设置</option>
          </select>
        </div>
        <div>
          <p class="mb-1 text-xs text-slate-500">关键字</p>
          <n-input v-model:value="filters.keyword" placeholder="目标 / 动作 / 变更值" clearable @keyup.enter="search" />
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
      <div class="mt-3 flex items-center gap-2">
        <n-button type="primary" size="small" @click="search">查询</n-button>
        <n-button size="small" @click="load">刷新</n-button>
        <span class="ml-auto text-xs text-slate-400">审计日志独立保留，不随运行日志清理</span>
      </div>
    </n-card>

    <n-spin :show="loading">
      <div class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <table class="w-full min-w-[900px] text-sm">
          <thead class="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th class="px-3 py-2.5 text-left font-medium">时间</th>
              <th class="px-3 py-2.5 text-left font-medium">操作人</th>
              <th class="px-3 py-2.5 text-left font-medium">对象</th>
              <th class="px-3 py-2.5 text-left font-medium">动作</th>
              <th class="px-3 py-2.5 text-left font-medium">变更前</th>
              <th class="px-3 py-2.5 text-left font-medium">变更后</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="log in logs" :key="log.id" class="hover:bg-slate-50/70">
              <td class="whitespace-nowrap px-3 py-2 text-xs text-slate-500">{{ formatTime(log.created_at) }}</td>
              <td class="px-3 py-2 text-xs">#{{ log.admin_user_id }}</td>
              <td class="px-3 py-2 text-xs">
                <n-tag size="tiny" :bordered="false">{{ typeLabel[log.target_type] ?? log.target_type }}</n-tag>
                <span class="ml-1 break-all font-mono text-[10px] text-slate-400">{{ log.target_id || '—' }}</span>
              </td>
              <td class="px-3 py-2 text-xs font-medium text-slate-700">{{ log.action }}</td>
              <td class="max-w-[220px] px-3 py-2 font-mono text-[10px] text-slate-400" :title="log.before_value">
                {{ preview(log.before_value) }}
              </td>
              <td class="max-w-[220px] px-3 py-2 font-mono text-[10px] text-slate-500" :title="log.after_value">
                {{ preview(log.after_value) }}
              </td>
            </tr>
            <tr v-if="!logs.length && !loading">
              <td colspan="6" class="px-4 py-10 text-center text-sm text-slate-400">暂无审计记录</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="mt-3 flex items-center justify-between text-xs text-slate-500">
        <span>共 {{ total }} 条 · 第 {{ page }} / {{ totalPages }} 页</span>
        <div class="flex gap-2">
          <n-button size="small" :disabled="page <= 1" @click="changePage(page - 1)">上一页</n-button>
          <n-button size="small" :disabled="page >= totalPages" @click="changePage(page + 1)">下一页</n-button>
        </div>
      </div>
    </n-spin>
  </div>
</template>
