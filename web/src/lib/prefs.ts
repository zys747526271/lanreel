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

/* ------------------------- 手势配置（v0.3.1） ------------------------- */

/** 三种互斥场景：普通、沉浸、全屏；沉浸+全屏叠加时全屏优先。 */
export type GestureContext = 'normal' | 'immersive' | 'fullscreen'
export type SingleTapAction = 'pause' | 'advance' | 'immersive' | 'off'
export type DoubleTapAction = 'favorite' | 'pause' | 'off'
export type VerticalSwipeAction = 'switch' | 'volume' | 'off'
export type HorizontalSwipeAction = 'seek' | 'volume' | 'off'

export interface GestureConfig {
  singleTap: SingleTapAction
  doubleTap: DoubleTapAction
  vertical: VerticalSwipeAction
  horizontal: HorizontalSwipeAction
  longPressRate: boolean
}
export type GesturePrefs = Record<GestureContext, GestureConfig>

export const GESTURE_CONTEXTS: { value: GestureContext; label: string; desc: string }[] = [
  { value: 'normal', label: '普通播放', desc: '普通竖屏信息流；沉浸+全屏叠加时以全屏配置为准' },
  { value: 'immersive', label: '沉浸模式', desc: '隐藏页面 UI，仅保留退出按钮' },
  { value: 'fullscreen', label: '全屏模式', desc: '原生或 CSS 全屏，优先级最高' },
]

export const SINGLE_TAP_OPTIONS: { value: SingleTapAction; label: string }[] = [
  { value: 'pause', label: '暂停 / 播放' },
  { value: 'advance', label: '下一个视频' },
  { value: 'immersive', label: '进入 / 退出沉浸' },
  { value: 'off', label: '无操作' },
]
export const DOUBLE_TAP_OPTIONS: { value: DoubleTapAction; label: string }[] = [
  { value: 'favorite', label: '点赞 / 收藏' },
  { value: 'pause', label: '暂停 / 播放' },
  { value: 'off', label: '无操作' },
]
export const VERTICAL_OPTIONS: { value: VerticalSwipeAction; label: string }[] = [
  { value: 'switch', label: '切换视频' },
  { value: 'volume', label: '调节音量' },
  { value: 'off', label: '无操作' },
]
export const HORIZONTAL_OPTIONS: { value: HorizontalSwipeAction; label: string }[] = [
  { value: 'seek', label: '拖动快进 / 快退' },
  { value: 'volume', label: '调节音量' },
  { value: 'off', label: '无操作' },
]

const DEFAULT_GESTURES: GesturePrefs = {
  normal: { singleTap: 'pause', doubleTap: 'favorite', vertical: 'switch', horizontal: 'off', longPressRate: true },
  immersive: { singleTap: 'pause', doubleTap: 'favorite', vertical: 'volume', horizontal: 'seek', longPressRate: true },
  fullscreen: { singleTap: 'pause', doubleTap: 'favorite', vertical: 'volume', horizontal: 'seek', longPressRate: true },
}

function cloneGestures(): GesturePrefs {
  return JSON.parse(JSON.stringify(DEFAULT_GESTURES)) as GesturePrefs
}

/** 旧配置只含部分字段时按默认值补齐，并过滤非法取值。 */
function mergeGestures(raw: unknown): GesturePrefs {
  const out = cloneGestures()
  if (!raw || typeof raw !== 'object') return out
  const source = raw as Record<string, Partial<GestureConfig>>
  for (const ctx of ['normal', 'immersive', 'fullscreen'] as GestureContext[]) {
    const item = source[ctx]
    if (!item || typeof item !== 'object') continue
    if (SINGLE_TAP_OPTIONS.some((o) => o.value === item.singleTap)) out[ctx].singleTap = item.singleTap!
    if (DOUBLE_TAP_OPTIONS.some((o) => o.value === item.doubleTap)) out[ctx].doubleTap = item.doubleTap!
    if (VERTICAL_OPTIONS.some((o) => o.value === item.vertical)) out[ctx].vertical = item.vertical!
    if (HORIZONTAL_OPTIONS.some((o) => o.value === item.horizontal)) out[ctx].horizontal = item.horizontal!
    if (typeof item.longPressRate === 'boolean') out[ctx].longPressRate = item.longPressRate
  }
  return out
}

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
  /** 三种场景的手势映射（v0.3.1） */
  gestures: GesturePrefs
}

const KEY = 'lanreel.prefs'

const defaults: Prefs = {
  defaultMuted: false,
  defaultRate: 1,
  autoFullscreen: false,
  autoRotate: false,
  endAction: 'pause',
  randomRestartWatched: true,
  gestures: cloneGestures(),
}

function load(): Partial<Prefs> {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as Partial<Prefs>
    // 非法旧数据可能来自手工编辑 localStorage，所有枚举字段都回落到默认值。
    if (typeof parsed.defaultMuted !== 'boolean') parsed.defaultMuted = defaults.defaultMuted
    if (!RATE_OPTIONS.some((o) => o.value === parsed.defaultRate)) parsed.defaultRate = defaults.defaultRate
    if (typeof parsed.autoFullscreen !== 'boolean') parsed.autoFullscreen = defaults.autoFullscreen
    if (typeof parsed.autoRotate !== 'boolean') parsed.autoRotate = defaults.autoRotate
    if (typeof parsed.randomRestartWatched !== 'boolean') parsed.randomRestartWatched = defaults.randomRestartWatched
    if (!END_ACTION_OPTIONS.some((o) => o.value === parsed.endAction)) parsed.endAction = defaults.endAction
    // 手势配置必须逐字段补齐，否则旧浏览器升级后会出现 undefined 配置。
    return { ...parsed, gestures: mergeGestures(parsed.gestures) }
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
  Object.assign(prefs, { ...defaults, gestures: cloneGestures() })
}

/** 只恢复某一个场景的手势默认值。 */
export function resetGestureContext(ctx: GestureContext) {
  Object.assign(prefs.gestures[ctx], cloneGestures()[ctx])
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
