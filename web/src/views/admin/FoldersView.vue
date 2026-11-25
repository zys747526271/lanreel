<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
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
import { formatDuration } from '@/lib/format'
import type { FolderPolicyView, ScheduleType } from '@/api/types'

const message = useMessage()
const dialog = useDialog()
const route = useRoute()

const folders = ref<FolderPolicyView[]>([])
const loading = ref(true)
const saving = ref(false)
const showModal = ref(false)
/** 折叠的子目录集合：key = rootIndex:folderPath（v0.3.0 需求 10）。 */
const collapsed = ref<Set<string>>(new Set())
const editing = ref<EditState>(blankEdit())

interface EditState {
  root_index: number
  folder_path: string
  enabled: boolean
  guest_enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
  guest_schedule_type: ScheduleType
  guest_time_start: string
  guest_time_end: string
  sync_children: boolean
}

function blankEdit(): EditState {
  return {
    root_index: 0, folder_path: '', enabled: true, guest_enabled: true,
    schedule_type: 'always', time_start: '18:00', time_end: '23:00',
    guest_schedule_type: 'always', guest_time_start: '18:00', guest_time_end: '23:00',
    sync_children: false,
  }
}

const totalMedia = computed(() => folders.value.reduce((sum, f) => sum + f.total, 0))
const configuredCount = computed(() => folders.value.filter((f) => f.configured).length)

/** 按根分组，构建「根 → 子目录」的成本。子目录行按路径深度排序。 */
const grouped = computed(() => {
  const map = new Map<number, FolderPolicyView[]>()
  for (const f of folders.value) {
    const arr = map.get(f.root_index) ?? []
    arr.push(f)
    map.set(f.root_index, arr)
  }
  for (const arr of map.values()) {
    arr.sort((a, b) => {
      const depth = (p: string) => (p ? p.split('/').length : 0)
      return depth(a.folder_path) - depth(b.folder_path) || a.folder_path.localeCompare(b.folder_path)
    })
  }
  return map
})

/** 某子目录是否处于折叠的根下面（隐藏显示）。 */
function hiddenByRoot(root: number, path: string): boolean {
  if (!path) return false
  return collapsed.value.has(`${root}:`)
}

function toggleRoot(root: number) {
  const key = `${root}:`
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsed.value = next
}

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
    root_index: folder.root_index,
    folder_path: folder.folder_path,
    enabled: folder.enabled,
    guest_enabled: folder.guest_enabled,
    schedule_type: folder.configured ? folder.schedule_type : 'always',
    time_start: folder.time_start || '18:00',
    time_end: folder.time_end || '23:00',
    guest_schedule_type: folder.configured ? (folder.guest_schedule_type || 'always') : 'always',
    guest_time_start: folder.guest_time_start || '18:00',
    guest_time_end: folder.guest_time_end || '23:00',
    sync_children: false,
  }
  showModal.value = true
}

