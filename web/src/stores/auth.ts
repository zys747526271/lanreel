import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as authApi from '@/api/auth'
import * as setupApi from '@/api/setup'
import { ApiError, tokenStorage } from '@/api/client'
import { getItem, putItem, deleteItem } from '@/lib/db'
import { logger } from '@/lib/logger'
import type { User } from '@/api/types'

interface CachedUser {
  key: string
  user: User
  cached_at: string
}

const USER_CACHE_KEY = 'current_user'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const registrationOpen = ref(true)
  const loading = ref(false)
  const initialized = ref(false)
  /** null 表示尚未查询：首次启动由路由守卫拉取一次，决定是否进入初始化页面 */
  const setupRequired = ref<boolean | null>(null)

  const isLoggedIn = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  /** 查询（并缓存）首次初始化状态。失败时不阻断页面，按已初始化处理。 */
  async function loadSetupStatus(force = false): Promise<boolean> {
    if (!force && setupRequired.value !== null) return setupRequired.value
    try {
      const status = await setupApi.fetchSetupStatus()
      setupRequired.value = status.setup_required
    } catch {
      // 后端不可达：不把用户困在初始化页，登录/浏览页会各自给出网络提示
      setupRequired.value = false
    }
    return setupRequired.value
  }

  /** 完成首次初始化：创建管理员并直接登录。 */
  async function completeSetup(username: string, password: string) {
    loading.value = true
    try {
      const data = await setupApi.createInitialAdmin(username, password)
      user.value = data.user
      tokenStorage.save(data.tokens)
      setupRequired.value = false
      initialized.value = true
      logger.setUser(data.user.id)
      await cacheUser(data.user)
      logger.info('auth', 'setup', '首次初始化完成，已创建管理员', {
        user_id: data.user.id,
        role: data.user.role,
      })
      return data.user
    } finally {
      loading.value = false
    }
  }

  async function restoreCache() {
    try {
      const cached = await getItem<CachedUser>('user_state', USER_CACHE_KEY)
      if (cached?.user && !user.value) {
        user.value = cached.user
        logger.setUser(cached.user.id)
      }
    } catch (err) {
      logger.warn('app', 'auth', '读取用户缓存失败', { error: err instanceof Error ? err.message : String(err) })
    }
  }

  async function cacheUser(u: User | null) {
    try {
      if (u) {
        await putItem<CachedUser>('user_state', {
          key: USER_CACHE_KEY,
          user: u,
          cached_at: new Date().toISOString(),
        })
      } else {
        await deleteItem('user_state', USER_CACHE_KEY)
      }
    } catch {
      /* 缓存失败不影响主流程 */
    }
  }

  /** 启动时校验本地 Token；失败则清理登录态。 */
  async function bootstrap() {
    if (initialized.value) return
    initialized.value = true
    if (!tokenStorage.access) {
      await restoreCache()
      user.value = null
      return
    }
    try {
      const res = await authApi.me()
      user.value = res.data.user
      registrationOpen.value = res.data.registration_open
      logger.setUser(res.data.user.id)
      await cacheUser(res.data.user)
      logger.info('auth', 'session', '会话校验成功', { user_id: res.data.user.id })
    } catch (err) {
      user.value = null
      logger.setUser(null)
      if (err instanceof ApiError && err.status === 401) {
        tokenStorage.clear()
        await cacheUser(null)
      }
      await restoreCache()
    }
  }

  async function login(username: string, password: string) {
    loading.value = true
    try {
      const res = await authApi.login(username, password)
      user.value = res.data.user
      tokenStorage.save(res.data.tokens)
      logger.setUser(res.data.user.id)
      await cacheUser(res.data.user)
      logger.info('auth', 'login', '登录成功', { user_id: res.data.user.id, role: res.data.user.role })
      return res.data.user
    } finally {
      loading.value = false
    }
  }

  async function register(username: string, password: string) {
    loading.value = true
    try {
      const res = await authApi.register(username, password)
      user.value = res.data.user
      tokenStorage.save(res.data.tokens)
      logger.setUser(res.data.user.id)
      await cacheUser(res.data.user)
      logger.info('auth', 'register', '注册成功', { user_id: res.data.user.id })
      return res.data.user
    } finally {
      loading.value = false
    }
  }

  /** 登出：清 Token 与本地缓存（F-10 / N-09）。 */
  async function logout() {
    try {
      await authApi.logout()
    } catch {
      /* 网络异常也要完成本地登出 */
    }
    logger.info('auth', 'logout', '已登出并清理本地缓存')
    // 登出时清理本地缓存并对日志做匿名化（F-10 / N-09 / N-27）
    await logger.anonymize()
    tokenStorage.clear()
    user.value = null
    logger.setUser(null)
    await cacheUser(null)
  }

  function setRegistrationOpen(v: boolean) {
    registrationOpen.value = v
  }

  return {
    user,
    registrationOpen,
    loading,
    initialized,
    setupRequired,
    isLoggedIn,
    isAdmin,
    loadSetupStatus,
    completeSetup,
    bootstrap,
    login,
    register,
    logout,
    setRegistrationOpen,
  }
})
