// Package config 负责加载 LanReel 服务端配置。
//
// 配置来源优先级（后者覆盖前者）：
//  1. 内置默认值
//  2. YAML 配置文件（默认 ./config.yaml，可用 LANREEL_CONFIG 指定）
//  3. 环境变量（见 applyEnv，便于容器化与 NAS 部署）
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // 内嵌时区数据，避免 Windows/NAS 缺少 tzdata 导致 Asia/Shanghai 解析失败

	"gopkg.in/yaml.v3"
)

// TLSConfig 控制 HTTPS 开关（N-12 / N-13）。
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// ServerConfig HTTP 服务参数。
type ServerConfig struct {
	Host string    `yaml:"host"`
	Port int       `yaml:"port"`
	TLS  TLSConfig `yaml:"tls"`
	// TrustedProxies 为空表示不信任任何代理，直接使用 RemoteAddr。
	TrustedProxies []string `yaml:"trusted_proxies"`
}

// MediaConfig 媒体扫描参数。
type MediaConfig struct {
	// Roots 支持多个媒体根目录（F-06）。
	Roots []string `yaml:"roots"`
	// VideoExtensions / ImageExtensions 决定扫描纳入的格式。
	VideoExtensions []string `yaml:"video_extensions"`
	ImageExtensions []string `yaml:"image_extensions"`
	// ScanIntervalSeconds 定时重扫间隔；0 表示仅启动时扫描。
	ScanIntervalSeconds int `yaml:"scan_interval_seconds"`
	// ImageDisplaySeconds 图片自动切换停留时长（F-03）。
	ImageDisplaySeconds int `yaml:"image_display_seconds"`
	// IgnoredDirs 不参与扫描的目录名（任意层级命中即跳过）。
	IgnoredDirs []string `yaml:"ignored_dirs"`

	// 缩略图（B-01）：图片用标准库缩放，视频依赖 ffmpeg。
	ThumbnailEnabled     bool    `yaml:"thumbnail_enabled"`
	ThumbnailWidth       int     `yaml:"thumbnail_width"`
	ThumbnailSeekSeconds float64 `yaml:"thumbnail_seek_seconds"`
	// FFmpegPath 显式指定 ffmpeg；空则依次查找 PATH 与程序目录。
	FFmpegPath string `yaml:"ffmpeg_path"`

	// 文件系统监听（B-04）：监听变化后去抖触发扫描；定时扫描仍作为兜底。
	WatchEnabled         bool `yaml:"watch_enabled"`
	WatchDebounceSeconds int  `yaml:"watch_debounce_seconds"`
	WatchMaxDirs         int  `yaml:"watch_max_dirs"`
}

// DatabaseConfig 业务库参数（日志库单独配置，见 LoggingConfig）。
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// LoggingConfig 运行日志参数（3.7 / 4.5 / 第 10 章）。
type LoggingConfig struct {
	Path          string `yaml:"path"`
	FallbackDir   string `yaml:"fallback_dir"`
	Level         string `yaml:"level"`
	RetentionDays int    `yaml:"retention_days"`
	MaxEntries    int    `yaml:"max_entries"`
	// FrontendUploadEnabled 是否允许前端日志上报（F-41，默认关闭）。
	FrontendUploadEnabled bool `yaml:"frontend_upload_enabled"`
}

// AuthConfig 认证与管理员初始化参数。
type AuthConfig struct {
	Username       string `yaml:"username"`
	InitPassword   string `yaml:"init_password"`
	JWTSecret      string `yaml:"jwt_secret"`
	AccessTTLMin   int    `yaml:"access_ttl_minutes"`
	RefreshTTLHour int    `yaml:"refresh_ttl_hours"`
	// 登录失败限流（B-05）：窗口内失败次数达到上限后锁定。
	LoginMaxAttempts   int `yaml:"login_max_attempts"`
	LoginWindowMinutes int `yaml:"login_window_minutes"`
	LoginLockMinutes   int `yaml:"login_lock_minutes"`
}

// AppConfig 应用级参数。
type AppConfig struct {
	Timezone         string `yaml:"timezone"`
	RegistrationOpen *bool  `yaml:"registration_open"`
	// DataDir 数据库、日志等运行数据的默认落盘目录。
	DataDir string `yaml:"data_dir"`
	// WebDir 前端静态资源目录；为空时使用二进制内嵌资源。
	WebDir string `yaml:"web_dir"`
	// SourceID 媒体源标识，参与 video_key 计算（9.1）。
	SourceID string `yaml:"source_id"`
}

// Config 聚合配置。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Media    MediaConfig    `yaml:"media"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`
	Auth     AuthConfig     `yaml:"auth"`
	App      AppConfig      `yaml:"app"`

	// ConfigFile 实际加载的配置文件路径（可能为空）。
	ConfigFile string `yaml:"-"`
	// Location 解析后的时区。
	Location *time.Location `yaml:"-"`
}

