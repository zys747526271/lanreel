import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { installGlobalErrorHooks, logger } from './lib/logger'
import { fetchPublicConfig } from './api/client'
import { clearAllPlayerSessions } from './lib/session'
import './styles/main.css'

installGlobalErrorHooks()

// 整页刷新（F5）不是「从设置返回」：清掉会话，下次进入重新随机播放。
window.addEventListener('pagehide', clearAllPlayerSessions)
window.addEventListener('beforeunload', clearAllPlayerSessions)

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
