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
import { deleteFileOverride, listFiles, listFolders, setFileOverride } from '@/api/admin'
import { formatBytes, formatDuration } from '@/lib/format'
import { reasonText } from '@/lib/format'
import type { FileView, FolderPolicyView, ScheduleType } from '@/api/types'

const message = useMessage()
const dialog = useDialog()

const files = ref<FileView[]>([])
const folders = ref<FolderPolicyView[]>([])
const loading = ref(true)
const saving = ref(false)
const keyword = ref('')
const folderFilter = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const showModal = ref(false)
const current = ref<FileView | null>(null)
const form = ref<{
  enabled: boolean
  guest_enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
  note: string
}>({ enabled: true, guest_enabled: true, schedule_type: 'always', time_start: '18:00', time_end: '23:00', note: '' })

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

async function load() {
  loading.value = true
  try {
    const res = await listFiles({
      page: page.value,
      page_size: pageSize,
      keyword: keyword.value || undefined,
      folder: folderFilter.value || undefined,
    })
    files.value = res.data
    total.value = res.meta.total
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadFolders() {
  try {
    folders.value = (await listFolders()).data
  } catch {
    /* 忽略：目录筛选不可用不影响主功能 */
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

function openEdit(file: FileView) {
  current.value = file
  const o = file.override
  form.value = {
    enabled: o?.enabled ?? true,
    guest_enabled: o?.guest_enabled ?? true,
    schedule_type: o?.schedule_type ?? 'always',
    time_start: o?.time_start || '18:00',
    time_end: o?.time_end || '23:00',
    note: o?.note ?? '',
  }
  showModal.value = true
}

async function save() {
  if (!current.value) return
  if (form.value.schedule_type === 'time_window' && (!form.value.time_start || !form.value.time_end)) {
    message.warning('请填写时间窗口的起止时间')
    return
  }
  saving.value = true
  try {
    await setFileOverride(current.value.key, {
      enabled: form.value.enabled,
      guest_enabled: form.value.guest_enabled,
      schedule_type: form.value.enabled || form.value.guest_enabled ? form.value.schedule_type : 'disabled',
      time_start: form.value.time_start,
      time_end: form.value.time_end,
      note: form.value.note,
    })
    message.success('文件策略已保存')
    showModal.value = false
    await load()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function clearOverride(file: FileView) {
  dialog.warning({
    title: '清除文件级策略',
    content: `确认清除「${file.rel_path}」的文件级覆盖？将回落到文件夹策略。`,
    positiveText: '确认清除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteFileOverride(file.key)
        message.success('已清除文件级策略')
        await load()
      } catch (err) {
        message.error(err instanceof Error ? err.message : '操作失败')
      }
    },
  })
}

function userStatus(f: FileView) {
  if (f.playable) return { text: '可播', type: 'success' as const }
  return { text: reasonText(f.reason), type: 'error' as const }
}

function guestStatus(f: FileView) {
  if (f.guest_playable) return { text: '可播', type: 'success' as const }
  return { text: reasonText(f.guest_reason), type: 'error' as const }
}

onMounted(async () => {
  await Promise.all([load(), loadFolders()])
})
</script>

<template>
  <div class="space-y-4">
    <n-card size="small">
      <div class="flex flex-wrap items-end gap-2">
        <div class="min-w-[200px] flex-1">
          <p class="mb-1 text-xs text-slate-500">关键字（相对路径）</p>
          <n-input v-model:value="keyword" placeholder="例如 sub/ 或 .mp4" clearable @keyup.enter="search" />
        </div>
        <div class="min-w-[180px]">
          <p class="mb-1 text-xs text-slate-500">文件夹</p>
          <select
            v-model="folderFilter"
            class="h-[34px] w-full rounded-md border border-slate-300 bg-white px-2 text-sm"
            @change="search"
          >
            <option value="">全部文件夹</option>
            <option
              v-for="f in folders.filter((x) => x.root_index >= 0 && x.folder_path !== '')"
              :key="`${f.root_index}:${f.folder_path}`"
              :value="f.folder_path"
            >
              {{ f.root_name }} › {{ f.folder_path }}
            </option>
          </select>
        </div>
        <n-button type="primary" @click="search">查询</n-button>
        <n-button @click="load">刷新</n-button>
      </div>
      <p class="mt-2 text-xs text-slate-400">
        分别按普通用户与游客视角显示生效状态；管理员预览不受策略限制。共 {{ total }} 个文件。
      </p>
    </n-card>

    <n-spin :show="loading">
      <div class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <table class="w-full min-w-[860px] text-sm">
          <thead class="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th class="px-4 py-2.5 text-left font-medium">文件</th>
              <th class="px-4 py-2.5 text-left font-medium">类型</th>
              <th class="px-4 py-2.5 text-left font-medium">大小 / 时长</th>
              <th class="px-4 py-2.5 text-left font-medium">普通用户</th>
              <th class="px-4 py-2.5 text-left font-medium">游客</th>
              <th class="px-4 py-2.5 text-left font-medium">文件级策略</th>
              <th class="px-4 py-2.5 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="f in files" :key="f.key" class="hover:bg-slate-50/70">
              <td class="max-w-[320px] px-4 py-2.5">
                <div class="flex items-center gap-1.5">
                  <n-tag size="tiny" :bordered="false">{{ f.root_name }}</n-tag>
                  <span class="truncate font-mono text-xs text-slate-800" :title="f.rel_path">{{ f.rel_path }}</span>
                </div>
                <p class="mt-0.5 font-mono text-[10px] text-slate-400">{{ f.key }}</p>
              </td>
              <td class="px-4 py-2.5 text-xs text-slate-600">{{ f.media_type === 'video' ? '视频' : '图片' }}</td>
              <td class="px-4 py-2.5 text-xs text-slate-500">
                {{ formatBytes(f.size) }}<span v-if="f.duration"> · {{ formatDuration(f.duration) }}</span>
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="userStatus(f).type" :bordered="false">{{ userStatus(f).text }}</n-tag>
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="guestStatus(f).type" :bordered="false">{{ guestStatus(f).text }}</n-tag>
              </td>
              <td class="px-4 py-2.5 text-xs text-slate-500">
                <template v-if="f.override">
                  <span class="font-medium text-slate-700">
                    普通{{ f.override.enabled ? '开' : '关' }} / 游客{{ f.override.guest_enabled ? '开' : '关' }}
                  </span>
                  <span v-if="f.override.schedule_type === 'time_window'" class="ml-1">
                    {{ f.override.time_start }}–{{ f.override.time_end }}
                  </span>
                  <p v-if="f.override.note" class="mt-0.5 text-[11px] text-slate-400">{{ f.override.note }}</p>
                </template>
                <span v-else class="text-slate-400">继承文件夹策略</span>
              </td>
              <td class="whitespace-nowrap px-4 py-2.5 text-right">
                <n-button size="tiny" quaternary type="primary" @click="openEdit(f)">设置</n-button>
                <n-button v-if="f.override" size="tiny" quaternary @click="clearOverride(f)">清除</n-button>
              </td>
            </tr>
            <tr v-if="!files.length && !loading">
              <td colspan="7" class="px-4 py-10 text-center text-sm text-slate-400">没有匹配的文件</td>
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

    <n-modal v-model:show="showModal" preset="card" style="width: 500px" title="文件级策略">
      <div v-if="current" class="space-y-4 text-sm">
        <div>
          <p class="text-xs text-slate-500">文件</p>
          <p class="mt-1 break-all rounded-lg bg-slate-50 px-3 py-2 font-mono text-xs">{{ current.rel_path }}</p>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <p class="font-medium">普通用户可播</p>
            <p class="text-xs text-slate-500">关闭将覆盖文件夹策略，直接禁用该文件</p>
          </div>
          <n-switch v-model:value="form.enabled" />
        </div>

        <div class="flex items-center justify-between">
          <div>
            <p class="font-medium">游客可播</p>
            <p class="text-xs text-slate-500">未登录访问的用户</p>
          </div>
          <n-switch v-model:value="form.guest_enabled" />
        </div>

        <div v-if="form.enabled || form.guest_enabled">
          <p class="mb-2 text-xs text-slate-500">播放时段（两个受众共用）</p>
          <n-radio-group v-model:value="form.schedule_type">
            <n-radio-button value="always">始终可播</n-radio-button>
            <n-radio-button value="time_window">时间窗口</n-radio-button>
          </n-radio-group>
        </div>

        <div
          v-if="(form.enabled || form.guest_enabled) && form.schedule_type === 'time_window'"
          class="grid grid-cols-2 gap-3"
        >
          <div>
            <p class="mb-1 text-xs text-slate-500">开始时间</p>
            <n-input v-model:value="form.time_start" placeholder="18:00" />
          </div>
          <div>
            <p class="mb-1 text-xs text-slate-500">结束时间</p>
            <n-input v-model:value="form.time_end" placeholder="23:00" />
          </div>
        </div>

        <div>
          <p class="mb-1 text-xs text-slate-500">备注（写入审计日志，便于追溯）</p>
          <n-input v-model:value="form.note" type="textarea" :rows="2" placeholder="例如：片源授权到期，暂时禁用" />
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
