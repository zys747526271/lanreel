<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NCard, NInput, NInputNumber, NModal, NSpin, NSwitch, useMessage } from 'naive-ui'
import { getLoggingSettings, resetToInitial, setLoggingSettings, setRegistration } from '@/api/admin'
import { fetchPublicConfig } from '@/api/client'
import type { LoggingSettings, PublicConfig } from '@/api/types'

const message = useMessage()

const loading = ref(true)
const savingLogging = ref(false)
const registrationOpen = ref(false)
const savingRegistration = ref(false)
const config = ref<PublicConfig | null>(null)
const resetModal = ref(false)
const resetConfirm = ref('')
const resetting = ref(false)

const logging = ref<LoggingSettings>({
  retention_days: 30,
  max_entries: 200000,
  level: 'info',
  frontend_upload_enabled: false,
})

async function load() {
  loading.value = true
  try {
    const [settings, cfg] = await Promise.all([getLoggingSettings(), fetchPublicConfig()])
    logging.value = settings.data
    registrationOpen.value = cfg.registration_open
    config.value = cfg
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function saveLogging() {
  savingLogging.value = true
  try {
    logging.value = (await setLoggingSettings(logging.value)).data
    message.success('日志配置已保存并即时生效')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    savingLogging.value = false
  }
}

async function toggleRegistration(value: boolean) {
  savingRegistration.value = true
  try {
    await setRegistration(value)
    registrationOpen.value = value
    message.success(value ? '已开放注册' : '已关闭注册')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '设置失败')
    await load()
  } finally {
    savingRegistration.value = false
  }
}

async function confirmReset() {
  if (resetConfirm.value.trim() !== '确认初始化') {
    message.error('请输入「确认初始化」')
    return
  }
  resetting.value = true
  try {
    await resetToInitial(resetConfirm.value.trim())
    resetModal.value = false
    resetConfirm.value = ''
    message.success('已恢复初始化，即将进入首次部署')
    window.setTimeout(() => window.location.assign('/setup'), 600)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '恢复初始化失败')
  } finally {
    resetting.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <n-spin :show="loading">
      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <n-card title="注册与访问" size="small">
        <div class="space-y-4 text-sm">
          <div class="flex items-center justify-between">
            <div>
              <p class="font-medium">允许普通用户注册</p>
              <p class="text-xs text-slate-500">关闭后注册接口返回 403，已登录用户不受影响</p>
            </div>
            <n-switch :value="registrationOpen" :loading="savingRegistration" @update:value="toggleRegistration" />
          </div>
          <div class="rounded-lg bg-slate-50 p-3 text-xs text-slate-600">
            <p>注册开关同时写入审计日志，便于追溯谁在何时封闭了注册入口。</p>
          </div>
          <div v-if="config" class="grid grid-cols-2 gap-2 text-xs text-slate-600">
            <p>服务端版本：{{ config.version }}</p>
            <p>时区：{{ config.timezone }}</p>
            <p>图片停留：{{ config.image_display_seconds }} 秒</p>
            <p>扫描间隔：{{ config.scan_interval_seconds }} 秒</p>
          </div>
        </div>
      </n-card>

      <n-card title="日志策略" size="small">
        <div class="space-y-4 text-sm">
          <div>
            <p class="mb-1 font-medium">保留天数</p>
            <p class="mb-2 text-xs text-slate-500">超过天数的运行日志会在启动时与每日 03:30 清理</p>
            <n-input-number v-model:value="logging.retention_days" :min="1" :max="3650" class="w-full" />
          </div>
          <div>
            <p class="mb-1 font-medium">最大条数</p>
            <p class="mb-2 text-xs text-slate-500">超出后按时间淘汰最旧记录，避免日志库无限膨胀</p>
            <n-input-number v-model:value="logging.max_entries" :min="1000" :max="5000000" :step="1000" class="w-full" />
          </div>
          <div>
            <p class="mb-1 font-medium">日志级别</p>
            <select v-model="logging.level" class="h-[34px] w-full rounded-md border border-slate-300 bg-white px-2 text-sm">
              <option value="debug">debug（最详细，排查用）</option>
              <option value="info">info（推荐）</option>
              <option value="warn">warn</option>
              <option value="error">error</option>
            </select>
          </div>
          <div class="flex items-center justify-between">
            <div>
              <p class="font-medium">允许前端日志上报</p>
              <p class="text-xs text-slate-500">开启后前端可将本地日志上报到后端 frontend 分类</p>
            </div>
            <n-switch v-model:value="logging.frontend_upload_enabled" />
          </div>
          <div class="flex justify-end">
            <n-button type="primary" :loading="savingLogging" @click="saveLogging">保存日志配置</n-button>
          </div>
        </div>
        </n-card>
      </div>
    </n-spin>

    <n-card title="危险操作" size="small" class="border-red-200">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div class="text-xs text-slate-500">
          <p class="font-medium text-red-600">恢复初始化</p>
          <p class="mt-1">清空所有账号、收藏、历史、策略、统计与日志，回到首次部署状态。不会删除媒体文件。</p>
        </div>
        <n-button type="error" strong @click="resetModal = true">恢复初始化</n-button>
      </div>
    </n-card>

    <n-modal v-model:show="resetModal" preset="card" title="恢复初始化" class="max-w-md" :bordered="false">
      <div class="space-y-3 text-sm">
        <p class="text-red-600">此操作不可撤销！所有用户与业务数据都会被清空。</p>
        <p class="text-xs text-slate-500">请输入「确认初始化」以继续：</p>
        <n-input v-model:value="resetConfirm" placeholder="确认初始化" />
        <div class="flex justify-end gap-2 pt-1">
          <n-button @click="resetModal = false">取消</n-button>
          <n-button type="error" strong :loading="resetting" @click="confirmReset">确认恢复</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>
