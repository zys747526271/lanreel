<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { fetchPublicConfig } from '@/api/client'

const router = useRouter()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const confirm = ref('')
const error = ref('')
const submitting = ref(false)
const serverRegistrationOpen = ref(true)
const loaded = ref(false)

const canSubmit = computed(
  () => loaded.value && serverRegistrationOpen.value && !submitting.value,
)

onMounted(async () => {
  try {
    const cfg = await fetchPublicConfig()
    serverRegistrationOpen.value = cfg.registration_open
  } catch {
    error.value = '无法连接服务端，请稍后重试'
  } finally {
    loaded.value = true
  }
})

async function submit() {
  error.value = ''
  if (!serverRegistrationOpen.value) {
    error.value = '管理员已关闭注册入口'
    return
  }
  if (username.value.trim().length < 3) {
    error.value = '用户名至少 3 个字符'
    return
  }
  if (password.value.length < 6) {
    error.value = '密码至少 6 个字符'
    return
  }
  if (password.value !== confirm.value) {
    error.value = '两次输入的密码不一致'
    return
  }
  submitting.value = true
  try {
    await auth.register(username.value.trim(), password.value)
    router.replace('/')
  } catch (err) {
    error.value = err instanceof Error ? err.message : '注册失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="relative flex min-h-[100dvh] items-center justify-center overflow-hidden bg-[#08080a] px-5 py-10 text-white">
    <div class="pointer-events-none absolute -top-32 right-1/2 h-[420px] w-[420px] translate-x-1/2 rounded-full bg-emerald-500/15 blur-[120px]"></div>
    <div class="pointer-events-none absolute -bottom-40 left-1/4 h-[360px] w-[360px] rounded-full bg-sky-500/10 blur-[120px]"></div>

    <div class="relative w-full max-w-sm">
      <div class="mb-8 text-center">
        <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-400/90 to-emerald-600/40 shadow-lg shadow-emerald-500/20">
          <svg class="h-7 w-7 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <circle cx="9" cy="8" r="3.5" /><path d="M3 20c0-3.3 2.7-5.5 6-5.5s6 2.2 6 5.5" /><path d="M18 8v6m3-3h-6" />
          </svg>
        </div>
        <h1 class="text-3xl font-semibold tracking-tight">创建账号</h1>
        <p class="mt-2 text-sm text-white/45">注册后即可浏览、收藏并云端续播</p>
      </div>

      <form class="space-y-3 rounded-2xl border border-white/10 bg-white/[0.04] p-5 shadow-2xl backdrop-blur-xl" @submit.prevent="submit">
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">用户名</span>
          <input
            v-model="username"
            class="w-full rounded-xl border border-white/12 bg-black/25 px-4 py-3 text-sm outline-none transition placeholder:text-white/25 focus:border-emerald-400/60 focus:bg-black/35"
            autocomplete="username"
            placeholder="3-32 位字母、数字、下划线"
          />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">密码</span>
          <input
            v-model="password"
            type="password"
            class="w-full rounded-xl border border-white/12 bg-black/25 px-4 py-3 text-sm outline-none transition placeholder:text-white/25 focus:border-emerald-400/60 focus:bg-black/35"
            autocomplete="new-password"
            placeholder="至少 6 位"
          />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">确认密码</span>
          <input
            v-model="confirm"
            type="password"
            class="w-full rounded-xl border border-white/12 bg-black/25 px-4 py-3 text-sm outline-none transition placeholder:text-white/25 focus:border-emerald-400/60 focus:bg-black/35"
            autocomplete="new-password"
            placeholder="再次输入密码"
          />
        </label>

        <p v-if="error" class="rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-300">{{ error }}</p>
        <p
          v-else-if="loaded && !serverRegistrationOpen"
          class="rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-300"
        >
          管理员已关闭注册入口，请联系管理员开通账号
        </p>

        <button
          type="submit"
          class="mt-2 flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-sm font-medium text-black transition hover:bg-white/90 active:scale-[0.99] disabled:opacity-60"
          :disabled="!canSubmit"
        >
          <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-black/25 border-t-black"></span>
          {{ submitting ? '注册中…' : '注册并登录' }}
        </button>
      </form>

      <div class="mt-5 text-center text-xs text-white/45">
        已有账号？
        <router-link to="/login" class="text-emerald-300/90 transition hover:text-emerald-300">返回登录 →</router-link>
      </div>
    </div>
  </div>
</template>
