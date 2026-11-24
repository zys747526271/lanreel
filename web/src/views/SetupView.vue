<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ApiError, fetchPublicConfig } from '@/api/client'

const router = useRouter()
const auth = useAuthStore()

const usernameEl = ref<HTMLInputElement | null>(null)
const username = ref('')
const password = ref('')
const confirm = ref('')
const error = ref('')
const submitting = ref(false)
const version = ref('')

onMounted(async () => {
  usernameEl.value?.focus()
  try {
    version.value = (await fetchPublicConfig()).version
  } catch {
    /* 版本号仅作展示，失败忽略 */
  }
})

const passwordHint = computed(() => {
  const v = password.value
  if (!v) return '至少 6 位'
  if (v.length < 6) return `还差 ${6 - v.length} 位`
  if (v.length < 10) return '强度一般，建议加入大小写与符号'
  return '强度良好'
})

const canSubmit = computed(
  () => username.value.trim().length >= 3 && password.value.length >= 6 && confirm.value === password.value,
)

async function submit() {
  error.value = ''
  const name = username.value.trim()
  if (name.length < 3 || name.length > 32) {
    error.value = '用户名长度需为 3-32 个字符'
    return
  }
  if (!/^[A-Za-z0-9_.-]+$/.test(name)) {
    error.value = '用户名只能包含字母、数字、下划线、短横线、点'
    return
  }
  if (password.value.length < 6) {
    error.value = '密码长度至少 6 位'
    return
  }
  if (password.value !== confirm.value) {
    error.value = '两次输入的密码不一致'
    return
  }
  submitting.value = true
  try {
    await auth.completeSetup(name, password.value)
    router.replace('/')
  } catch (err) {
    // 409：实例已被其他窗口/设备完成初始化，刷新状态后转到登录页
    if (err instanceof ApiError && err.status === 409) {
      await auth.loadSetupStatus(true)
      router.replace('/login')
      return
    }
    error.value = err instanceof Error ? err.message : '创建管理员失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-[100dvh] items-center justify-center bg-[#08080a] px-5 py-10 text-white">
    <div class="w-full max-w-sm">
      <div class="mb-7 text-center">
        <span class="inline-flex items-center gap-1.5 rounded-full border border-emerald-400/25 bg-emerald-400/10 px-2.5 py-1 text-[11px] text-emerald-300">
          <span class="h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
          首次部署
        </span>
        <h1 class="mt-4 text-3xl font-semibold tracking-tight">创建管理员</h1>
        <p class="mt-2 text-sm leading-relaxed text-white/45">
          这是本实例第一次启动。创建一个管理员账号后即可开始使用，
          <br class="hidden sm:block" />
          该账号拥有全部管理权限，之后可在后台添加普通用户。
        </p>
      </div>

      <form class="space-y-3" @submit.prevent="submit">
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">管理员用户名</span>
          <input
            ref="usernameEl"
            v-model="username"
            class="w-full rounded-xl border border-white/12 bg-white/[0.04] px-4 py-3 text-sm outline-none transition focus:border-white/35 focus:bg-white/[0.07]"
            autocomplete="username"
            spellcheck="false"
            placeholder="例如 admin"
          />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">密码</span>
          <input
            v-model="password"
            type="password"
            class="w-full rounded-xl border border-white/12 bg-white/[0.04] px-4 py-3 text-sm outline-none transition focus:border-white/35 focus:bg-white/[0.07]"
            autocomplete="new-password"
            placeholder="至少 6 位"
          />
          <span class="mt-1 block text-[11px] text-white/35">{{ passwordHint }}</span>
        </label>
        <label class="block">
          <span class="mb-1.5 block text-xs text-white/55">确认密码</span>
          <input
            v-model="confirm"
            type="password"
            class="w-full rounded-xl border border-white/12 bg-white/[0.04] px-4 py-3 text-sm outline-none transition focus:border-white/35 focus:bg-white/[0.07]"
            autocomplete="new-password"
            placeholder="再输入一次密码"
          />
          <span
            v-if="confirm && confirm !== password"
            class="mt-1 block text-[11px] text-amber-300/80"
          >
            两次输入的密码不一致
          </span>
        </label>

        <p v-if="error" class="rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-300">{{ error }}</p>

        <button
          type="submit"
          class="mt-2 flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-sm font-medium text-black transition enabled:hover:bg-white/90 disabled:bg-white/10 disabled:text-white/35"
          :disabled="submitting || !canSubmit"
        >
          <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-black/25 border-t-black"></span>
          {{ submitting ? '创建中…' : '创建并进入' }}
        </button>
      </form>

      <p class="mt-5 text-center text-[11px] leading-relaxed text-white/30">
        初始化仅可执行一次，完成后再次启动会直接进入主页。
        <br />
        如需重置管理员密码，可在服务器上使用
        <code class="rounded bg-white/5 px-1 py-0.5 text-white/45">--reset-admin-password</code>
      </p>

      <div class="mt-6 flex items-center justify-between text-xs text-white/30">
        <span>LanReel v{{ version || '—' }}</span>
        <a href="/api/v1/health" class="hover:text-white/60">服务状态</a>
      </div>
    </div>
  </div>
</template>
