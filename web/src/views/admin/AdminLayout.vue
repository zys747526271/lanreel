<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const menus = [
  { path: '/admin/dashboard', label: '概览', icon: 'M4 13h7V4H4v9Zm0 7h7v-5H4v5Zm9 0h7V11h-7v9Zm0-16v5h7V4h-7Z' },
  { path: '/admin/folders', label: '文件夹策略', icon: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z' },
  { path: '/admin/files', label: '文件策略', icon: 'M6 3h8l4 4v14H6V3Zm8 0v4h4' },
  { path: '/admin/users', label: '用户管理', icon: 'M16 20v-1.5A3.5 3.5 0 0 0 12.5 15h-5A3.5 3.5 0 0 0 4 18.5V20M10 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Zm7 9v-1.6a3.4 3.4 0 0 0-2-3.1M16 4.2a3.5 3.5 0 0 1 0 6.6' },
  { path: '/admin/logs', label: '运行日志', icon: 'M4 5h16M4 10h16M4 15h10M4 20h7' },
  { path: '/admin/audit', label: '审计日志', icon: 'M12 3l8 4v6c0 5-3.4 7.4-8 8-4.6-.6-8-3-8-8V7l8-4Z' },
  { path: '/admin/settings', label: '系统设置', icon: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm8-3a8 8 0 0 1-.2 1.7l2 1.5-2 3.4-2.3-1a8 8 0 0 1-2.9 1.7L14 22h-4l-.6-2.7a8 8 0 0 1-2.9-1.7l-2.3 1-2-3.4 2-1.5A8 8 0 0 1 4 12a8 8 0 0 1 .2-1.7l-2-1.5 2-3.4 2.3 1a8 8 0 0 1 2.9-1.7L10 2h4l.6 2.7a8 8 0 0 1 2.9 1.7l2.3-1 2 3.4-2 1.5c.1.5.2 1.1.2 1.7Z' },
]

const currentTitle = computed(() => (route.meta.title as string) || '管理后台')

async function signOut() {
  await auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="admin-scope min-h-[100dvh] bg-slate-100 text-slate-800">
    <div class="mx-auto flex min-h-[100dvh] w-full max-w-[1400px]">
      <!-- 侧边栏 -->
      <aside class="hidden w-60 shrink-0 border-r border-slate-200 bg-white px-3 py-5 md:block">
        <div class="mb-6 px-2">
          <p class="text-lg font-semibold tracking-tight">LanReel</p>
          <p class="text-xs text-slate-400">管理后台</p>
        </div>
        <nav class="space-y-1">
          <router-link
            v-for="m in menus"
            :key="m.path"
            :to="m.path"
            class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition"
            :class="
              route.path === m.path
                ? 'bg-slate-900 text-white'
                : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
            "
          >
            <svg class="h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7">
              <path :d="m.icon" />
            </svg>
            {{ m.label }}
          </router-link>
        </nav>
        <div class="mt-8 rounded-xl bg-slate-50 p-3 text-xs text-slate-500">
          <p class="font-medium text-slate-700">{{ auth.user?.username }}</p>
          <p class="mt-0.5">管理员</p>
          <button class="mt-3 text-slate-600 underline-offset-2 hover:underline" @click="signOut">退出登录</button>
        </div>
      </aside>

      <!-- 内容区 -->
      <main class="min-w-0 flex-1">
        <header class="flex items-center justify-between border-b border-slate-200 bg-white/80 px-4 py-3 backdrop-blur md:px-6">
          <div class="flex items-center gap-3">
            <router-link to="/" class="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs text-slate-600 hover:bg-slate-100">
              ← 返回前台
            </router-link>
            <h1 class="text-base font-semibold">{{ currentTitle }}</h1>
          </div>
          <div class="flex items-center gap-2 text-xs text-slate-500">
            <span class="hidden sm:inline">{{ auth.user?.username }}</span>
            <router-link to="/settings" class="rounded-lg border border-slate-200 px-2.5 py-1.5 hover:bg-slate-100">
              诊断
            </router-link>
          </div>
        </header>

        <!-- 移动端菜单 -->
        <div class="flex gap-1 overflow-x-auto border-b border-slate-200 bg-white px-3 py-2 md:hidden">
          <router-link
            v-for="m in menus"
            :key="m.path"
            :to="m.path"
            class="whitespace-nowrap rounded-full px-3 py-1.5 text-xs"
            :class="route.path === m.path ? 'bg-slate-900 text-white' : 'bg-slate-100 text-slate-600'"
          >
            {{ m.label }}
          </router-link>
        </div>

        <div class="px-4 py-5 md:px-6">
          <router-view />
        </div>
      </main>
    </div>
  </div>
</template>
