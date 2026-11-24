<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NButton, NCard, NInput, NModal, NSpin, NSwitch, NTag, useMessage } from 'naive-ui'
import { fetchStats, resetToInitial, setRegistration, triggerScan } from '@/api/admin'
import { getFolderHeat, getMediaHeat, getUserHeat } from '@/api/watch'
import { formatDuration, formatTime } from '@/lib/format'
import type { AdminStats, HeatView } from '@/api/types'

interface UserHeatRow {
  user_id: number
  username: string
  watched_videos: number
  completed_videos: number
  total_watch_time: number
  last_played_at: string
}

const message = useMessage()
const stats = ref<AdminStats | null>(null)
const loading = ref(true)
const scanning = ref(false)
const savingRegistration = ref(false)
/** 需求 5：热度排行（媒体 / 文件夹 / 用户）。 */
const mediaHeat = ref<HeatView[]>([])
const folderHeat = ref<HeatView[]>([])
const userHeat = ref<UserHeatRow[]>([])
const heatTab = ref<'media' | 'folder' | 'user'>('media')
const resetModal = ref(false)
const resetConfirm = ref('')
const resetting = ref(false)

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

const activeHeat = computed(() =>
  heatTab.value === 'media' ? mediaHeat.value : heatTab.value === 'folder' ? folderHeat.value : [],
)

const logCategories = computed(() => Object.entries(stats.value?.log_counts ?? {}).sort((a, b) => b[1] - a[1]))

async function load() {
  loading.value = true
  try {
    const [s, mh, fh, uh] = await Promise.all([
      fetchStats(), getMediaHeat(20), getFolderHeat(20), getUserHeat(20),
    ])
    stats.value = s.data
    mediaHeat.value = mh.data
    folderHeat.value = fh.data
    userHeat.value = uh.data as unknown as UserHeatRow[]
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
    // 当前账号与 Token 已清空，跳回初始化页重新创建管理员。
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

    <!-- 需求 5：热度 / 播放统计（供推荐算法参考，接口已预留） -->
    <n-card title="观看热度与统计" size="small">
      <div class="mb-3 flex gap-1.5">
        <n-button
          v-for="t in ([['media','媒体'],['folder','文件夹'],['user','用户']] as const)"
          :key="t[0]"
          :type="heatTab === t[0] ? 'primary' : 'default'"
          size="small"
          @click="heatTab = t[0]"
        >{{ t[1] }}</n-button>
        <span class="ml-auto text-[11px] text-slate-400">热度 = log(完播)×12 + log(时长/60)×6 + log(收藏)×3 + log(播放)，完播权重最高</span>
      </div>

      <!-- 媒体 / 文件夹热度表 -->
      <div v-if="heatTab !== 'user'" class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="text-slate-500">
            <tr>
              <th class="py-1.5 pr-2">{{ heatTab === 'media' ? '媒体' : '文件夹' }}</th>
              <th class="py-1.5 pr-2">热度</th>
              <th class="py-1.5 pr-2">播放</th>
              <th class="py-1.5 pr-2">完播</th>
              <th class="py-1.5 pr-2">时长</th>
              <th class="py-1.5 pr-2">收藏</th>
              <th v-if="heatTab === 'folder'" class="py-1.5 pr-2">文件数</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(h, i) in activeHeat" :key="h.key ?? `${h.root_index}:${h.folder}`" class="border-t border-slate-100">
              <td
                class="max-w-[260px] truncate py-1.5 pr-2"
                :title="heatTab === 'media' ? h.rel_path : `${h.root_name || `媒体源 ${h.root_index}`} / ${h.folder || '根目录'}`"
              >
                <span class="mr-1 text-slate-400">{{ i + 1 }}.</span>
                <template v-if="heatTab === 'media'">{{ h.rel_path?.split('/').pop() ?? h.key }}</template>
                <template v-else>
                  <span class="text-slate-400">{{ h.root_name || `媒体源 ${h.root_index}` }}</span>
                  <span class="mx-1 text-slate-300">/</span>
                  <span :class="h.folder ? '' : 'text-slate-500'">{{ h.folder || '根目录' }}</span>
                </template>
              </td>
              <td class="py-1.5 pr-2 font-medium tabular-nums text-slate-700">{{ h.heat_score.toFixed(1) }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ h.play_count }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ h.completed_count }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ formatDuration(h.watched_seconds) }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ h.favorite_count }}</td>
              <td v-if="heatTab === 'folder'" class="py-1.5 pr-2 tabular-nums">{{ h.media_count ?? '—' }}</td>
            </tr>
            <tr v-if="!activeHeat.length">
              <td :colspan="heatTab === 'folder' ? 7 : 6" class="py-4 text-center text-slate-400">暂无统计数据</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 用户观看排行 -->
      <div v-else class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="text-slate-500">
            <tr>
              <th class="py-1.5 pr-2">用户</th>
              <th class="py-1.5 pr-2">观看视频</th>
              <th class="py-1.5 pr-2">完整播完</th>
              <th class="py-1.5 pr-2">观看时长</th>
              <th class="py-1.5 pr-2">最近观看</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(u, i) in userHeat" :key="u.user_id" class="border-t border-slate-100">
              <td class="py-1.5 pr-2"><span class="mr-1 text-slate-400">{{ i + 1 }}.</span>{{ u.username }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ u.watched_videos }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ u.completed_videos }}</td>
              <td class="py-1.5 pr-2 tabular-nums">{{ formatDuration(u.total_watch_time) }}</td>
              <td class="py-1.5 pr-2">{{ formatTime(u.last_played_at) }}</td>
            </tr>
            <tr v-if="!userHeat.length">
              <td colspan="5" class="py-4 text-center text-slate-400">暂无观看记录</td>
            </tr>
          </tbody>
        </table>
      </div>
    </n-card>

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

    <n-card title="危险操作" size="small" class="border-red-200">
      <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div class="text-xs text-slate-500">
          <p class="font-medium text-red-600">恢复初始化</p>
          <p class="mt-1">清空所有账号、收藏、历史、策略、统计与日志，回到首次部署状态。不会删除媒体文件。</p>
        </div>
        <n-button type="error" strong @click="resetModal = true">恢复初始化</n-button>
      </div>
    </n-card>

    <n-modal
      v-model:show="resetModal"
      preset="card"
      title="恢复初始化"
      class="max-w-md"
      :bordered="false"
    >
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