async function save() {
  const e = editing.value
  if (e.schedule_type === 'time_window' && (!e.time_start || !e.time_end)) {
    message.warning('请填写普通用户时间窗口的起止时间')
    return
  }
  if (e.guest_schedule_type === 'time_window' && (!e.guest_time_start || !e.guest_time_end)) {
    message.warning('请填写游客时间窗口的起止时间')
    return
  }
  saving.value = true
  try {
    await setFolderPolicy({
      root_index: e.root_index,
      folder_path: e.folder_path,
      enabled: e.enabled,
      guest_enabled: e.guest_enabled,
      schedule_type: e.enabled ? e.schedule_type : 'disabled',
      time_start: e.time_start,
      time_end: e.time_end,
      guest_schedule_type: e.guest_enabled ? e.guest_schedule_type : 'disabled',
      guest_time_start: e.guest_time_start,
      guest_time_end: e.guest_time_end,
      sync_children: e.sync_children,
    })
    message.success(e.sync_children ? '策略已保存并同步到所有子文件夹' : '策略已保存，客户端缓存将在数秒内失效')
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
    content: `确认删除「${describe(folder)}」的策略？该目录将回落到上级或默认允许。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteFolderPolicy(folder.root_index, folder.folder_path)
        message.success('已恢复默认策略')
        await load()
      } catch (err) {
        message.error(err instanceof Error ? err.message : '删除失败')
      }
    },
  })
}

function userScheduleLabel(f: FolderPolicyView) {
  if (f.schedule_type === 'time_window') return `${f.time_start}–${f.time_end}`
  if (f.schedule_type === 'disabled' || !f.enabled) return '不可见'
  return '始终'
}

function guestScheduleLabel(f: FolderPolicyView) {
  const t = f.guest_schedule_type || 'always'
  if (t === 'time_window') return `${f.guest_time_start}–${f.guest_time_end}`
  if (t === 'disabled' || !f.guest_enabled) return '不可见'
  return '始终'
}

function describe(f: FolderPolicyView) {
  if (f.root_index === -1) return f.folder_path ? `全局默认 › ${f.folder_path}` : '全局默认（所有根）'
  const folder = f.folder_path || '整个根目录'
  return `${f.root_name} › ${folder}`
}

function sourceLabel(f: FolderPolicyView) {
  if (f.configured) return { text: '单独配置', type: 'default' as const }
  if (f.inherited) {
    const from =
      f.inherited_root === -1
        ? `全局默认${f.inherited_from ? ' › ' + f.inherited_from : ''}`
        : `${f.inherited_from || '根目录'}`
    return { text: `继承「${from}」`, type: 'info' as const }
  }
  return { text: '默认允许', type: 'success' as const }
}

const focusKey = computed(() => {
  const root = Number(route.query.root)
  const folderParam = route.query.folder
  // 根目录的 folder 是空字符串，必须保留才能定位。
  if (!Number.isFinite(root) || folderParam === undefined) return ''
  return `${root}:${String(folderParam)}`
})

onMounted(async () => {
  if (focusKey.value) {
    // 目标可能在折叠的根下面：先展开再滚动定位。
    const next = new Set(collapsed.value)
    next.delete(`${Number(route.query.root)}:`)
    collapsed.value = next
  }
  await load()
  if (focusKey.value) {
    requestAnimationFrame(() => {
      document.querySelector('[data-folder-focus="true"]')?.scrollIntoView({ block: 'center' })
    })
  }
})
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
        策略优先级：文件级覆盖 &gt; 文件夹策略（同一媒体根内最长前缀）&gt; 全局默认 &gt; 默认允许。
        普通用户与游客的播放时段<strong class="text-slate-700">各自独立</strong>；按服务端时区判断，支持跨零点。
        根目录行可折叠/展开子文件夹，编辑根目录时可选「同步到所有子文件夹」。
      </p>
    </n-card>

    <n-spin :show="loading">
      <div v-for="[root, rows] in grouped" :key="root" class="mb-3 overflow-hidden rounded-xl border border-slate-200 bg-white">
        <table class="w-full min-w-[900px] text-sm">
          <thead class="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th class="px-4 py-2.5 text-left font-medium">位置</th>
              <th class="px-4 py-2.5 text-left font-medium">媒体</th>
              <th class="px-4 py-2.5 text-left font-medium">普通用户时段</th>
              <th class="px-4 py-2.5 text-left font-medium">游客时段</th>
              <th class="px-4 py-2.5 text-left font-medium">热度 / 播放 / 收藏</th>
              <th class="px-4 py-2.5 text-left font-medium">来源</th>
              <th class="px-4 py-2.5 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr
              v-for="f in rows.filter((x) => x.root_index === -1 || !hiddenByRoot(root, x.folder_path))"
              :key="`${f.root_index}:${f.folder_path}`"
              class="hover:bg-slate-50/70"
              :class="[
                f.folder_path === '' ? 'bg-slate-50/40 font-medium' : '',
                focusKey === `${f.root_index}:${f.folder_path}` ? 'bg-emerald-50/80 ring-1 ring-inset ring-emerald-200' : '',
              ]"
              :data-folder-focus="focusKey === `${f.root_index}:${f.folder_path}` ? 'true' : undefined"
            >
              <td class="max-w-[340px] px-4 py-2.5">
                <div class="flex flex-wrap items-center gap-1.5">
                  <button
                    v-if="f.folder_path === '' && root >= 0"
                    class="flex h-5 w-5 items-center justify-center rounded text-slate-500 hover:bg-slate-200"
                    :title="collapsed.has(`${root}:`) ? '展开子文件夹' : '收起子文件夹'"
                    @click="toggleRoot(root)"
                  >
                    <svg
                      class="h-3.5 w-3.5 transition"
                      :class="collapsed.has(`${root}:`) ? '-rotate-90' : ''"
                      viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
                    ><path d="m6 9 6 6 6-6" /></svg>
                  </button>
                  <n-tag size="tiny" :bordered="false" type="default">
                    {{ f.root_index === -1 ? '全局默认' : f.root_name }}
                  </n-tag>
                  <span class="font-mono text-xs text-slate-800">
                    {{ f.folder_path || (f.root_index === -1 ? '所有根目录' : '整个根目录') }}
                  </span>
                  <n-tag v-if="f.folder_path === '' && f.recursive" size="tiny" :bordered="false" type="info">递归</n-tag>
                </div>
              </td>
              <td class="px-4 py-2.5 text-xs text-slate-500">
                {{ f.total }} 项<span class="text-slate-300"> · </span>视频 {{ f.videos }} · 图片 {{ f.images }}
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="f.enabled && f.schedule_type !== 'disabled' ? 'success' : 'error'" :bordered="false">
                  {{ userScheduleLabel(f) }}
                </n-tag>
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="f.guest_enabled && f.guest_schedule_type !== 'disabled' ? 'success' : 'error'" :bordered="false">
                  {{ guestScheduleLabel(f) }}
                </n-tag>
              </td>
              <td class="px-4 py-2.5 text-xs tabular-nums text-slate-600">
                <span :title="`播放 ${f.play_count} 次 · 观看 ${formatDuration(f.watched_seconds)} · 收藏 ${f.favorite_count} 次`">
                  {{ f.play_count }} 播 · {{ formatDuration(f.watched_seconds) }} · {{ f.favorite_count }} 藏
                </span>
              </td>
              <td class="px-4 py-2.5">
                <n-tag size="small" :type="sourceLabel(f).type" :bordered="false">{{ sourceLabel(f).text }}</n-tag>
              </td>
              <td class="whitespace-nowrap px-4 py-2.5 text-right">
                <n-button size="tiny" quaternary type="primary" @click="openEdit(f)">编辑</n-button>
                <n-button v-if="f.configured" size="tiny" quaternary @click="resetPolicy(f)">恢复默认</n-button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!folders.length && !loading" class="py-10 text-center text-sm text-slate-400">
        尚未扫描到任何媒体目录，请到「媒体目录」页添加
      </p>
    </n-spin>

    <n-modal v-model:show="showModal" preset="card" style="width: 560px" title="设置文件夹策略">
      <div class="space-y-5 text-sm">
        <div>
          <p class="text-xs text-slate-500">位置</p>
          <p class="mt-1 rounded-lg bg-slate-50 px-3 py-2 font-mono text-xs">
            {{ editing.root_index === -1 ? '全局默认' : '根 ' + editing.root_index }} ·
            {{ editing.folder_path || '整个根目录' }}
          </p>
        </div>

        <!-- 普通用户 -->
        <div class="rounded-lg border border-slate-200 p-3">
          <div class="mb-2 flex items-center justify-between">
            <p class="font-medium">普通用户</p>
            <n-switch v-model:value="editing.enabled" />
          </div>
          <template v-if="editing.enabled">
            <p class="mb-1.5 text-xs text-slate-500">播放时段</p>
            <n-radio-group v-model:value="editing.schedule_type" size="small">
              <n-radio-button value="always">始终可播</n-radio-button>
              <n-radio-button value="time_window">时间窗口</n-radio-button>
            </n-radio-group>
            <div v-if="editing.schedule_type === 'time_window'" class="mt-2 grid grid-cols-2 gap-3">
              <div>
                <p class="mb-1 text-xs text-slate-500">开始 HH:MM</p>
                <n-input v-model:value="editing.time_start" placeholder="18:00" />
              </div>
              <div>
                <p class="mb-1 text-xs text-slate-500">结束 HH:MM</p>
                <n-input v-model:value="editing.time_end" placeholder="23:00" />
              </div>
            </div>
          </template>
        </div>

        <!-- 游客（独立时段，需求 9） -->
        <div class="rounded-lg border border-slate-200 p-3">
          <div class="mb-2 flex items-center justify-between">
            <p class="font-medium">游客</p>
            <n-switch v-model:value="editing.guest_enabled" />
          </div>
          <template v-if="editing.guest_enabled">
            <p class="mb-1.5 text-xs text-slate-500">播放时段（与普通用户独立）</p>
            <n-radio-group v-model:value="editing.guest_schedule_type" size="small">
              <n-radio-button value="always">始终可播</n-radio-button>
              <n-radio-button value="time_window">时间窗口</n-radio-button>
            </n-radio-group>
            <div v-if="editing.guest_schedule_type === 'time_window'" class="mt-2 grid grid-cols-2 gap-3">
              <div>
                <p class="mb-1 text-xs text-slate-500">开始 HH:MM</p>
                <n-input v-model:value="editing.guest_time_start" placeholder="18:00" />
              </div>
              <div>
                <p class="mb-1 text-xs text-slate-500">结束 HH:MM</p>
                <n-input v-model:value="editing.guest_time_end" placeholder="23:00" />
              </div>
            </div>
          </template>
        </div>

        <div v-if="editing.root_index >= 0 && editing.folder_path === ''" class="flex items-center justify-between rounded-lg bg-slate-50 p-3">
          <div>
            <p class="font-medium">同步到所有子文件夹</p>
            <p class="text-xs text-slate-500">把当前根目录策略一键应用到该根下全部子文件夹（需求 10）</p>
          </div>
          <n-switch v-model:value="editing.sync_children" />
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