// Default 返回内置默认配置。
func Default() *Config {
	reg := true
	return &Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
		Media: MediaConfig{
			Roots:                []string{"./media"},
			VideoExtensions:      []string{".mp4", ".webm", ".mov", ".m4v", ".ogv"},
			ImageExtensions:      []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif"},
			ScanIntervalSeconds:  60,
			ImageDisplaySeconds:  5,
			IgnoredDirs:          []string{".git", "node_modules", "@eaDir", "#recycle", ".DS_Store"},
			ThumbnailEnabled:     true,
			ThumbnailWidth:       480,
			ThumbnailSeekSeconds: 1,
			WatchEnabled:         true,
			WatchDebounceSeconds: 2,
			WatchMaxDirs:         4096,
		},
		Database: DatabaseConfig{Path: "./data/lanreel.db"},
		Logging: LoggingConfig{
			Path:          "./data/logs.db",
			FallbackDir:   "./data/logs",
			Level:         "info",
			RetentionDays: 30,
			MaxEntries:    200000,
		},
		Auth: AuthConfig{
			Username:           "admin",
			AccessTTLMin:       120,
			RefreshTTLHour:     720,
			LoginMaxAttempts:   5,
			LoginWindowMinutes: 5,
			LoginLockMinutes:   5,
		},
		App: AppConfig{
			Timezone:         "Asia/Shanghai",
			RegistrationOpen: &reg,
			DataDir:          "./data",
			SourceID:         "local",
		},
	}
}

// Load 读取配置：默认值 → YAML → 环境变量，并做校验与归一化。
func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		path = os.Getenv("LANREEL_CONFIG")
	}
	if path == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			path = "config.yaml"
		}
	}
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
		}
		cfg.ConfigFile = path
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("LANREEL_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("LANREEL_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("LANREEL_PORT 非法: %w", err)
		}
		cfg.Server.Port = p
	}
	if v := os.Getenv("LANREEL_TLS_ENABLED"); v != "" {
		cfg.Server.TLS.Enabled = truthy(v)
	}
	if v := os.Getenv("LANREEL_TLS_CERT"); v != "" {
		cfg.Server.TLS.CertFile = v
	}
	if v := os.Getenv("LANREEL_TLS_KEY"); v != "" {
		cfg.Server.TLS.KeyFile = v
	}
	// 媒体根目录：LANREEL_MEDIA_ROOTS 支持逗号或分号分隔（Windows 路径含冒号，故不用冒号分隔）。
	if v := os.Getenv("LANREEL_MEDIA_ROOTS"); v != "" {
		cfg.Media.Roots = splitList(v)
	}
	if v := os.Getenv("LANREEL_MEDIA_ROOT"); v != "" {
		cfg.Media.Roots = []string{v}
	}
	if v := os.Getenv("LANREEL_FFMPEG_PATH"); v != "" {
		cfg.Media.FFmpegPath = v
	}
	if v := os.Getenv("LANREEL_WATCH_ENABLED"); v != "" {
		cfg.Media.WatchEnabled = truthy(v)
	}
	if v := os.Getenv("LANREEL_THUMBNAILS_ENABLED"); v != "" {
		cfg.Media.ThumbnailEnabled = truthy(v)
	}
	if v := os.Getenv("LANREEL_DATA_DIR"); v != "" {
		cfg.App.DataDir = v
		cfg.Database.Path = filepath.Join(v, "lanreel.db")
		cfg.Logging.Path = filepath.Join(v, "logs.db")
		cfg.Logging.FallbackDir = filepath.Join(v, "logs")
	}
	if v := os.Getenv("LANREEL_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("LANREEL_LOG_DB_PATH"); v != "" {
		cfg.Logging.Path = v
	}
	if v := os.Getenv("LANREEL_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LANREEL_LOG_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("LANREEL_LOG_RETENTION_DAYS 非法: %w", err)
		}
		cfg.Logging.RetentionDays = n
	}
	if v := os.Getenv("LANREEL_TIMEZONE"); v != "" {
		cfg.App.Timezone = v
	}
	if v := os.Getenv("LANREEL_WEB_DIR"); v != "" {
		cfg.App.WebDir = v
	}
	if v := os.Getenv("LANREEL_REGISTRATION_OPEN"); v != "" {
		b := truthy(v)
		cfg.App.RegistrationOpen = &b
	}
	if v := os.Getenv("LANREEL_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("LANREEL_ADMIN_USERNAME"); v != "" {
		cfg.Auth.Username = v
	}
	if v := os.Getenv("LANREEL_LOGIN_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("LANREEL_LOGIN_MAX_ATTEMPTS 非法: %w", err)
		}
		cfg.Auth.LoginMaxAttempts = n
	}
	if v := os.Getenv("LANREEL_LOGIN_LOCK_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("LANREEL_LOGIN_LOCK_MINUTES 非法: %w", err)
		}
		cfg.Auth.LoginLockMinutes = n
	}
	// N-17：管理员初始密码只从环境变量/配置文件读取，不硬编码在代码中。
	if v := os.Getenv("ADMIN_INIT_PASSWORD"); v != "" {
		cfg.Auth.InitPassword = v
	}
	if v := os.Getenv("GOMEMLIMIT"); v != "" {
		// 仅记录用途，实际由 Go 运行时读取；此处不修改。
		_ = v
	}
	return nil
}

