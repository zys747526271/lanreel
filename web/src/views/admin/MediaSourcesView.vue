<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NInput,
  NModal,
  NSpin,
  NTag,
  useMessage,
} from 'naive-ui'
import {
  getMediaSettings,
  listDirs,
  resetMediaRoots,
  setMediaRoots,
  triggerScan,
} from '@/api/admin'
import type { DirListing, MediaSettings } from '@/api/types'

const message = useMessage()

const loading = ref(true)
const saving = ref(false)
const resetting = ref(false)
const scanning = ref(false)
const settings = ref<MediaSettings | null>(null)
/** 编辑中的目录列表（保存前不影响运行） */
const draft = ref<string[]>([])
const newPath = ref('')

const pickerOpen = ref(false)
const pickerLoading = ref(false)
const listing = ref<DirListing | null>(null)

let pollTimer: number | undefined

const dirty = computed(() => {
  if (!settings.value) return false
  return JSON.stringify(draft.value) !== JSON.stringify(settings.value.roots)
})

const busy = computed(() => saving.value || resetting.value || !!settings.value?.scanning)

function formatTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN', { hour12: false })
}

async function load() {
  loading.value = true
  try {
    const s = (await getMediaSettings()).data
    settings.value = s
    draft.value = [...s.roots]
    if (s.scanning) schedulePoll()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载媒体目录失败')
  } finally {
    loading.value = false
  }
}

function stopPoll() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

/** 保存/恢复后轮询扫描结果，直到 scanning=false。 */
function schedulePoll() {
  stopPoll()
  let ticks = 0
  pollTimer = window.setInterval(async () => {
    ticks += 1
    try {
      const s = (await getMediaSettings()).data
      settings.value = s
      if (!s.scanning) {
        stopPoll()
        message.success(`扫描完成：共 ${s.scan.total} 项（视频 ${s.scan.videos} / 图片 ${s.scan.images}）`)
      }
    } catch {
      stopPoll()
    }
    if (ticks > 200) stopPoll()
  }, 1200)
}

function normalizeInput(v: string) {
  let p = v.trim()
  // 保留盘符根（如 C:\），其余去掉结尾分隔符
  if (p.length > 3) p = p.replace(/[\\/]+$/, '')
  return p
}

function addPath(p?: string) {
  const v = normalizeInput(p ?? newPath.value)
  if (!v) return
  if (draft.value.some((x) => x.toLowerCase() === v.toLowerCase())) {
    message.warning('该目录已在列表中')
    return
  }
  draft.value = [...draft.value, v]
  newPath.value = ''
}

function removePath(idx: number) {
  draft.value = draft.value.filter((_, i) => i !== idx)
}

function revertDraft() {
  if (settings.value) draft.value = [...settings.value.roots]
}

async function save() {
  if (!draft.value.length) {
    message.warning('至少保留一个媒体目录')
    return
  }
  saving.value = true
  try {
    const s = (await setMediaRoots(draft.value)).data
    settings.value = s
    draft.value = [...s.roots]
    message.success('已保存，正在后台扫描')
    schedulePoll()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function resetToConfig() {
  resetting.value = true
  try {
    const s = (await resetMediaRoots()).data
    settings.value = s
    draft.value = [...s.roots]
    message.success('已恢复为配置文件中的目录')
    schedulePoll()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '恢复失败')
  } finally {
    resetting.value = false
  }
}

async function scanNow() {
  scanning.value = true
  try {
    const res = await triggerScan()
    message.success(`扫描完成：共 ${res.data.media_total} 项`)
    await load()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '扫描失败')
  } finally {
    scanning.value = false
  }
}

async function openPicker() {
  pickerOpen.value = true
  await browse('')
}

async function browse(path: string) {
  pickerLoading.value = true
  try {
    listing.value = (await listDirs(path)).data
  } catch (err) {
    message.error(err instanceof Error ? err.message : '无法读取该目录')
  } finally {
    pickerLoading.value = false
  }
}

function chooseCurrent() {
  const cur = listing.value?.current
  if (!cur) return
  addPath(cur)
  pickerOpen.value = false
}

onMounted(load)
onBeforeUnmount(stopPoll)
</script>

