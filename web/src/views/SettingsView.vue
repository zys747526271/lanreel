<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useFavoritesStore } from '@/stores/favorites'
import { logger, type LogRecord } from '@/lib/logger'
import { countAll, clearStore } from '@/lib/db'
import { fetchPublicConfig, withToken } from '@/api/client'
import { formatTime, formatDuration } from '@/lib/format'
import {
  RATE_OPTIONS, END_ACTION_OPTIONS, prefs, resetPrefs,
  type PlaybackRate, type EndAction,
} from '@/lib/prefs'
import type { PublicConfig, WatchSummary } from '@/api/types'
import { playerReturnPath } from '@/lib/session'
import { getMyWatchSummary } from '@/api/watch'

const router = useRouter()
const auth = useAuthStore()
const favorites = useFavoritesStore()

const logs = ref<LogRecord[]>([])
const category = ref('')
const level = ref('')
const keyword = ref('')
const loading = ref(false)
const logCount = ref(0)
const favoriteCacheCount = ref(0)
const config = ref<PublicConfig | null>(null)
const hint = ref('')
/** 当前用户观看统计（需求 5）。 */
const watchSummary = ref<WatchSummary | null>(null)


const categories = [
  { value: '', label: '全部分类' },
  { value: 'api', label: 'api 接口' },
  { value: 'player', label: 'player 播放' },
  { value: 'interaction', label: 'interaction 交互' },
  { value: 'performance', label: 'performance 性能' },
  { value: 'exception', label: 'exception 异常' },
  { value: 'auth', label: 'auth 认证' },
  { value: 'app', label: 'app 应用' },
]

const levelTag: Record<string, string> = {
  debug: 'bg-slate-500/15 text-slate-300',
  info: 'bg-sky-500/15 text-sky-300',
  warn: 'bg-amber-500/15 text-amber-300',
  error: 'bg-red-500/15 text-red-300',
}

const filteredHint = computed(() => {
  if (!logs.value.length) return '暂无日志记录'
  return `共 ${logs.value.length} 条（最多展示 300 条）`
})

async function loadLogs() {
  // 本地日志仅管理员可见（普通用户的日志排障入口在管理后台）
  if (!auth.isAdmin) return
  loading.value = true
  try {
    logs.value = await logger.query({
      category: category.value || undefined,
      level: level.value || undefined,
      keyword: keyword.value || undefined,
      limit: 300,
    })
    logCount.value = await countAll('app_logs')
  } catch (err) {
    hint.value = `读取日志失败：${String(err)}`
  } finally {
    loading.value = false
  }
}

async function loadMeta() {
  try {
    favoriteCacheCount.value = await countAll('favorites')
    config.value = await fetchPublicConfig()
  } catch {
    /* 忽略 */
  }
  if (auth.isLoggedIn) {
    try {
      watchSummary.value = (await getMyWatchSummary()).data
    } catch {
      watchSummary.value = null
    }
  }
}

