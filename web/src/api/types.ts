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
  /** 媒体根索引与名称（多根目录展示用） */
  root_index: number
  root_name: string
  rel_path: string
  folder: string
  duration: number
  size: number
  playable: boolean
  reason?: string
  favorite: boolean
  /** 当前视频被收藏的次数（v0.3.0，非全站总数） */
  favorite_count?: number
  /** 观看进度 0~1（v0.3.0，收藏/历史网格显示进度条） */
  watchProgress?: number
  stream_url: string
  thumb_url: string
  /** 前端本地唯一标识（同一条目可能重复出现） */
  uid?: string
}

/** 单条观看进度（v0.3.0）。 */
export interface ProgressView {
  position: number
  duration: number
  progress: number
  completed: boolean
  play_count: number
  /** 累计真实观看时长（拖动进度条不计） */
  watched_seconds: number
}

/** 服务端观看历史条目（按用户隔离，v0.3.0）。 */
export interface HistoryEntryView {
  key: string
  media_type: MediaType
  rel_path: string
  root_index: number
  root_name: string
  folder: string
  duration: number
  position: number
  progress: number
  play_count: number
  completed: boolean
  playable: boolean
  reason?: string
  stream_url: string
  thumb_url: string
  last_played_at: string
  favorite_count: number
}

/** 用户观看汇总（v0.3.0）。 */
export interface WatchSummary {
  watched_videos: number
  completed_videos: number
  total_watch_time: number
}

/** 今日全站观看汇总（不统计完播数） */
export interface DailyWatchSummary {
  watched_videos: number
  total_watch_time: number
}

/** 视频/文件夹热度（v0.3.0）。 */
export interface HeatView {
  key?: string
  rel_path?: string
  folder: string
  root_index: number
  root_name?: string
  media_type?: string
  play_count: number
  completed_count: number
  watched_seconds: number
  favorite_count: number
  last_played_at?: string
  heat_score: number
  media_count?: number
}

export interface PublicConfig {
  /** 是否允许游客（未登录）浏览媒体 */
  guest_access?: boolean
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
  /** 媒体根索引；-1 表示全局默认策略 */
  root_index: number
  root_name: string
  folder_path: string
  total: number
  videos: number
  images: number
  enabled: boolean
  guest_enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
  /** 游客独立播放时段（v0.3.0） */
  guest_schedule_type: ScheduleType
  guest_time_start: string
  guest_time_end: string
  configured: boolean
  inherited: boolean
  inherited_from: string
  inherited_root: number
  updated_at?: string
  /** 热度统计（v0.3.0） */
  play_count: number
  watched_seconds: number
  favorite_count: number
  recursive: boolean
}

export interface FileOverride {
  id: number
  video_key: string
  enabled: boolean
  guest_enabled: boolean
  schedule_type: ScheduleType
  time_start: string
  time_end: string
  guest_schedule_type: ScheduleType
  guest_time_start: string
  guest_time_end: string
  note: string
  updated_at: string
}

export interface FileView {
  key: string
  root_index: number
  root_name: string
  rel_path: string
  media_type: MediaType
  folder: string
  duration: number
  size: number
  playable: boolean
  reason?: string
  guest_playable: boolean
  guest_reason?: string
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
  /** 历史全站观看与时长 */
  watch_summary: WatchSummary
  /** 今日全站观看与时长 */
  today_watch_summary: DailyWatchSummary
}

/** 媒体目录设置（后台可热更新，CR-003）。 */
export interface RootSetting {
  path: string
  recursive: boolean
}

export interface MediaSettings {
  roots: string[]
  /** 每个根的递归选项（v0.3.0） */
  root_settings: RootSetting[]
  /** database=后台设置优先，config=使用配置文件 media.roots */
  source: 'database' | 'config'
  config_roots: string[]
  scan_interval_seconds: number
  image_display_seconds: number
  scanning: boolean
  media_total: number
  scan: ScanSummary
}

/** 目录浏览器条目。 */
export interface DirEntry {
  name: string
  path: string
}

export interface DirListing {
  current: string
  parent: string
  entries: DirEntry[]
  separator: string
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
