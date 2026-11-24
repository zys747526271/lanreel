/**
 * 不可播放原因枚举 → 展示文案（9.4：新增原因不破坏旧前端）。
 */
export function reasonText(reason?: string): string {
  switch (reason) {
    case 'disabled_by_admin':
      return '该内容已被管理员禁用'
    case 'out_of_time_window':
      return '当前不在可播放时间段'
    case 'file_missing':
      return '媒体文件已丢失'
    case 'not_favorited':
      return '尚未收藏该内容'
    default:
      return '当前不可播放'
  }
}

export function formatDuration(seconds?: number): string {
  if (!seconds || seconds <= 0) return ''
  const total = Math.round(seconds)
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

export function formatBytes(size?: number): string {
  if (!size) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = size
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function formatTime(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
