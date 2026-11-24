import { request } from './client'
import type {
  AdminStats,
  AuditLog,
  FileView,
  FolderPolicyView,
  LoggingSettings,
  MediaItem,
  Paged,
  ScheduleType,
  SystemLogEntry,
  UserView,
} from './types'

// ---------- 概览与扫描 ----------
export function fetchStats() {
  return request<{ data: AdminStats }>('/admin/stats')
}

export function triggerScan() {
  return request<{ data: { media_total: number } }>('/admin/scan', { method: 'POST' })
}

// ---------- 文件夹策略 ----------
export function listFolders() {
  return request<{ data: FolderPolicyView[] }>('/admin/folders')
}

export function setFolderPolicy(payload: {
  folder_path: string
  enabled: boolean
  schedule_type: ScheduleType
  time_start?: string
  time_end?: string
}) {
  return request<{ data: FolderPolicyView }>('/admin/folders', { method: 'PUT', body: payload })
}

export function deleteFolderPolicy(folderPath: string) {
  return request<{ data: { success: boolean } }>(
    `/admin/folders?folder_path=${encodeURIComponent(folderPath)}`,
    { method: 'DELETE' },
  )
}

// ---------- 文件级策略 ----------
export function listFiles(params: { page?: number; page_size?: number; keyword?: string; folder?: string }) {
  const search = new URLSearchParams()
  search.set('page', String(params.page ?? 1))
  search.set('page_size', String(params.page_size ?? 20))
  if (params.keyword) search.set('keyword', params.keyword)
  if (params.folder) search.set('folder', params.folder)
  return request<Paged<FileView>>(`/admin/files?${search.toString()}`)
}

export function setFileOverride(
  key: string,
  payload: { enabled: boolean; schedule_type: ScheduleType; time_start?: string; time_end?: string; note?: string },
) {
  return request<{ data: FileView }>(`/admin/files/${key}`, { method: 'PUT', body: payload })
}

export function deleteFileOverride(key: string) {
  return request<{ data: { success: boolean } }>(`/admin/files/${key}`, { method: 'DELETE' })
}

// ---------- 用户管理 ----------
export function listUsers(params: { page?: number; page_size?: number; keyword?: string }) {
  const search = new URLSearchParams()
  search.set('page', String(params.page ?? 1))
  search.set('page_size', String(params.page_size ?? 20))
  if (params.keyword) search.set('keyword', params.keyword)
  return request<Paged<UserView>>(`/admin/users?${search.toString()}`)
}

export function updateUser(
  id: number,
  payload: { enabled?: boolean; role?: 'admin' | 'user'; password?: string },
) {
  return request<{ data: UserView }>(`/admin/users/${id}`, { method: 'PUT', body: payload })
}

export function deleteUser(id: number) {
  return request<{ data: { success: boolean } }>(`/admin/users/${id}`, { method: 'DELETE' })
}

export function listUserFavorites(id: number) {
  return request<{ data: MediaItem[] }>(`/admin/users/${id}/favorites`)
}

export function cleanUserFavorites(id: number) {
  return request<{ data: { removed: number } }>(`/admin/users/${id}/favorites/clean`, {
    method: 'POST',
  })
}

// ---------- 日志与审计 ----------
export function listAuditLogs(params: {
  page?: number
  page_size?: number
  target_type?: string
  keyword?: string
  from?: string
  to?: string
}) {
  const search = new URLSearchParams()
  search.set('page', String(params.page ?? 1))
  search.set('page_size', String(params.page_size ?? 20))
  if (params.target_type) search.set('target_type', params.target_type)
  if (params.keyword) search.set('keyword', params.keyword)
  if (params.from) search.set('from', params.from)
  if (params.to) search.set('to', params.to)
  return request<Paged<AuditLog>>(`/admin/audit-logs?${search.toString()}`)
}

export function listLogs(params: {
  page?: number
  page_size?: number
  category?: string
  level?: string
  keyword?: string
  request_id?: string
  from?: string
  to?: string
}) {
  const search = new URLSearchParams()
  search.set('page', String(params.page ?? 1))
  search.set('page_size', String(params.page_size ?? 50))
  Object.entries(params).forEach(([k, v]) => {
    if (v && k !== 'page' && k !== 'page_size') search.set(k, String(v))
  })
  return request<Paged<SystemLogEntry>>(`/admin/logs?${search.toString()}`)
}

export function purgeLogs() {
  return request<{ data: { removed: number } }>('/admin/logs/purge', { method: 'POST' })
}

// ---------- 设置 ----------
export function getLoggingSettings() {
  return request<{ data: LoggingSettings }>('/admin/settings/logging')
}

export function setLoggingSettings(payload: LoggingSettings) {
  return request<{ data: LoggingSettings }>('/admin/settings/logging', {
    method: 'PUT',
    body: payload,
  })
}

export function setRegistration(enabled: boolean) {
  return request<{ data: { registration_open: boolean } }>('/admin/settings/registration', {
    method: 'PUT',
    body: { enabled },
  })
}
