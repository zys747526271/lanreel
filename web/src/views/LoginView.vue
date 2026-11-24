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
  <div class="flex min-h-[100dvh] items-center justify-center bg-[#08080a] px-5 py-10 text-white">
    <div class="w-full max-w-sm">
      <div class="mb-8 text-center">
        <h1 class="text-3xl font-semibold tracking-tight">LanReel</h1>
        <p class="mt-2 text-sm text-white/45">局域网随机视频流</p>
      </div>

      <form class="space-y-3" @submit.prevent="submit">
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">用户名</span>
          <input
            v-model="username"
            class="w-full rounded-xl border border-white/12 bg-white/[0.04] px-4 py-3 text-sm outline-none transition focus:border-white/35 focus:bg-white/[0.07]"
            autocomplete="username"
            placeholder="请输入用户名"
          />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">密码</span>
          <input
            v-model="password"
            type="password"
            class="w-full rounded-xl border border-white/12 bg-white/[0.04] px-4 py-3 text-sm outline-none transition focus:border-white/35 focus:bg-white/[0.07]"
            autocomplete="current-password"
            placeholder="请输入密码"
          />
        </label>

        <p v-if="error" class="rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-300">{{ error }}</p>

        <button
          type="submit"
          class="mt-2 flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-sm font-medium text-black transition hover:bg-white/90 disabled:opacity-60"
          :disabled="submitting"
        >
          <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-black/25 border-t-black"></span>
          {{ submitting ? '登录中…' : '登录' }}
        </button>
      </form>

      <div class="mt-5 flex items-center justify-between text-xs text-white/45">
        <span>v{{ version || '—' }}</span>
        <router-link v-if="registrationOpen" to="/register" class="text-white/70 hover:text-white">
          没有账号？注册
        </router-link>
        <span v-else>注册入口已关闭</span>
      </div>
    </div>
  </div>
</template>
