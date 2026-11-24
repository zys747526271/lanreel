/**
 * 播放偏好（设备级，保存在 localStorage）。
 * 与服务端配置分开：服务端负责策略与媒体，客户端负责「这台设备怎么播」。
 */
import { reactive, watch } from 'vue'

export type PlaybackRate = 0.5 | 1 | 1.25 | 1.5 | 2 | 3

export const RATE_OPTIONS: { value: PlaybackRate; label: string }[] = [
  { value: 0.5, label: '0.5×' },
  { value: 1, label: '1×' },
  { value: 1.25, label: '1.25×' },
  { value: 1.5, label: '1.5×' },
  { value: 2, label: '2×' },
  { value: 3, label: '3×' },
]

export interface Prefs {
  /** 进入播放时静音（默认关闭 = 默认有声） */
  defaultMuted: boolean
  /** 默认倍速 */
  defaultRate: PlaybackRate
  /** 从收藏/历史点开播放时尝试进入全屏 */
  autoFullscreen: boolean
  /** 进入全屏后尝试锁定横屏（移动端有效） */
  autoRotate: boolean
}

const KEY = 'lanreel.prefs'

const defaults: Prefs = {
  defaultMuted: false,
  defaultRate: 1,
  autoFullscreen: false,
  autoRotate: false,
}

function load(): Partial<Prefs> {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? (JSON.parse(raw) as Partial<Prefs>) : {}
  } catch {
    return {}
  }
}

export const prefs = reactive<Prefs>({ ...defaults, ...load() })

watch(
  prefs,
  (value) => {
    try {
      localStorage.setItem(KEY, JSON.stringify(value))
    } catch {
      /* 隐私模式下写入失败不影响播放 */
    }
  },
  { deep: true },
)

export function resetPrefs() {
  Object.assign(prefs, defaults)
}

/** 请求全屏（可选尝试锁定横屏）。必须由用户手势触发。 */
export async function enterFullscreen(rotate: boolean): Promise<void> {
  try {
    const el = document.documentElement
    if (!document.fullscreenElement) {
      await el.requestFullscreen?.()
    }
    if (rotate) {
      const orientation = screen.orientation as ScreenOrientation & {
        lock?: (o: string) => Promise<void>
      }
      await orientation?.lock?.('landscape')
    }
  } catch {
    // 浏览器不支持或拒绝（例如桌面端/未获手势）时忽略，不影响播放
  }
}

export async function exitFullscreen(): Promise<void> {
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    const orientation = screen.orientation as ScreenOrientation & { unlock?: () => void }
    orientation?.unlock?.()
  } catch {
    /* 忽略 */
  }
}
