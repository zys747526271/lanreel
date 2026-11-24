<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useFavoritesStore } from '@/stores/favorites'
import { logger, type LogRecord } from '@/lib/logger'
import { countAll, clearStore } from '@/lib/db'
import { fetchPublicConfig } from '@/api/client'
import { formatTime } from '@/lib/format'
import type { PublicConfig } from '@/api/types'

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
            @click="router.push('/')"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M15 18l-6-6 6-6" />
            </svg>
          </button>
          <h1 class="text-lg font-semibold">设置与诊断</h1>
        </div>
        <button class="rounded-full bg-white px-3 py-1.5 text-xs font-medium text-black" @click="router.push('/')">
          继续浏览
        </button>
      </header>

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
          </div>
          <div class="flex gap-2">
            <router-link
              v-if="auth.isAdmin"
              to="/admin"
              class="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/80 hover:bg-white/10"
            >
              管理后台
            </router-link>
            <button
              class="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/80 hover:bg-white/10"
              @click="signOut"
            >
              退出登录
            </button>
          </div>
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
            <dt class="text-white/35">日志上报</dt>
            <dd class="mt-0.5 text-white/80">
              {{ config?.frontend_log_upload ? '已开启（需管理员配置）' : '未开启（仅本地保存）' }}
            </dd>
          </div>
          <div>
            <dt class="text-white/35">本地收藏缓存</dt>
            <dd class="mt-0.5 text-white/80">{{ favoriteCacheCount }} 条</dd>
          </div>
          <div>
            <dt class="text-white/35">本地日志</dt>
            <dd class="mt-0.5 text-white/80">{{ logCount }} 条 / 上限 5000</dd>
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

      <!-- 前端日志 -->
      <section class="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-sm font-medium text-white/80">本地运行日志</h2>
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
        <p v-if="hint" class="mb-2 rounded-lg bg-white/[0.06] px-3 py-2 text-[11px] text-white/70">{{ hint }}</p>

        <ul class="max-h-[380px] space-y-1.5 overflow-y-auto pr-1">
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
      </section>

      <p class="pb-4 text-center text-[11px] text-white/25">
        日志仅保存在本机浏览器（IndexedDB），导出前已对密码、Token 等敏感字段脱敏
      </p>
    </div>
  </div>
</template>
