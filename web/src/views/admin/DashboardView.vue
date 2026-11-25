<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NCard, NSpin, NSwitch, NTag, useMessage } from 'naive-ui'
import { fetchStats, setRegistration, triggerScan } from '@/api/admin'
import { getFolderHeatPage, getMediaHeatPage, getUserHeatPage } from '@/api/watch'
import { formatDuration, formatTime } from '@/lib/format'
import type { AdminStats, HeatView, UserHeatRow } from '@/api/types'
import type { HeatSortKey, HeatOrder } from '@/api/watch'

type HeatTab = 'media' | 'folder' | 'user'

const message = useMessage()
const router = useRouter()
const stats = ref<AdminStats | null>(null)
const loading = ref(true)
const heatLoading = ref(false)
const scanning = ref(false)
const savingRegistration = ref(false)

const mediaHeat = ref<HeatView[]>([])
const folderHeat = ref<HeatView[]>([])
const userHeat = ref<UserHeatRow[]>([])
const heatTab = ref<HeatTab>('media')
const page = ref(1)
const pageSize = 10
const total = ref(0)
const sortKey = ref<Record<HeatTab, HeatSortKey>>({ media: 'heat', folder: 'heat', user: 'name' })
const order = ref<Record<HeatTab, HeatOrder>>({ media: 'desc', folder: 'desc', user: 'asc' })
// 媒体统计里已移除文件的历史行默认不显示；关闭开关可重新查看。
const hideMissing = ref(true)

const sortColumns: Record<HeatTab, Array<{ key: HeatSortKey; label: string }>> = {
  media: [
    { key: 'name', label: '媒体' },
    { key: 'heat', label: '热度' },
    { key: 'play', label: '播放' },
    { key: 'complete', label: '完播' },
    { key: 'watched', label: '时长' },
    { key: 'favorite', label: '收藏' },
  ],
  folder: [
    { key: 'name', label: '文件夹' },
    { key: 'heat', label: '热度' },
    { key: 'play', label: '播放' },
    { key: 'complete', label: '完播' },
    { key: 'watched', label: '时长' },
    { key: 'favorite', label: '收藏' },
  ],
  user: [
    { key: 'name', label: '用户' },
    { key: 'play', label: '观看视频' },
    { key: 'complete', label: '完整播完' },
    { key: 'watched', label: '观看时长' },
  ],
}

const cards = computed(() => {
  const s = stats.value
  if (!s) return []
  const w = s.watch_summary
  const today = s.today_watch_summary
  return [
    { label: '媒体总数', value: s.media_total, hint: `视频 ${s.videos} · 图片 ${s.images}` },
    { label: '用户', value: s.users, hint: `管理员 ${s.admins}` },
    { label: '今日全站观看', value: today?.watched_videos ?? '—', hint: '今日有真实播放的视频数' },
    { label: '今日观看时长', value: today ? formatDuration(today.total_watch_time) : '—', hint: '按今日实际观看累计' },
    { label: '历史全站观看', value: w?.watched_videos ?? '—', hint: `完整播完 ${w?.completed_videos ?? '—'} 次` },
    { label: '历史观看时长', value: w ? formatDuration(w.total_watch_time) : '—', hint: '按实际观看部分累计' },
  ]
})

const logCategories = computed(() => Object.entries(stats.value?.log_counts ?? {}).sort((a, b) => b[1] - a[1]))

