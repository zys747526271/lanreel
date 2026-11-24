<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { fetchPublicConfig } from '@/api/client'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const error = ref('')
const submitting = ref(false)
const version = ref('')
const registrationOpen = ref(true)

onMounted(async () => {
  try {
    const cfg = await fetchPublicConfig()
    version.value = cfg.version
    registrationOpen.value = cfg.registration_open
    auth.setRegistrationOpen(cfg.registration_open)
  } catch {
    /* 后端不可达时提示由登录请求给出 */
  }
})

async function submit() {
  error.value = ''
  if (!username.value.trim() || !password.value) {
    error.value = '请输入用户名与密码'
    return
  }
  submitting.value = true
  try {
    await auth.login(username.value.trim(), password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '登录失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="relative flex min-h-[100dvh] items-center justify-center overflow-hidden bg-[#08080a] px-5 py-10 text-white">
    <!-- 柔和光晕背景 -->
    <div class="pointer-events-none absolute -top-32 left-1/2 h-[420px] w-[420px] -translate-x-1/2 rounded-full bg-reel-accent/20 blur-[120px]"></div>
    <div class="pointer-events-none absolute -bottom-40 -right-20 h-[360px] w-[360px] rounded-full bg-sky-500/10 blur-[120px]"></div>

    <div class="relative w-full max-w-sm">
      <div class="mb-8 text-center">
        <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-reel-accent/90 to-reel-accent/40 shadow-lg shadow-reel-accent/20">
          <svg class="h-7 w-7 text-white" viewBox="0 0 24 24" fill="currentColor">
            <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3h11A2.5 2.5 0 0 1 20 5.5v13a2.5 2.5 0 0 1-2.5 2.5h-11A2.5 2.5 0 0 1 4 18.5v-13Zm5.2 2.9v7.2l6.1-3.6-6.1-3.6Z" />
          </svg>
        </div>
        <h1 class="text-3xl font-semibold tracking-tight">LanReel</h1>
        <p class="mt-2 text-sm text-white/45">你的私人随机视频流</p>
      </div>

      <form class="space-y-3 rounded-2xl border border-white/10 bg-white/[0.04] p-5 shadow-2xl backdrop-blur-xl" @submit.prevent="submit">
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">用户名</span>
          <div class="relative">
            <svg class="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-white/35" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <circle cx="12" cy="8" r="4" /><path d="M4 20c0-3.5 3.6-6 8-6s8 2.5 8 6" />
            </svg>
            <input
              v-model="username"
              class="w-full rounded-xl border border-white/12 bg-black/25 py-3 pl-10 pr-4 text-sm outline-none transition placeholder:text-white/25 focus:border-reel-accent/60 focus:bg-black/35"
              autocomplete="username"
              placeholder="请输入用户名"
            />
          </div>
        </label>
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">密码</span>
          <div class="relative">
            <svg class="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-white/35" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <rect x="4" y="10" width="16" height="10" rx="2.5" /><path d="M8 10V7a4 4 0 0 1 8 0v3" />
            </svg>
            <input
              v-model="password"
              type="password"
              class="w-full rounded-xl border border-white/12 bg-black/25 py-3 pl-10 pr-4 text-sm outline-none transition placeholder:text-white/25 focus:border-reel-accent/60 focus:bg-black/35"
              autocomplete="current-password"
              placeholder="请输入密码"
            />
          </div>
        </label>

        <p v-if="error" class="rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-300">{{ error }}</p>

        <button
          type="submit"
          class="mt-2 flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-sm font-medium text-black transition hover:bg-white/90 active:scale-[0.99] disabled:opacity-60"
          :disabled="submitting"
        >
          <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-black/25 border-t-black"></span>
          {{ submitting ? '登录中…' : '登录' }}
        </button>
      </form>

      <div class="mt-5 flex items-center justify-between px-1 text-xs text-white/45">
        <span>v{{ version || '—' }}</span>
        <router-link v-if="registrationOpen" to="/register" class="text-reel-accent/90 transition hover:text-reel-accent">
          没有账号？注册 →
        </router-link>
        <span v-else>注册入口已关闭</span>
      </div>
    </div>
  </div>
</template>
