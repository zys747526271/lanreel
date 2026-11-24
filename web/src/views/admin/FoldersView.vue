<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NButton,
  NCard,
  NInput,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSpin,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import { deleteFolderPolicy, listFolders, setFolderPolicy } from '@/api/admin'
import type { FolderPolicyView, ScheduleType } from '@/api/types'

const message = useMessage()
const dialog = useDialog()

const folders = ref<FolderPolicyView[]>([])
const loading = ref(true)
const saving = ref(false)
const showModal = ref(false)
const editing = ref<{
  folder_path: string
  enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
}>({ folder_path: '', enabled: true, schedule_type: 'always', time_start: '18:00', time_end: '23:00' })

const totalMedia = computed(() => folders.value.reduce((sum, f) => sum + f.total, 0))
const configuredCount = computed(() => folders.value.filter((f) => f.configured).length)

async function load() {
  loading.value = true
  try {
    folders.value = (await listFolders()).data
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function openEdit(folder: FolderPolicyView) {
  editing.value = {
    folder_path: folder.folder_path,
    enabled: folder.enabled,
    schedule_type: folder.configured ? folder.schedule_type : 'always',
    time_start: folder.time_start || '18:00',
    time_end: folder.time_end || '23:00',
  }
  showModal.value = true
}

async function save() {
  const e = editing.value
  if (e.schedule_type === 'time_window' && (!e.time_start || !e.time_end)) {
    message.warning('请填写时间窗口的起止时间')
    return
  }
  saving.value = true
  try {
    await setFolderPolicy({
      folder_path: e.folder_path,
      enabled: e.enabled,
      schedule_type: e.enabled ? e.schedule_type : 'disabled',
      time_start: e.time_start,
      time_end: e.time_end,
    })
    message.success('策略已保存，客户端缓存将在数秒内失效')
    showModal.value = false
    await load()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function resetPolicy(folder: FolderPolicyView) {
  dialog.warning({
    title: '恢复默认策略',
    content: `确认删除「${folder.folder_path || '根目录'}」的策略？该目录将回落到上级或默认允许。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteFolderPolicy(folder.folder_path)
        message.success('已恢复默认策略')
        await load()
      } catch (err) {
        message.error(err instanceof Error ? err.message : '删除失败')
      }
    },
  })
}

function scheduleLabel(f: FolderPolicyView) {
  if (!f.enabled || f.schedule_type === 'disabled') return '已禁用'
  if (f.schedule_type === 'time_window') return `${f.time_start}–${f.time_end} 可播`
  return '始终可播'
}

function sourceLabel(f: FolderPolicyView) {
  if (f.configured) return { text: '单独配置', type: 'default' as const }
  if (f.inherited) return { text: `继承「${f.inherited_from || '根目录'}」`, type: 'info' as const }
  return { text: '默认允许', type: 'success' as const }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <n-card size="small">
      <div class="flex flex-wrap items-center justify-between gap-3 text-sm">
        <div class="flex flex-wrap items-center gap-4 text-slate-600">
          <span>目录 {{ folders.length }} 个 · 媒体 {{ totalMedia }} 项</span>
          <span class="text-slate-400">已单独配置 {{ configuredCount }} 个</span>
        </div>
        <n-button size="small" @click="load">刷新</n-button>
      </div>
      <p class="mt-2 rounded-lg bg-slate-50 px-3 py-2 text-xs leading-relaxed text-slate-500">
        策略优先级：文件级覆盖 &gt; 文件夹策略（最长前缀匹配）&gt; 默认允许。根目录策略作为全局默认，会对未单独配置的子目录生效；
        时间窗口按服务端配置时区判断，支持跨零点（如 22:00–02:00）。
      </p>
    </n-card>

    <n-spin :show="loading">
      <div class="overflow-hidden rounded-xl border border-slate-200 bg-white">
        <table class="w-full min-w-[720px] text-sm">
          <thead class="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th class="px-4 py-2.5 text-left font-medium">文件夹</th>
              <th class="px-4 py-2.5 text-left font-medium">媒体</th>
              <th class="px-4 py-2.5 text-left font-medium">生效状态</th>
              <th class="px-4 py-2.5 text-left font-medium">来源</th>
              <th class="px-4 py-2.5 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="f in folders" :key="f.folder_path" class="hover:bg-slate-50/70">
              <td class="px-4 py-2.5">
                <span class="font-mono text-xs text-slate-800">{{ f.folder_path || '/（根目录）' }}</span>
              </td>
              <td class="px-4 py-2.5 text-xs text-slate-500">
                {{ f.total }} 项<span class="text-slate-300"> · </span>视频 {{ f.videos }} · 图片 {{ f.images }}
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="!f.enabled || f.schedule_type === 'disabled' ? 'error' : f.schedule_type === 'time_window' ? 'warning' : 'success'" :bordered="false">
                  {{ scheduleLabel(f) }}
                </n-tag>
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="sourceLabel(f).type" :bordered="false">{{ sourceLabel(f).text }}</n-tag>
              </td>
              <td class="px-4 py-2.5 text-right">
                <n-button size="tiny" quaternary type="primary" @click="openEdit(f)">编辑</n-button>
                <n-button v-if="f.configured" size="tiny" quaternary @click="resetPolicy(f)">恢复默认</n-button>
              </td>
            </tr>
            <tr v-if="!folders.length && !loading">
              <td colspan="5" class="px-4 py-10 text-center text-sm text-slate-400">
                尚未扫描到任何媒体目录，请确认服务端 media.roots 配置
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </n-spin>

    <n-modal v-model:show="showModal" preset="card" style="width: 460px" title="设置文件夹策略">
      <div class="space-y-4 text-sm">
        <div>
          <p class="text-xs text-slate-500">文件夹</p>
          <p class="mt-1 rounded-lg bg-slate-50 px-3 py-2 font-mono text-xs">
            {{ editing.folder_path || '/（根目录，作为全局默认）' }}
          </p>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <p class="font-medium">允许普通用户可见</p>
            <p class="text-xs text-slate-500">关闭后该目录内容进入禁用状态</p>
          </div>
          <n-switch v-model:value="editing.enabled" />
        </div>

        <div v-if="editing.enabled">
          <p class="mb-2 text-xs text-slate-500">播放时段</p>
          <n-radio-group v-model:value="editing.schedule_type">
            <n-radio-button value="always">始终可播</n-radio-button>
            <n-radio-button value="time_window">时间窗口</n-radio-button>
          </n-radio-group>
        </div>

        <div v-if="editing.enabled && editing.schedule_type === 'time_window'" class="grid grid-cols-2 gap-3">
          <div>
            <p class="mb-1 text-xs text-slate-500">开始时间</p>
            <n-input v-model:value="editing.time_start" placeholder="18:00" />
          </div>
          <div>
            <p class="mb-1 text-xs text-slate-500">结束时间</p>
            <n-input v-model:value="editing.time_end" placeholder="23:00" />
          </div>
          <p class="col-span-2 text-xs text-slate-400">格式 HH:MM；结束时间早于开始时间表示跨零点。</p>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存策略</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
