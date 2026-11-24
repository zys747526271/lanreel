/**
 * 页面级播放器互斥：任意时刻只允许一个 <video> 真正在播放。
 *
 * 会话恢复时会出现「首屏第 0 条先自动播放 → 随后切到恢复条目」的竞态：
 * play() 是异步的，旧条目的 pause() 可能与它交错，结果两条同时出声。
 * 这里用注册表在每次 play 前统一暂停其它条目，并在 play 完成后复核 active 状态。
 */
const videos = new Set<HTMLVideoElement>()

export function registerVideo(el: HTMLVideoElement) {
  videos.add(el)
}

export function unregisterVideo(el: HTMLVideoElement | null | undefined) {
  if (el) videos.delete(el)
}

/** 播放当前视频前，暂停其它所有已注册播放器。 */
export function pauseOtherVideos(active: HTMLVideoElement) {
  for (const el of videos) {
    if (el !== active && !el.paused) el.pause()
  }
}

/** 兜底：暂停全部播放器（离开播放器页面 / 组件树重建前调用）。 */
export function pauseAllVideos() {
  for (const el of videos) {
    if (!el.paused) el.pause()
  }
}

/** 仅用于诊断 */
export function playingVideoCount() {
  let count = 0
  for (const el of videos) if (!el.paused) count += 1
  return count
}
