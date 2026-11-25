<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NCard, NSpin, NTag, useMessage } from 'naive-ui'
import { fetchDiagnostics } from '@/api/admin'
import { formatTime } from '@/lib/format'
import type { DiagnosticCheck, DiagnosticReport } from '@/api/types'

const message = useMessage()
const report = ref<DiagnosticReport | null>(null)
const loading = ref(false)

const tone: Record<DiagnosticCheck['status'], { label: string; type: 'success' | 'warning' | 'error' }> = {
  ok: { label: '正常', type: 'success' },
  warn: { label: '注意', type: 'warning' },
  error: { label: '异常', type: 'error' },
}

async function run() {
  loading.value = true
  try {
    report.value = (await fetchDiagnostics()).data
  } catch (err) {
    message.error(err instanceof Error ? err.message : '诊断失败')
  } finally {
    loading.value = false
  }
}

onMounted(run)
</script>

<template>
  <div class="space-y-4">
    <n-card size="small">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="text-sm font-medium">系统诊断</p>
          <p class="mt-1 text-xs text-slate-500">
            真实执行检测：数据库完整性、媒体目录可达性、文件丢失、扫描任务、策略状态、日志异常、前端资源与数据目录权限。数据目录会创建一个临时文件并立即删除，用于验证可写。
          </p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="report" class="text-xs text-slate-400">检测于 {{ formatTime(report.checked_at) }}</span>
          <n-button size="small" type="primary" :loading="loading" @click="run">重新检测</n-button>
        </div>
      </div>
      <div v-if="report" class="mt-3 flex flex-wrap gap-2 text-xs">
        <n-tag size="small" type="success" :bordered="false">正常 {{ report.counts.ok }}</n-tag>
        <n-tag size="small" type="warning" :bordered="false">注意 {{ report.counts.warn }}</n-tag>
        <n-tag size="small" :type="report.counts.error ? 'error' : 'default'" :bordered="false">
          异常 {{ report.counts.error }}
        </n-tag>
        <n-tag size="small" :type="report.healthy ? 'success' : 'error'" :bordered="false">
          {{ report.healthy ? '总体正常' : '存在问题' }}
        </n-tag>
      </div>
    </n-card>

    <n-spin :show="loading">
      <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <n-card
          v-for="check in report?.checks ?? []"
          :key="check.id"
          size="small"
          :class="check.status === 'error' ? 'border-red-200' : check.status === 'warn' ? 'border-amber-200' : ''"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="text-sm font-medium">{{ check.name }}</p>
              <p class="mt-1 text-xs text-slate-600">{{ check.summary }}</p>
            </div>
            <n-tag size="small" :type="tone[check.status].type" :bordered="false">
              {{ tone[check.status].label }}
            </n-tag>
          </div>
          <p v-if="check.detail" class="mt-2 break-all rounded-lg bg-slate-50 px-3 py-2 font-mono text-[11px] leading-relaxed text-slate-500">
            {{ check.detail }}
          </p>
          <p v-if="check.suggestion" class="mt-2 text-xs text-slate-500">
            <span class="font-medium text-slate-700">建议：</span>{{ check.suggestion }}
          </p>
        </n-card>
      </div>
      <p v-if="!report && !loading" class="py-10 text-center text-sm text-slate-400">暂无诊断结果</p>
    </n-spin>
  </div>
</template>
