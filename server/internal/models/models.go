// Package models 定义业务实体（对应第 6 章数据模型）。
package models

import "time"

// 角色常量。
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
	// RoleGuest 不是数据库中的账号角色，而是未登录访问者的判定身份（CR-004）。
	RoleGuest = "guest"
)

// RootGlobal 文件夹策略的「全局默认」根索引：对所有媒体根生效。
const RootGlobal = -1

// 媒体类型常量。
const (
	MediaVideo = "video"
	MediaImage = "image"
)

// 调度类型常量。
const (
	ScheduleAlways     = "always"
	ScheduleTimeWindow = "time_window"
	ScheduleDisabled   = "disabled"
)

// 不可播放原因枚举（9.4）。
const (
	ReasonDisabledByAdmin = "disabled_by_admin"
	ReasonOutOfWindow     = "out_of_time_window"
	ReasonFileMissing     = "file_missing"
	ReasonNotFavorited    = "not_favorited"
	ReasonLoginRequired   = "login_required"
)

// User 用户账号。
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// IsAdmin 是否管理员。
func (u *User) IsAdmin() bool { return u != nil && u.Role == RoleAdmin }

// Favorite 收藏记录。
type Favorite struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoKey  string    `json:"video_key"`
	VideoPath string    `json:"video_path"`
	CreatedAt time.Time `json:"created_at"`
}

// FolderPolicy 文件夹策略（按根索引隔离，B-07）。
// v0.3.0：普通用户与游客各自拥有独立的播放时段（GuestSchedule*）。
type FolderPolicy struct {
	ID           int64     `json:"id"`
	RootIndex    int       `json:"root_index"`
	FolderPath   string    `json:"folder_path"`
	Enabled      bool      `json:"enabled"`
	GuestEnabled bool      `json:"guest_enabled"`
	ScheduleType string    `json:"schedule_type"`
	TimeStart    string    `json:"time_start"`
	TimeEnd      string    `json:"time_end"`
	GuestScheduleType string `json:"guest_schedule_type"`
	GuestTimeStart    string `json:"guest_time_start"`
	GuestTimeEnd      string `json:"guest_time_end"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// FileOverride 文件级覆盖。
// v0.3.0：普通用户与游客各自拥有独立的播放时段（GuestSchedule*）。
type FileOverride struct {
	ID           int64     `json:"id"`
	VideoKey     string    `json:"video_key"`
	Enabled      bool      `json:"enabled"`
	GuestEnabled bool      `json:"guest_enabled"`
	ScheduleType string    `json:"schedule_type"`
	TimeStart    string    `json:"time_start"`
	TimeEnd      string    `json:"time_end"`
	GuestScheduleType string `json:"guest_schedule_type"`
	GuestTimeStart    string `json:"guest_time_start"`
	GuestTimeEnd      string `json:"guest_time_end"`
	Note         string    `json:"note"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// WatchHistory 观看历史（服务端存储，按用户隔离，v0.3.0）。
type WatchHistory struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	VideoKey       string    `json:"video_key"`
	RelPath        string    `json:"rel_path"`
	MediaType      string    `json:"media_type"`
	RootIndex      int       `json:"root_index"`
	FolderPath     string    `json:"folder_path"`
	Duration       float64   `json:"duration"`
	Position       float64   `json:"position"`
	WatchedSeconds float64   `json:"watched_seconds"`
	PlayCount      int       `json:"play_count"`
	Completed      bool      `json:"completed"`
	LastPlayedAt   time.Time `json:"last_played_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// MediaStats 单个媒体的聚合统计（热度/播放/收藏，v0.3.0）。
type MediaStats struct {
	VideoKey       string    `json:"video_key"`
	PlayCount      int       `json:"play_count"`
	CompletedCount int       `json:"completed_count"`
	WatchedSeconds float64   `json:"watched_seconds"`
	FavoriteCount  int       `json:"favorite_count"`
	LastPlayedAt   time.Time `json:"last_played_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FolderStats 文件夹聚合统计（v0.3.0）。
type FolderStats struct {
	RootIndex      int       `json:"root_index"`
	FolderPath     string    `json:"folder_path"`
	PlayCount      int       `json:"play_count"`
	CompletedCount int       `json:"completed_count"`
	WatchedSeconds float64   `json:"watched_seconds"`
	FavoriteCount  int       `json:"favorite_count"`
	MediaCount     int       `json:"media_count"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// MediaMeta 扫描缓存。
type MediaMeta struct {
	VideoKey  string    `json:"video_key"`
	MediaType string    `json:"media_type"`
	Duration  float64   `json:"duration"`
	ThumbPath string    `json:"thumb_path"`
	ScannedAt time.Time `json:"scanned_at"`
}

// AuditLog 管理员操作审计（业务库内，独立于运行日志）。
type AuditLog struct {
	ID          int64     `json:"id"`
	AdminUserID int64     `json:"admin_user_id"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	Action      string    `json:"action"`
	BeforeValue string    `json:"before_value"`
	AfterValue  string    `json:"after_value"`
	CreatedAt   time.Time `json:"created_at"`
}

// 审计对象类型。
const (
	AuditTargetUser    = "user"
	AuditTargetFolder  = "folder"
	AuditTargetFile    = "file"
	AuditTargetSetting = "setting"
	AuditTargetLog     = "log"
)

// SystemLogEntry 运行日志（独立日志库 logs.db / 前端 app_logs store，字段语义对齐）。
type SystemLogEntry struct {
	ID          int64  `json:"id"`
	TS          string `json:"ts"`
	Level       string `json:"level"`
	Category    string `json:"category"`
	Module      string `json:"module"`
	Message     string `json:"message"`
	ContextJSON string `json:"context_json"`
	RequestID   string `json:"request_id"`
	UserID      *int64 `json:"user_id"`
	CreatedAt   string `json:"created_at"`
}
