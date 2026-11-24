<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NButton,
  NCard,
  NDrawer,
  NDrawerContent,
  NInput,
  NModal,
  NSpin,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  cleanUserFavorites,
  deleteUser,
  listUserFavorites,
  listUsers,
  updateUser,
} from '@/api/admin'
import { useAuthStore } from '@/stores/auth'
import { formatTime } from '@/lib/format'
import type { MediaItem, UserView } from '@/api/types'

const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()

const users = ref<UserView[]>([])
const loading = ref(true)
const keyword = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const passwordModal = ref(false)
const passwordTarget = ref<UserView | null>(null)
const newPassword = ref('')
const savingPassword = ref(false)

const drawerOpen = ref(false)
const drawerUser = ref<UserView | null>(null)
const drawerItems = ref<MediaItem[]>([])
const drawerLoading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await listUsers({ page: page.value, page_size: pageSize, keyword: keyword.value || undefined })
    users.value = res.data
    total.value = res.meta.total
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  void load()
}

function changePage(next: number) {
  if (next < 1 || next > totalPages.value) return
  page.value = next
  void load()
}

async function toggleEnabled(user: UserView, enabled: boolean) {
  try {
    await updateUser(user.id, { enabled })
    user.enabled = enabled
    message.success(enabled ? '账号已启用' : '账号已禁用')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
    await load()
  }
}

function changeRole(user: UserView, role: 'admin' | 'user') {
  if (user.role === role) return
  dialog.warning({
    title: '调整角色',
    content: `确认将「${user.username}」的角色调整为${role === 'admin' ? '管理员' : '普通用户'}？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await updateUser(user.id, { role })
        user.role = role
        message.success('角色已更新')
      } catch (err) {
        message.error(err instanceof Error ? err.message : '修改失败')
        await load()
      }
    },
  })
}

function openPassword(user: UserView) {
  passwordTarget.value = user
  newPassword.value = ''
  passwordModal.value = true
}

async function savePassword() {
  if (!passwordTarget.value) return
  if (newPassword.value.length < 6) {
    message.warning('密码至少 6 位')
    return
  }
  savingPassword.value = true
  try {
    await updateUser(passwordTarget.value.id, { password: newPassword.value })
    message.success('密码已重置')
    passwordModal.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '重置失败')
  } finally {
    savingPassword.value = false
  }
}

function removeUser(user: UserView) {
  dialog.error({
    title: '删除账号',
    content: `确认删除「${user.username}」？其收藏数据会一并删除，操作不可恢复。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteUser(user.id)
        message.success('账号已删除')
        await load()
      } catch (err) {
        message.error(err instanceof Error ? err.message : '删除失败')
      }
    },
  })
}

async function openFavorites(user: UserView) {
  drawerUser.value = user
  drawerOpen.value = true
  drawerLoading.value = true
  try {
    drawerItems.value = (await listUserFavorites(user.id)).data
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载收藏失败')
  } finally {
    drawerLoading.value = false
  }
}