<template>
  <n-spin :show="loading">
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <n-card class="lg:col-span-2" title="媒体目录" size="small">
        <template #header-extra>
          <n-tag v-if="settings" size="small" :type="settings.source === 'database' ? 'success' : 'default'">
            {{ settings.source === 'database' ? '后台设置生效中' : '来自配置文件' }}
          </n-tag>
        </template>

        <div class="space-y-3">
          <p class="text-xs leading-relaxed text-slate-500">
            这里配置「想看哪些文件夹」。只能填服务器本机的目录，保存后立即重新扫描；
            视频与图片会按文件扩展名识别，子目录自动递归。
          </p>

          <div v-if="draft.length" class="space-y-2">
            <div
              v-for="(p, idx) in draft"
              :key="p"
              class="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2"
            >
              <span class="w-5 shrink-0 text-center text-[11px] text-slate-400">{{ idx + 1 }}</span>
              <code class="min-w-0 flex-1 break-all text-xs text-slate-700">{{ p }}</code>
              <n-button size="tiny" quaternary type="error" @click="removePath(idx)">移除</n-button>
            </div>
          </div>
          <n-empty v-else description="还没有目录，添加一个包含视频或图片的文件夹" size="small" />

          <div class="flex flex-col gap-2 sm:flex-row">
            <n-input
              v-model:value="newPath"
              size="small"
              placeholder="例如 D:\Videos 或 /mnt/media"
              @keyup.enter="addPath()"
            />
            <div class="flex gap-2">
              <n-button size="small" @click="openPicker">浏览…</n-button>
              <n-button size="small" type="primary" secondary @click="addPath()">添加</n-button>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3">
            <n-button type="primary" size="small" :loading="saving" :disabled="!dirty" @click="save">
              保存并重新扫描
            </n-button>
            <n-button size="small" :disabled="!dirty" @click="revertDraft">撤销修改</n-button>
            <n-button
              v-if="settings?.source === 'database'"
              size="small"
              quaternary
              :loading="resetting"
              @click="resetToConfig"
            >
              恢复配置文件设置
            </n-button>
            <span v-if="dirty" class="text-[11px] text-amber-600">有未保存的修改</span>
          </div>

          <n-alert v-if="settings?.source === 'config'" type="default" :bordered="false" size="small">
            当前使用配置文件 <code>media.roots</code> 的值。在这里保存后会以后台设置为准，
            配置文件的目录不再生效，可随时「恢复配置文件设置」。
          </n-alert>

          <n-alert type="warning" :bordered="false" size="small">
            更换目录会重新计算每个文件的标识，原有收藏可能变成「文件不存在」；仅新增目录时旧收藏不受影响。
          </n-alert>
        </div>
      </n-card>

      <n-card title="扫描状态" size="small">
        <div class="space-y-3 text-sm">
          <div class="grid grid-cols-3 gap-2 text-center">
            <div class="rounded-lg bg-slate-50 py-2">
              <p class="text-lg font-semibold text-slate-800">{{ settings?.media_total ?? 0 }}</p>
              <p class="text-[11px] text-slate-500">总条目</p>
            </div>
            <div class="rounded-lg bg-slate-50 py-2">
              <p class="text-lg font-semibold text-slate-800">{{ settings?.scan.videos ?? 0 }}</p>
              <p class="text-[11px] text-slate-500">视频</p>
            </div>
            <div class="rounded-lg bg-slate-50 py-2">
              <p class="text-lg font-semibold text-slate-800">{{ settings?.scan.images ?? 0 }}</p>
              <p class="text-[11px] text-slate-500">图片</p>
            </div>
          </div>

          <div class="space-y-1 text-xs text-slate-500">
            <p>生效目录：{{ settings?.roots.length ?? 0 }} 个 · 配置上限 16 个</p>
            <p>上次扫描：{{ formatTime(settings?.scan.finished_at) }}</p>
            <p>耗时：{{ settings?.scan.duration_ms ?? 0 }} ms · 跳过 {{ settings?.scan.skipped ?? 0 }} 个文件</p>
            <p>
              自动重扫间隔：{{
                settings?.scan_interval_seconds ? `${settings.scan_interval_seconds} 秒` : '仅启动/手动扫描'
              }}
            </p>
          </div>

          <div v-if="settings?.scan.errors?.length" class="rounded-lg bg-amber-50 p-2 text-[11px] text-amber-700">
            <p class="font-medium">部分目录读取失败（{{ settings.scan.errors.length }}）</p>
            <p v-for="e in settings.scan.errors.slice(0, 3)" :key="e" class="mt-1 break-all">{{ e }}</p>
          </div>

          <n-button block size="small" :loading="scanning || !!settings?.scanning" @click="scanNow">
            {{ settings?.scanning ? '扫描中…' : '立即扫描' }}
          </n-button>
        </div>
      </n-card>
    </div>

    <n-modal
      v-model:show="pickerOpen"
      preset="card"
      title="选择文件夹"
      style="max-width: 560px"
      :bordered="false"
    >
      <div class="space-y-3">
        <div class="flex items-center gap-2">
          <n-button size="tiny" :disabled="!listing?.parent" @click="browse(listing?.parent || '')">
            上一级
          </n-button>
          <code class="min-w-0 flex-1 break-all rounded bg-slate-50 px-2 py-1 text-xs text-slate-600">
            {{ listing?.current || '根目录' }}
          </code>
        </div>
        <div class="max-h-72 overflow-y-auto rounded-lg border border-slate-200">
          <n-spin :show="pickerLoading">
            <div v-if="listing?.entries.length" class="divide-y divide-slate-100">
              <button
                v-for="e in listing.entries"
                :key="e.path"
                class="flex w-full items-center justify-between px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-50"
                @click="browse(e.path)"
              >
                <span class="truncate">{{ e.name }}</span>
                <span class="text-xs text-slate-400">进入 →</span>
              </button>
            </div>
            <p v-else class="px-3 py-6 text-center text-xs text-slate-400">
              {{ pickerLoading ? '读取中…' : '没有可进入的子目录' }}
            </p>
          </n-spin>
        </div>
        <p class="text-[11px] text-slate-400">
          进入目标文件夹后点「选择此目录」。以点开头的隐藏目录不列出。
        </p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button size="small" @click="pickerOpen = false">取消</n-button>
          <n-button size="small" type="primary" :disabled="!listing?.current" @click="chooseCurrent">
            选择此目录
          </n-button>
        </div>
      </template>
    </n-modal>
  </n-spin>
</template>
