import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { installGlobalErrorHooks, logger } from './lib/logger'
import { fetchPublicConfig } from './api/client'
import { clearAllPlayerSessions } from './lib/session'
import { pauseAllVideos } from './lib/playerRegistry'
import './styles/main.css'

installGlobalErrorHooks()

/**
 * 只有真正的手动刷新才清掉会话并重新随机。
 * 不再监听 pagehide/beforeunload 清会话：手机横竖屏或全屏时浏览器可能重建 WebView，
 * 此时清掉会话会让页面像被刷新一样重新随机，出现黑屏与串音。
 */
const navEntry = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined
if (navEntry?.type === 'reload') clearAllPlayerSessions()

// 页面进入 bfcache / 关闭时暂停所有视频，避免后台残留别的视频声音。
window.addEventListener('pagehide', () => pauseAllVideos())
window.addEventListener('pageshow', (event) => {
  if (event.persisted) pauseAllVideos()
})
// 播放器自管滚动位置，交给浏览器恢复容易错位。
if ('scrollRestoration' in history) history.scrollRestoration = 'manual'

// 读取服务端配置：决定是否允许前端日志上报（F-41，默认关闭）
void fetchPublicConfig()
  .then((cfg) => logger.setUploadEnabled(cfg.frontend_log_upload))
  .catch(() => logger.setUploadEnabled(false))

const app = createApp(App)
app.use(createPinia())
app.use(router)

app.config.errorHandler = (err, _instance, info) => {
  // 同时输出到 console，便于开发排查（生产仍以本地日志为准）
  console.error('[lanreel][vue]', err, info)
  logger.error('exception', 'vue', '组件渲染异常', {
    message: err instanceof Error ? err.message : String(err),
    stack: err instanceof Error ? err.stack?.slice(0, 1024) : undefined,
    info,
  })
}

window.addEventListener('load', () => {
  const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined
  if (nav) {
    logger.info('performance', 'boot', '首屏加载完成', {
      dom_ready_ms: Math.round(nav.domContentLoadedEventEnd),
      load_ms: Math.round(nav.loadEventEnd),
      transfer_size: nav.transferSize,
    })
  }
})

app.mount('#app')
