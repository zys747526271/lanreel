/** 与后端约定的数据类型（/api/v1，见 5. API 草案）。 */

export type Role = 'admin' | 'user'
export type MediaType = 'video' | 'image'
export type ScheduleType = 'always' | 'time_window' | 'disabled'

export interface User {
  id: number
  username: string
  role: Role
  enabled: boolean
  created_at: string
}

export interface TokenPair {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
  access_expires_at: string
  refresh_expires_at: string
}

export interface MediaItem {
  key: string
  media_type: MediaType
  rel_path: string
  folder: string
  duration: number
  size: number
  playable: boolean
  reason?: string
  favorite: boolean
  stream_url: string
  thumb_url: string
  /** 前端本地唯一标识（同一条目可能重复出现） */
  uid?: string
}

export interface PublicConfig {
  app: string
  version: string
  registration_open: boolean
  image_display_seconds: number
  timezone: string
  max_random_limit: number
  scan_interval_seconds: number
  frontend_log_upload: boolean
}

export interface FolderPolicyView {
  folder_path: string
  total: number
  videos: number
  images: number
  enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
  configured: boolean
  inherited: boolean
  inherited_from: string
  updated_at?: string
}

export interface FileOverride {
  id: number
  video_key: string
  enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
  note: string
  updated_at: string
}

export interface FileView {
  key: string
  rel_path: string
  media_type: MediaType
  folder: string
  duration: number
  size: number
  playable: boolean
  reason?: string
  override?: FileOverride
}

export interface UserView extends User {
  favorite_count: number
}

export interface AuditLog {
  id: number
  admin_user_id: number
  target_type: string
  target_id: string
  action: string
  before_value: string
  after_value: string
  created_at: string
}

export interface SystemLogEntry {
  id: number
  ts: string
  level: string
  category: string
  module: string
  message: string
  context_json: string
  request_id: string
  user_id: number | null
  created_at: string
}

export interface ScanSummary {
  started_at: string
  finished_at: string
  duration_ms: number
  roots: number
  total: number
  videos: number
  images: number
  skipped: number
  added: number
  removed: number
  errors: string[] | null
}

export interface AdminStats {
  users: number
  admins: number
  favorites: number
  media_total: number
  videos: number
  images: number
  folder_policies: number
  file_overrides: number
  scan: ScanSummary
  log_counts: Record<string, number>
  registration_open: boolean
  logging: LoggingSettings
}

export interface LoggingSettings {
  retention_days: number
  max_entries: number
  level: string
  frontend_upload_enabled: boolean
}

export interface Paged<T> {
  data: T[]
  meta: { total: number; page: number; page_size: number }
}