func (c *Config) normalize() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("server.port 必须在 1-65535 之间")
	}
	if len(c.Media.Roots) == 0 {
		return errors.New("media.roots 至少需要配置一个媒体根目录")
	}
	for i, r := range c.Media.Roots {
		if strings.TrimSpace(r) == "" {
			return errors.New("media.roots 存在空目录")
		}
		c.Media.Roots[i] = filepath.Clean(r)
	}
	if c.Auth.LoginMaxAttempts <= 0 {
		c.Auth.LoginMaxAttempts = 5
	}
	if c.Auth.LoginWindowMinutes <= 0 {
		c.Auth.LoginWindowMinutes = 5
	}
	if c.Auth.LoginLockMinutes <= 0 {
		c.Auth.LoginLockMinutes = 5
	}
	if c.Media.ImageDisplaySeconds <= 0 {
		c.Media.ImageDisplaySeconds = 5
	}
	if c.Media.ThumbnailWidth <= 0 {
		c.Media.ThumbnailWidth = 480
	}
	if c.Media.ThumbnailSeekSeconds <= 0 {
		c.Media.ThumbnailSeekSeconds = 1
	}
	if c.Media.WatchDebounceSeconds <= 0 {
		c.Media.WatchDebounceSeconds = 2
	}
	if c.Media.WatchMaxDirs <= 0 {
		c.Media.WatchMaxDirs = 4096
	}
	if c.Media.ScanIntervalSeconds < 0 {
		c.Media.ScanIntervalSeconds = 0
	}
	if c.Logging.RetentionDays <= 0 {
		c.Logging.RetentionDays = 30
	}
	if c.Logging.MaxEntries <= 0 {
		c.Logging.MaxEntries = 200000
	}
	if c.Auth.AccessTTLMin <= 0 {
		c.Auth.AccessTTLMin = 120
	}
	if c.Auth.RefreshTTLHour <= 0 {
		c.Auth.RefreshTTLHour = 720
	}
	if c.Auth.Username == "" {
		c.Auth.Username = "admin"
	}
	if c.App.SourceID == "" {
		c.App.SourceID = "local"
	}
	if c.App.Timezone == "" {
		c.App.Timezone = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(c.App.Timezone)
	if err != nil {
		return fmt.Errorf("app.timezone 非法 (%s): %w", c.App.Timezone, err)
	}
	c.Location = loc
	if c.App.DataDir == "" {
		c.App.DataDir = filepath.Dir(c.Database.Path)
	}
	return nil
}

// EnsureDirs 创建配置依赖的目录。
func (c *Config) EnsureDirs() error {
	dirs := []string{
		filepath.Dir(c.Database.Path),
		filepath.Dir(c.Logging.Path),
		c.Logging.FallbackDir,
	}
	for _, d := range dirs {
		if d == "" || d == "." {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", d, err)
		}
	}
	return nil
}

// LoadOrCreateSecret 若未配置 JWT 密钥，则在数据目录生成并持久化一个随机密钥，
// 保证重启后已签发的 Token 仍然有效，同时避免硬编码。
func (c *Config) LoadOrCreateSecret(gen func(n int) string) error {
	if strings.TrimSpace(c.Auth.JWTSecret) != "" {
		return nil
	}
	path := filepath.Join(c.App.DataDir, "jwt.secret")
	if raw, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(raw))
		if len(s) >= 32 {
			c.Auth.JWTSecret = s
			return nil
		}
	}
	if err := os.MkdirAll(c.App.DataDir, 0o755); err != nil {
		return err
	}
	secret := gen(48)
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return fmt.Errorf("写入 JWT 密钥失败: %w", err)
	}
	c.Auth.JWTSecret = secret
	return nil
}

// AccessTTL 访问令牌有效期。
func (c *Config) AccessTTL() time.Duration {
	return time.Duration(c.Auth.AccessTTLMin) * time.Minute
}

// RefreshTTL 刷新令牌有效期。
func (c *Config) RefreshTTL() time.Duration {
	return time.Duration(c.Auth.RefreshTTLHour) * time.Hour
}

// RegistrationAllowed 默认注册开关（数据库 settings 中的值优先）。
func (c *Config) RegistrationAllowed() bool {
	if c.App.RegistrationOpen == nil {
		return true
	}
	return *c.App.RegistrationOpen
}

// LogLevel 归一化日志级别。
func (c *Config) LogLevel() string {
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "warning", "error":
		if strings.EqualFold(c.Logging.Level, "warning") {
			return "warn"
		}
		return strings.ToLower(c.Logging.Level)
	default:
		return "info"
	}
}

// Addr 监听地址。
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on", "enabled":
		return true
	}
	return false
}

func splitList(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