async function loadStats() {
  loading.value = true
  try {
    stats.value = (await fetchStats()).data
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadHeat() {
  heatLoading.value = true
  const q = {
    page: page.value,
    pageSize,
    sort: sortKey.value[heatTab.value],
    order: order.value[heatTab.value],
    hideMissing: hideMissing.value,
  }
  try {
    if (heatTab.value === 'media') {
      const res = await getMediaHeatPage(q)
      mediaHeat.value = res.data
      total.value = res.meta.total
    } else if (heatTab.value === 'folder') {
      const res = await getFolderHeatPage(q)
      folderHeat.value = res.data
      total.value = res.meta.total
    } else {
      const res = await getUserHeatPage(q)
      userHeat.value = res.data
      total.value = res.meta.total
    }
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载统计失败')
  } finally {
    heatLoading.value = false
  }
}

function switchTab(tab: HeatTab) {
  if (heatTab.value === tab) return
  heatTab.value = tab
  page.value = 1
  void loadHeat()
}

function toggleHideMissing(value: boolean) {
  hideMissing.value = value
  page.value = 1
  void loadHeat()
}

function toggleSort(key: HeatSortKey) {
  if (sortKey.value[heatTab.value] === key) {
    order.value[heatTab.value] = order.value[heatTab.value] === 'desc' ? 'asc' : 'desc'
  } else {
    sortKey.value[heatTab.value] = key
    order.value[heatTab.value] = key === 'name' ? 'asc' : 'desc'
  }
  page.value = 1
  void loadHeat()
}

function sortIcon(key: HeatSortKey) {
  if (sortKey.value[heatTab.value] !== key) return '↕'
  return order.value[heatTab.value] === 'desc' ? '↓' : '↑'
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
function changePage(next: number) {
  if (next < 1 || next > totalPages.value) return
  page.value = next
  void loadHeat()
}

/** 兼容旧接口：没有 display_name 时从路径/ key 兜底，绝不渲染空白名称。 */
function mediaName(row: HeatView): string {
  // 新接口已返回文件名（必要时带「已移除」前缀）；旧接口仍从完整路径截取。
  if (row.display_name) return row.display_name
  const raw = row.rel_path || row.key || ''
  const parts = raw.split(/[\\/]/)
  return parts[parts.length - 1] || '未命名媒体'
}
function mediaTitle(row: HeatView): string {
  const path = row.rel_path || row.key || '未知路径'
  return row.missing ? `${path}（文件已移除，仅保留历史统计）` : path
}

function openMedia(row: HeatView) {
  if (row.missing || !row.rel_path) return
  void router.push({ path: '/admin/files', query: { focus: row.rel_path } })
}
function openFolder(row: HeatView) {
  void router.push({
    path: '/admin/folders',
    query: { root: String(row.root_index), folder: row.folder },
  })
}
function openUser(row: UserHeatRow) {
  void router.push({ path: '/admin/users', query: { focus: row.username } })
}

async function runScan() {
  scanning.value = true
  try {
    const res = await triggerScan()
    message.success(`扫描完成，当前共 ${res.data.media_total} 个媒体文件`)
    await Promise.all([loadStats(), loadHeat()])
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
    await loadStats()
  } finally {
    savingRegistration.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadStats(), loadHeat()])
})
</script>

<template>
  <div class="space-y-4">
    <n-spin :show="loading">
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        <n-card v-for="c in cards" :key="c.label" size="small" :bordered="true">
          <p class="text-xs text-slate-500">{{ c.label }}</p>
          <p class="mt-1 text-xl font-semibold tabular-nums">{{ c.value }}</p>
          <p class="mt-1 truncate text-[11px] text-slate-400" :title="c.hint">{{ c.hint }}</p>
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

          <!-- 日志分类分布并入扫描卡片，避免单独占一整行 -->
          <div class="rounded-lg border border-slate-200 p-3">
            <div class="mb-2 flex items-center justify-between">
              <p class="text-xs font-medium text-slate-600">日志分类分布</p>
              <router-link to="/admin/logs" class="text-[11px] text-slate-500 hover:underline">查看日志 →</router-link>
            </div>
            <div v-if="logCategories.length" class="flex flex-wrap gap-1.5">
              <n-tag v-for="[cat, count] in logCategories" :key="cat" size="tiny" :bordered="false">
                {{ cat }} · {{ count }}
              </n-tag>
            </div>
            <p v-else class="text-xs text-slate-400">暂无日志</p>
          </div>

          <div v-if="stats.scan.errors?.length" class="rounded-lg bg-amber-50 p-3 text-xs text-amber-700">
            <p class="font-medium">扫描告警（{{ stats.scan.errors.length }}）</p>
            <ul class="mt-1 list-inside list-disc space-y-0.5">
              <li v-for="(e, i) in stats.scan.errors.slice(0, 3)" :key="i">{{ e }}</li>
            </ul>
          </div>
          <div class="flex gap-2">
            <n-button type="primary" :loading="scanning" @click="runScan">立即重新扫描</n-button>
            <n-button quaternary @click="loadStats">刷新数据</n-button>
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
          <router-link to="/admin/diagnostics" class="text-xs text-emerald-700 hover:underline">
            运行诊断 →
          </router-link>
          <router-link to="/admin/settings" class="block text-xs text-slate-700 hover:underline">
            前往系统设置 →
          </router-link>
        </div>
      </n-card>
    </div>

    <n-card title="观看热度与统计" size="small">
      <div class="mb-3 flex flex-wrap items-center gap-1.5">
        <n-button
          v-for="t in ([['media','媒体'],['folder','文件夹'],['user','用户']] as const)"
          :key="t[0]"
          :type="heatTab === t[0] ? 'primary' : 'default'"
          size="small"
          @click="switchTab(t[0])"
        >{{ t[1] }}</n-button>
        <label v-if="heatTab === 'media'" class="ml-2 flex items-center gap-1.5 text-[11px] text-slate-500">
          <n-switch :value="hideMissing" size="small" @update:value="toggleHideMissing" />
          隐藏已移除文件
        </label>
        <span class="ml-auto text-[11px] text-slate-400">点击表头可对全量数据排序，再分页显示；点击行可跳转到对应管理页</span>
      </div>

      <n-spin :show="heatLoading">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead class="text-slate-500">
              <tr>
                <th v-for="col in sortColumns[heatTab]" :key="col.key" class="py-1.5 pr-2 whitespace-nowrap">
                  <button class="inline-flex items-center gap-1 hover:text-slate-900" @click="toggleSort(col.key)">
                    {{ col.label }}
                    <span class="text-[10px] text-slate-400">{{ sortIcon(col.key) }}</span>
                  </button>
                </th>
                <th v-if="heatTab === 'folder'" class="py-1.5 pr-2">文件数</th>
                <th v-if="heatTab === 'user'" class="py-1.5 pr-2">最近观看</th>
              </tr>
            </thead>
            <tbody>
              <template v-if="heatTab === 'media'">
                <tr
                  v-for="(h, i) in mediaHeat"
                  :key="h.key ?? `${h.root_index}:${h.folder}`"
                  class="cursor-pointer border-t border-slate-100 hover:bg-slate-50"
                  @click="openMedia(h)"
                >
                  <td class="max-w-[260px] truncate py-1.5 pr-2" :title="mediaTitle(h)">
                    <span class="mr-1 text-slate-400">{{ (page - 1) * pageSize + i + 1 }}.</span>
                    <span :class="h.missing ? 'text-slate-400' : ''">{{ mediaName(h) }}</span>
                  </td>
                  <td class="py-1.5 pr-2 font-medium tabular-nums text-slate-700">{{ h.heat_score.toFixed(1) }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.play_count }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.completed_count }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ formatDuration(h.watched_seconds) }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.favorite_count }}</td>
                </tr>
                <tr v-if="!mediaHeat.length">
                  <td colspan="6" class="py-4 text-center text-slate-400">暂无统计数据</td>
                </tr>
              </template>

              <template v-else-if="heatTab === 'folder'">
                <tr
                  v-for="(h, i) in folderHeat"
                  :key="`${h.root_index}:${h.folder}`"
                  class="cursor-pointer border-t border-slate-100 hover:bg-slate-50"
                  @click="openFolder(h)"
                >
                  <td class="max-w-[260px] truncate py-1.5 pr-2">
                    <span class="mr-1 text-slate-400">{{ (page - 1) * pageSize + i + 1 }}.</span>
                    <span class="text-slate-400">{{ h.root_name || `媒体源 ${h.root_index}` }}</span>
                    <span class="mx-1 text-slate-300">/</span>
                    {{ h.folder || '根目录' }}
                  </td>
                  <td class="py-1.5 pr-2 font-medium tabular-nums text-slate-700">{{ h.heat_score.toFixed(1) }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.play_count }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.completed_count }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ formatDuration(h.watched_seconds) }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.favorite_count }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ h.media_count ?? '—' }}</td>
                </tr>
                <tr v-if="!folderHeat.length">
                  <td colspan="7" class="py-4 text-center text-slate-400">暂无统计数据</td>
                </tr>
              </template>

              <template v-else>
                <tr
                  v-for="(u, i) in userHeat"
                  :key="u.user_id"
                  class="cursor-pointer border-t border-slate-100 hover:bg-slate-50"
                  @click="openUser(u)"
                >
                  <td class="py-1.5 pr-2">
                    <span class="mr-1 text-slate-400">{{ (page - 1) * pageSize + i + 1 }}.</span>{{ u.username || `已注销用户 #${u.user_id}` }}
                  </td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ u.watched_videos }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ u.completed_videos }}</td>
                  <td class="py-1.5 pr-2 tabular-nums">{{ formatDuration(u.total_watch_time) }}</td>
                  <td class="py-1.5 pr-2">{{ formatTime(u.last_played_at) }}</td>
                </tr>
                <tr v-if="!userHeat.length">
                  <td colspan="5" class="py-4 text-center text-slate-400">暂无观看记录</td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>

        <div class="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-slate-500">
          <span>共 {{ total }} 条 · 第 {{ page }} / {{ totalPages }} 页 · 每页 {{ pageSize }} 条</span>
          <div class="flex gap-2">
            <n-button size="small" :disabled="page <= 1" @click="changePage(page - 1)">上一页</n-button>
            <n-button size="small" :disabled="page >= totalPages" @click="changePage(page + 1)">下一页</n-button>
          </div>
        </div>
      </n-spin>
    </n-card>
  </div>
</template>
