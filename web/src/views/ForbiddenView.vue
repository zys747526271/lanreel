<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useFavoritesStore } from '@/stores/favorites'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const favorites = useFavoritesStore()

const attempted = computed(() => (route.query.from as string) || '/admin')

async function switchAccount() {
  await auth.logout()
  favorites.reset()
  router.push({ path: '/login', query: { redirect: attempted.value } })
}
</script>

<template>
  <div class="flex min-h-[100dvh] flex-col items-center justify-center gap-4 bg-[#08080a] px-6 text-center text-white">
    <svg class="h-12 w-12 text-amber-300/80" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
      <rect x="4" y="10.5" width="16" height="10" rx="2.5" />
      <path d="M8 10.5V8a4 4 0 0 1 8 0v2.5" />
    </svg>
    <h1 class="text-lg font-semibold">需要管理员权限</h1>
    <p class="max-w-sm text-sm leading-relaxed text-white/55">
      当前账号
      <span class="text-white/85">{{ auth.user?.username ?? '未登录' }}</span>
      （{{ auth.user?.role === 'admin' ? '管理员' : '普通用户' }}）无权访问
      <span class="font-mono text-xs text-white/70">{{ attempted }}</span>。
      请使用管理员账号登录，或联系管理员为你提升角色。
    </p>
    <div class="mt-2 flex gap-2">
      <button class="rounded-full bg-white px-4 py-2 text-sm font-medium text-black" @click="switchAccount">
        用管理员账号登录
      </button>
      <router-link to="/" class="rounded-full border border-white/20 px-4 py-2 text-sm text-white/80 hover:bg-white/10">
        返回浏览
      </router-link>
    </div>
    <p class="mt-4 max-w-sm text-[11px] leading-relaxed text-white/30">
      管理员账号由服务端启动配置创建（用户名见 auth.username，初始密码见环境变量
      ADMIN_INIT_PASSWORD）。若忘记密码，可用 --reset-admin-password 参数重启一次。
    </p>
  </div>
</template>