function download(content: string, filename: string, type: string) {
  const blob = new Blob([content], { type })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

async function exportJSON() {
  const content = await logger.export('json')
  download(content, `lanreel-logs-${Date.now()}.json`, 'application/json')
  hint.value = '已导出 JSON（敏感字段已脱敏）'
}

async function exportText() {
  const content = await logger.export('text')
  download(content, `lanreel-logs-${Date.now()}.txt`, 'text/plain')
  hint.value = '已导出文本日志（敏感字段已脱敏）'
}

async function clearLogs() {
  if (!window.confirm('确认清空本地日志？该操作不可恢复。')) return
  await logger.clear()
  await loadLogs()
  hint.value = '本地日志已清空'
}

async function clearCache() {
  if (!window.confirm('确认清空本地缓存（收藏缓存与用户状态）？下次进入会重新同步。')) return
  await clearStore('favorites')
  await clearStore('user_state')
  favoriteCacheCount.value = 0
  hint.value = '本地缓存已清空'
}

function setRate(value: PlaybackRate) {
  prefs.defaultRate = value
}

function setEndAction(value: EndAction) {
  prefs.endAction = value
}

function resetPreferences() {
  resetPrefs()
  hint.value = '播放设置已恢复默认'
}

async function signOut() {
  await auth.logout()
  favorites.reset()
  router.push('/login')
}

onMounted(async () => {
  await Promise.all([loadLogs(), loadMeta()])
})
</script>

<template>
  <div class="min-h-[100dvh] bg-[#08080a] px-4 py-5 text-white">
    <div class="mx-auto w-full max-w-2xl space-y-5">
      <header class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <button
            class="rounded-full border border-white/12 p-2 transition hover:bg-white/10"
            title="返回浏览"
            @click="router.push(playerReturnPath())"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M15 18l-6-6 6-6" />
            </svg>
          </button>
          <h1 class="text-lg font-semibold">设置</h1>
        </div>
        <button class="rounded-full bg-white px-3 py-1.5 text-xs font-medium text-black" @click="router.push(playerReturnPath())">
          继续浏览
        </button>
      </header>

      <p v-if="hint" class="rounded-lg bg-white/[0.06] px-3 py-2 text-[11px] text-white/70">{{ hint }}</p>

      <!-- 账号 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <h2 class="mb-3 text-sm font-medium text-white/80">账号</h2>
        <div class="flex items-center justify-between text-sm">
          <div>
            <p class="font-medium">{{ auth.user?.username ?? '未登录' }}</p>
            <p class="mt-0.5 text-xs text-white/45">
              {{ auth.user?.role === 'admin' ? '管理员' : '普通用户' }} ·
              {{ auth.isAdmin ? '可进入后台管理' : '可浏览与收藏' }}
            </p>
            <p v-if="!auth.isAdmin" class="mt-1 text-[11px] text-white/30">
              管理内容需要管理员账号（用户名见服务端 auth.username）
            </p>
          </div>
          <div class="flex gap-2">
            <router-link
              v-if="auth.isAdmin"
              to="/admin"
              class="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/80 hover:bg-white/10"
            >
              管理后台
            </router-link>
            <span
              v-else
              class="rounded-lg border border-white/10 px-3 py-1.5 text-xs text-white/35"
              title="管理后台仅对管理员开放，请用管理员账号登录"
            >
              管理后台（无权限）
            </span>
            <button
              class="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/80 hover:bg-white/10"
              @click="signOut"
            >
              退出登录
            </button>
          </div>
        </div>
      </section>

      <!-- 播放设置 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-medium text-white/80">播放设置</h2>
          <button class="text-[11px] text-white/40 hover:text-white/70" @click="resetPreferences">恢复默认</button>
        </div>

        <div class="space-y-4 text-sm">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p>默认静音</p>
              <p class="text-xs text-white/40">关闭时进入即尝试有声播放（浏览器可能要求先点击一次）</p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition"
              :class="prefs.defaultMuted ? 'bg-white' : 'bg-white/15'"
              @click="prefs.defaultMuted = !prefs.defaultMuted"
            >
              <span
                class="absolute top-0.5 h-5 w-5 rounded-full bg-black transition"
                :class="prefs.defaultMuted ? 'left-[1.375rem]' : 'left-0.5 bg-white/70'"
              ></span>
            </button>
          </div>

          <div>
            <p class="mb-2">默认倍速</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="r in RATE_OPTIONS"
                :key="r.value"
                class="rounded-lg border px-2.5 py-1.5 text-xs transition"
                :class="
                  prefs.defaultRate === r.value
                    ? 'border-white bg-white text-black'
                    : 'border-white/15 text-white/70 hover:bg-white/10'
                "
                @click="setRate(r.value)"
              >
                {{ r.label }}
              </button>
            </div>
          </div>

          <div class="flex items-center justify-between gap-4">
            <div>
              <p>从收藏/历史播放时自动全屏</p>
              <p class="text-xs text-white/40">需要浏览器允许全屏，桌面端与手机端均生效</p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition"
              :class="prefs.autoFullscreen ? 'bg-white' : 'bg-white/15'"
              @click="prefs.autoFullscreen = !prefs.autoFullscreen"
            >
              <span
                class="absolute top-0.5 h-5 w-5 rounded-full bg-black transition"
                :class="prefs.autoFullscreen ? 'left-[1.375rem]' : 'left-0.5 bg-white/70'"
              ></span>
            </button>
          </div>

          <div class="flex items-center justify-between gap-4">
            <div>
              <p>全屏时自动横屏</p>
              <p class="text-xs text-white/40">仅在移动端支持屏幕方向锁定时生效</p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition"
              :class="prefs.autoRotate ? 'bg-white' : 'bg-white/15'"
              @click="prefs.autoRotate = !prefs.autoRotate"
            >
              <span
                class="absolute top-0.5 h-5 w-5 rounded-full bg-black transition"
                :class="prefs.autoRotate ? 'left-[1.375rem]' : 'left-0.5 bg-white/70'"
              ></span>
            </button>
          </div>

          <!-- 需求 3：播放结束行为 -->
          <div>
            <p class="mb-2">播放结束后</p>
            <div class="grid grid-cols-3 gap-1.5">
              <button
                v-for="opt in END_ACTION_OPTIONS"
                :key="opt.value"
                class="rounded-lg border px-2 py-2 text-center text-xs transition"
                :class="
                  prefs.endAction === opt.value
                    ? 'border-white bg-white text-black'
                    : 'border-white/15 text-white/70 hover:bg-white/10'
                "
                :title="opt.desc"
                @click="setEndAction(opt.value)"
              >
                {{ opt.label }}
              </button>
            </div>
            <p class="mt-1.5 text-[11px] text-white/35">
              {{ END_ACTION_OPTIONS.find((o) => o.value === prefs.endAction)?.desc }}
            </p>
          </div>

          <!-- 需求 2：随机刷到看过视频的策略 -->
          <div class="flex items-center justify-between gap-4">
            <div>
              <p>随机刷到看过的视频从头播放</p>
              <p class="text-xs text-white/40">关闭则从上次进度继续；从收藏/历史打开始终续播并提示「从头开始」</p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition"
              :class="prefs.randomRestartWatched ? 'bg-white' : 'bg-white/15'"
              @click="prefs.randomRestartWatched = !prefs.randomRestartWatched"
            >
              <span
                class="absolute top-0.5 h-5 w-5 rounded-full bg-black transition"
                :class="prefs.randomRestartWatched ? 'left-[1.375rem]' : 'left-0.5 bg-white/70'"
              ></span>
            </button>
          </div>
        </div>
      </section>

      <!-- 观看统计（需求 5） -->
      <section v-if="auth.isLoggedIn" class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-medium text-white/80">我的观看统计</h2>
          <button class="text-[11px] text-white/40 hover:text-white/70" @click="loadMeta">刷新</button>
        </div>
        <div class="grid grid-cols-3 gap-2 text-center">
          <div class="rounded-xl border border-white/10 bg-black/25 px-2 py-3">
            <p class="text-xl font-semibold text-white">{{ watchSummary?.watched_videos ?? '—' }}</p>
            <p class="mt-1 text-[11px] text-white/45">观看视频</p>
          </div>
          <div class="rounded-xl border border-white/10 bg-black/25 px-2 py-3">
            <p class="text-xl font-semibold text-emerald-300">{{ watchSummary?.completed_videos ?? '—' }}</p>
            <p class="mt-1 text-[11px] text-white/45">完整播完</p>
          </div>
          <div class="rounded-xl border border-white/10 bg-black/25 px-2 py-3">
            <p class="text-xl font-semibold text-sky-300">
              {{ watchSummary ? formatDuration(watchSummary.total_watch_time) : '—' }}
            </p>
            <p class="mt-1 text-[11px] text-white/45">观看时长</p>
          </div>
        </div>
        <p class="mt-2 text-[11px] text-white/30">观看时长按实际看过的部分累计；看完的定义为播放至 95% 以上</p>
      </section>

      <!-- 观看历史入口 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-medium text-white/80">观看历史</h2>
            <p class="mt-0.5 text-[11px] text-white/35">按账号独立保存，可随时续播，与收藏夹分开管理</p>
          </div>
          <router-link to="/history" class="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/80 hover:bg-white/10">
            查看历史
          </router-link>
        </div>
      </section>

      <!-- 运行环境 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <h2 class="mb-3 text-sm font-medium text-white/80">运行环境</h2>
        <dl class="grid grid-cols-2 gap-3 text-xs text-white/55">
          <div>
            <dt class="text-white/35">服务端版本</dt>
            <dd class="mt-0.5 text-white/80">{{ config?.version ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-white/35">时区</dt>
            <dd class="mt-0.5 text-white/80">{{ config?.timezone ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-white/35">图片停留</dt>
            <dd class="mt-0.5 text-white/80">{{ config?.image_display_seconds ?? '—' }} 秒</dd>
          </div>
          <div>
            <dt class="text-white/35">游客访问</dt>
            <dd class="mt-0.5 text-white/80">{{ config?.guest_access ? '已开启' : '未开启' }}</dd>
          </div>
          <div>
            <dt class="text-white/35">本地收藏缓存</dt>
            <dd class="mt-0.5 text-white/80">{{ favoriteCacheCount }} 条</dd>
          </div>
          <div>
            <dt class="text-white/35">日志上报</dt>
            <dd class="mt-0.5 text-white/80">
              {{ config?.frontend_log_upload ? '已开启（需管理员配置）' : '未开启（仅本地保存）' }}
            </dd>
          </div>
        </dl>
        <div class="mt-3 flex flex-wrap gap-2">
          <button class="rounded-lg border border-white/15 px-3 py-1.5 text-xs hover:bg-white/10" @click="clearCache">
            清空本地缓存
          </button>
          <button class="rounded-lg border border-white/15 px-3 py-1.5 text-xs hover:bg-white/10" @click="loadMeta">
            刷新信息
          </button>
        </div>
      </section>

      <!-- 本地运行日志：仅管理员可见 -->
      <section v-if="auth.isAdmin" class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-sm font-medium text-white/80">本地运行日志</h2>
            <p class="mt-0.5 text-[11px] text-white/35">仅管理员可见；全站日志请到后台「运行日志」查看</p>
          </div>
          <div class="flex flex-wrap gap-1.5 text-xs">
            <button class="rounded-lg border border-white/15 px-2.5 py-1.5 hover:bg-white/10" @click="loadLogs">
              刷新
            </button>
            <button class="rounded-lg border border-white/15 px-2.5 py-1.5 hover:bg-white/10" @click="exportJSON">
              导出 JSON
            </button>
            <button class="rounded-lg border border-white/15 px-2.5 py-1.5 hover:bg-white/10" @click="exportText">
              导出文本
            </button>
            <button class="rounded-lg border border-red-400/30 px-2.5 py-1.5 text-red-300 hover:bg-red-500/10" @click="clearLogs">
              清空
            </button>
          </div>
        </div>

        <div class="mb-3 grid grid-cols-1 gap-2 sm:grid-cols-3">
          <select
            v-model="category"
            class="rounded-lg border border-white/12 bg-[#111114] px-3 py-2 text-xs text-white/85 outline-none"
            @change="loadLogs"
          >
            <option v-for="c in categories" :key="c.value" :value="c.value">{{ c.label }}</option>
          </select>
          <select
            v-model="level"
            class="rounded-lg border border-white/12 bg-[#111114] px-3 py-2 text-xs text-white/85 outline-none"
            @change="loadLogs"
          >
            <option value="">全部级别</option>
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
          <input
            v-model="keyword"
            placeholder="关键字搜索，回车确认"
            class="rounded-lg border border-white/12 bg-white/[0.04] px-3 py-2 text-xs outline-none focus:border-white/30"
            @keyup.enter="loadLogs"
          />
        </div>

        <p class="mb-2 text-[11px] text-white/40">{{ loading ? '加载中…' : filteredHint }}</p>

        <ul class="max-h-[320px] space-y-1.5 overflow-y-auto pr-1">
          <li
            v-for="(log, idx) in logs"
            :key="log.id ?? idx"
            class="rounded-lg border border-white/8 bg-black/30 px-3 py-2 text-[11px]"
          >
            <div class="flex items-center gap-2">
              <span class="rounded px-1.5 py-0.5 font-mono text-[10px]" :class="levelTag[log.level] ?? 'bg-white/10'">
                {{ log.level }}
              </span>
              <span class="font-mono text-white/45">{{ log.category }}</span>
              <span class="text-white/30">{{ formatTime(log.ts) }}</span>
            </div>
            <p class="mt-1 text-white/85">{{ log.message }}</p>
            <p v-if="log.context && Object.keys(log.context).length" class="mt-1 break-all font-mono text-[10px] text-white/40">
              {{ JSON.stringify(log.context) }}
            </p>
          </li>
        </ul>
        <p class="mt-2 text-[11px] text-white/25">日志仅保存在本机浏览器，导出前已对密码、Token 等敏感字段脱敏</p>
      </section>

      <!-- 关于 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <h2 class="mb-3 text-sm font-medium text-white/80">关于 LanReel</h2>
        <p class="text-xs leading-relaxed text-white/55">
          LanReel 是一个局域网随机视频流系统：把主机上的视频与图片文件夹变成抖音式竖屏信息流，
          支持收藏同步、游客浏览、按文件夹/文件设置可见范围与播放时段，并提供运行日志与审计后台。
          单二进制部署，浏览器直接访问，不做转码、不占资源。
        </p>
        <ul class="mt-3 space-y-1 text-[11px] text-white/40">
          <li>· 全屏竖滑播放，进度按账号云端同步，可随时续播</li>
          <li>· 收藏跨设备同步，收藏夹/历史以悬停抽帧网格浏览</li>
          <li>· 游客可直接浏览，管理员按游客/普通用户分别设置可见范围与播放时段</li>
          <li>· 观看统计与热度分析、文件监听、登录限流、分类日志</li>
        </ul>
        <div class="mt-3 flex flex-wrap items-center gap-3 text-[11px]">
          <span class="text-white/35">版本 v{{ config?.version ?? '—' }}</span>
          <a
            href="https://github.com/zys747526271/lanreel"
            target="_blank"
            rel="noreferrer"
            class="text-sky-300/80 hover:text-sky-200"
          >
            GitHub 仓库
          </a>
          <span class="text-white/25">GPL-3.0</span>
        </div>
      </section>

      <p class="pb-2 text-center text-[11px] text-white/20">LanReel · 局域网私有媒体浏览</p>
    </div>
  </div>
</template>
