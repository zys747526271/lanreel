/**
 * 播放偏好（设备级，保存在 localStorage）。
 * 与服务端配置分开：服务端负责策略与媒体，客户端负责「这台设备怎么播」。
 */
import { reactive, watch } from 'vue'

export type PlaybackRate = 0.5 | 1 | 1.25 | 1.5 | 2 | 3

/** 播放结束后的行为（v0.3.0 需求 3）。 */
export type EndAction = 'pause' | 'loop' | 'next'

export const END_ACTION_OPTIONS: { value: EndAction; label: string; desc: string }[] = [
  { value: 'pause', label: '暂停', desc: '播放结束后停在当前画面' },
  { value: 'loop', label: '循环播放', desc: '当前视频从头重播' },
  { value: 'next', label: '自动下一个', desc: '切到下一条媒体' },
]

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
  /** 播放结束后的行为（v0.3.0） */
  endAction: EndAction
  /** 随机流刷到已看过的视频时是否从头播放（v0.3.0 需求 2） */
  randomRestartWatched: boolean
}

const KEY = 'lanreel.prefs'

const defaults: Prefs = {
  defaultMuted: false,
  defaultRate: 1,
  autoFullscreen: false,
  autoRotate: false,
  endAction: 'pause',
  randomRestartWatched: true,
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
    document.documentElement.classList.remove('reel-css-fullscreen-host')
    const orientation = screen.orientation as ScreenOrientation & { unlock?: () => void }
    orientation?.unlock?.()
  } catch {
    /* 忽略 */
  }
}