async function cleanInvalid() {
  if (!drawerUser.value) return
  try {
    const res = await cleanUserFavorites(drawerUser.value.id)
    message.success(`已清理 ${res.data.removed} 条失效收藏`)
    await openFavorites(drawerUser.value)
    await load()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '清理失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <n-card size="small">
      <div class="flex flex-wrap items-end gap-2">
        <div class="min-w-[220px] flex-1">
          <p class="mb-1 text-xs text-slate-500">按用户名搜索</p>
          <n-input v-model:value="keyword" placeholder="输入用户名" clearable @keyup.enter="search" />
        </div>
        <n-button type="primary" @click="search">查询</n-button>
        <n-button @click="load">刷新</n-button>
      </div>
      <p class="mt-2 text-xs text-slate-400">
        管理员账号由启动配置初始化，不通过注册产生；删除账号会级联删除其收藏。
      </p>
    </n-card>

    <n-spin :show="loading">
      <div class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <table class="w-full min-w-[820px] text-sm">
          <thead class="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th class="px-4 py-2.5 text-left font-medium">用户</th>
              <th class="px-4 py-2.5 text-left font-medium">角色</th>
              <th class="px-4 py-2.5 text-left font-medium">状态</th>
              <th class="px-4 py-2.5 text-left font-medium">收藏</th>
              <th class="px-4 py-2.5 text-left font-medium">注册时间</th>
              <th class="px-4 py-2.5 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="u in users" :key="u.id" class="hover:bg-slate-50/70">
              <td class="px-4 py-2.5">
                <p class="font-medium text-slate-800">
                  {{ u.username }}
                  <span v-if="u.id === auth.user?.id" class="ml-1 text-[11px] text-slate-400">（当前登录）</span>
                </p>
                <p class="font-mono text-[10px] text-slate-400">#{{ u.id }}</p>
              </td>
              <td class="px-4 py-2.5">
                <select
                  :value="u.role"
                  class="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs"
                  @change="changeRole(u, ($event.target as HTMLSelectElement).value as 'admin' | 'user')"
                >
                  <option value="user">普通用户</option>
                  <option value="admin">管理员</option>
                </select>
              </td>
              <td class="px-4 py-2.5">
                <div class="flex items-center gap-2">
                  <n-switch
                    size="small"
                    :value="u.enabled"
                    :disabled="u.id === auth.user?.id"
                    @update:value="(v: boolean) => toggleEnabled(u, v)"
                  />
                  <n-tag size="small" :type="u.enabled ? 'success' : 'error'" :bordered="false">
                    {{ u.enabled ? '启用' : '禁用' }}
                  </n-tag>
                </div>
              </td>
              <td class="px-4 py-2.5">
                <button class="text-xs text-slate-700 underline-offset-2 hover:underline" @click="openFavorites(u)">
                  {{ u.favorite_count }} 条
                </button>
              </td>
              <td class="px-4 py-2.5 text-xs text-slate-500">{{ formatTime(u.created_at) }}</td>
              <td class="whitespace-nowrap px-4 py-2.5 text-right">
                <n-button size="tiny" quaternary @click="openPassword(u)">重置密码</n-button>
                <n-button size="tiny" quaternary type="error" :disabled="u.id === auth.user?.id" @click="removeUser(u)">
                  删除
                </n-button>
              </td>
            </tr>
            <tr v-if="!users.length && !loading">
              <td colspan="6" class="px-4 py-10 text-center text-sm text-slate-400">没有匹配的用户</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="mt-3 flex items-center justify-between text-xs text-slate-500">
        <span>共 {{ total }} 个账号 · 第 {{ page }} / {{ totalPages }} 页</span>
        <div class="flex gap-2">
          <n-button size="small" :disabled="page <= 1" @click="changePage(page - 1)">上一页</n-button>
          <n-button size="small" :disabled="page >= totalPages" @click="changePage(page + 1)">下一页</n-button>
        </div>
      </div>
    </n-spin>

    <!-- 重置密码 -->
    <n-modal v-model:show="passwordModal" preset="card" style="width: 400px" title="重置密码">
      <div class="space-y-2 text-sm">
        <p class="text-xs text-slate-500">
          为「{{ passwordTarget?.username }}」设置新密码（该操作会写入审计日志）
        </p>
        <n-input v-model:value="newPassword" type="password" show-password-on="click" placeholder="至少 6 位" />
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="passwordModal = false">取消</n-button>
          <n-button type="primary" :loading="savingPassword" @click="savePassword">确认重置</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 收藏只读查看 -->
    <n-drawer v-model:show="drawerOpen" :width="440" placement="right">
      <n-drawer-content :title="`${drawerUser?.username ?? ''} 的收藏（只读）`" closable>
        <div class="mb-3 flex justify-end">
          <n-button size="small" @click="cleanInvalid">清理失效条目</n-button>
        </div>
        <n-spin :show="drawerLoading">
          <ul class="space-y-2">
            <li
              v-for="item in drawerItems"
              :key="item.key"
              class="rounded-lg border border-slate-200 p-2.5 text-xs"
            >
              <p class="break-all font-mono text-slate-700">{{ item.rel_path || item.key }}</p>
              <div class="mt-1 flex items-center gap-2">
                <n-tag size="tiny" :bordered="false">{{ item.media_type === 'video' ? '视频' : '图片' }}</n-tag>
                <n-tag v-if="!item.playable" size="tiny" type="error" :bordered="false">当前不可播</n-tag>
                <span v-else class="text-slate-400">可播</span>
              </div>
            </li>
            <li v-if="!drawerItems.length && !drawerLoading" class="py-8 text-center text-slate-400">
              该用户暂无收藏
            </li>
          </ul>
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>
