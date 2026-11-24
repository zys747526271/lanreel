import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'feed',
      component: () => import('@/views/FeedView.vue'),
      // 游客可直接浏览，可见范围由服务端策略的「游客可见」控制
      meta: { title: '浏览' },
    },
    {
      path: '/setup',
      name: 'setup',
      component: () => import('@/views/SetupView.vue'),
      meta: { title: '首次部署 · 创建管理员' },
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { title: '登录' },
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('@/views/RegisterView.vue'),
      meta: { title: '注册' },
    },
    {
      path: '/favorites',
      name: 'favorites',
      component: () => import('@/views/FavoritesView.vue'),
      meta: { requiresAuth: true, title: '收藏夹' },
    },
    {
      path: '/history',
      name: 'history',
      component: () => import('@/views/HistoryView.vue'),
      meta: { requiresAuth: true, title: '观看历史' },
    },
    {
      path: '/favorites/play',
      name: 'favorites-play',
      component: () => import('@/views/FavoritesPlayView.vue'),
      meta: { requiresAuth: true, title: '收藏播放' },
    },
    {
      path: '/play',
      name: 'history-play',
      component: () => import('@/views/FavoritesPlayView.vue'),
      meta: { requiresAuth: true, title: '观看历史' },
    },
    {
      path: '/forbidden',
      name: 'forbidden',
      component: () => import('@/views/ForbiddenView.vue'),
      meta: { requiresAuth: true, title: '无访问权限' },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
      meta: { requiresAuth: true, title: '设置与诊断' },
    },
    {
      // 管理后台：路由 + 菜单配置驱动（9.6）
      path: '/admin',
      component: () => import('@/views/admin/AdminLayout.vue'),
      meta: { requiresAuth: true, requiresAdmin: true },
      children: [
        { path: '', redirect: '/admin/dashboard' },
        {
          path: 'dashboard',
          name: 'admin-dashboard',
          component: () => import('@/views/admin/DashboardView.vue'),
          meta: { title: '概览' },
        },
        {
          path: 'media',
          name: 'admin-media',
          component: () => import('@/views/admin/MediaSourcesView.vue'),
          meta: { title: '媒体目录' },
        },
        {
          path: 'folders',
          name: 'admin-folders',
          component: () => import('@/views/admin/FoldersView.vue'),
          meta: { title: '文件夹策略' },
        },
        {
          path: 'files',
          name: 'admin-files',
          component: () => import('@/views/admin/FilesView.vue'),
          meta: { title: '文件策略' },
        },
        {
          path: 'users',
          name: 'admin-users',
          component: () => import('@/views/admin/UsersView.vue'),
          meta: { title: '用户管理' },
        },
        {
          path: 'logs',
          name: 'admin-logs',
          component: () => import('@/views/admin/LogsView.vue'),
          meta: { title: '运行日志' },
        },
        {
          path: 'audit',
          name: 'admin-audit',
          component: () => import('@/views/admin/AuditView.vue'),
          meta: { title: '审计日志' },
        },
        {
          path: 'settings',
          name: 'admin-settings',
          component: () => import('@/views/admin/SiteSettingsView.vue'),
          meta: { title: '系统设置' },
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  // 首次部署：未初始化时把任意入口都导向创建管理员页；初始化后 /setup 失效
  const setupRequired = await auth.loadSetupStatus()
  if (setupRequired) {
    return to.path === '/setup' ? true : { path: '/setup' }
  }
  if (to.path === '/setup') {
    return { path: auth.isLoggedIn ? '/' : '/login' }
  }

  if (!auth.initialized) await auth.bootstrap()

  if (to.meta.requiresAuth && !auth.isLoggedIn) {
    return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
  }
  if (to.meta.requiresAdmin && !auth.isAdmin) {
    // 不静默重定向：给出明确的权限说明与切换账号入口
    return { path: '/forbidden', query: { from: to.fullPath } }
  }
  if ((to.path === '/login' || to.path === '/register') && auth.isLoggedIn) {
    return { path: '/' }
  }
  return true
})

router.afterEach((to) => {
  const title = (to.meta.title as string) || ''
  document.title = title ? `${title} · LanReel` : 'LanReel'
})

export default router
