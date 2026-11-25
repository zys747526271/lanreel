package api

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/config"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/repository"
	"github.com/zys747526271/lanreel/server/internal/service"
	"github.com/zys747526271/lanreel/server/internal/webui"
)

// Handlers 聚合所有 HTTP 依赖。
type Handlers struct {
	cfg       *config.Config
	auth      *service.AuthService
	media     *service.MediaService
	fav       *service.FavoriteService
	admin     *service.AdminService
	scanner   *media.Scanner
	store     *repository.Store
	log       *logging.Logger
	logStore  *logging.SQLiteStore
	watch     *service.WatchService
	version   string
	startedAt time.Time
}

// NewHandlers 构造 API 层。
func NewHandlers(cfg *config.Config, authSvc *service.AuthService, mediaSvc *service.MediaService,
	fav *service.FavoriteService, adminSvc *service.AdminService, scanner *media.Scanner,
	store *repository.Store, logger *logging.Logger, logStore *logging.SQLiteStore,
	watch *service.WatchService, version string) *Handlers {
	return &Handlers{
		cfg: cfg, auth: authSvc, media: mediaSvc, fav: fav, admin: adminSvc,
		scanner: scanner, store: store, log: logger, logStore: logStore,
		watch: watch, version: version, startedAt: time.Now(),
	}
}

// Engine 构建 Gin 路由。
func (h *Handlers) Engine() *gin.Engine {
	if h.cfg.LogLevel() != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.Use(h.requestID(), h.recovery(), h.cors(), h.accessLog())

	v1 := r.Group("/api/v1")
	v1.Use(h.bodyLimit())
	{
		v1.GET("/health", h.handleHealth)
		v1.GET("/config/public", h.publicConfig())

		// 可播放策略变更事件（v0.3.1 实时同步）：所有在线客户端共用同一全局版本号。
		v1.GET("/events/policy", h.handlePolicyEvents)
		v1.GET("/events/policy/version", h.handlePolicyVersion)

		// 首次部署引导：仅在未初始化时可用，创建第一个管理员
		v1.GET("/setup/status", h.handleSetupStatus)
		v1.POST("/setup/admin", h.handleSetupAdmin)

		v1.POST("/auth/register", h.handleRegister)
		v1.POST("/auth/login", h.handleLogin)
		v1.POST("/auth/refresh", h.handleRefresh)

		// 媒体浏览允许游客（未登录）访问，可见范围由策略的「游客可见」控制（CR-004）。
		viewer := v1.Group("")
		viewer.Use(h.optionalAuth())
		{
			viewer.GET("/media/random", h.handleRandomMedia)
			viewer.POST("/media/status", h.handleMediaStatus)
			viewer.GET("/media/:key/stream", h.handleStream)
			viewer.GET("/media/:key/thumb", h.handleThumb)
		}

		authed := v1.Group("")
		authed.Use(h.requireAuth())
		{
			authed.POST("/auth/logout", h.handleLogout)
			authed.GET("/auth/me", h.handleMe)

			authed.GET("/favorites", h.handleListFavorites)
			authed.POST("/favorites", h.handleAddFavorite)
			authed.POST("/favorites/batch-remove", h.handleBatchRemoveFavorites)
			authed.POST("/favorites/clean-missing", h.handleCleanMissingFavorites)
			authed.DELETE("/favorites/:key", h.handleRemoveFavorite)

			// 观看历史与进度（v0.3.0，服务端按用户隔离）。
			authed.GET("/history", h.handleListHistory)
			authed.DELETE("/history", h.handleClearHistory)
			authed.DELETE("/history/:key", h.handleRemoveHistory)
			authed.GET("/progress", h.handleGetProgress)
			authed.POST("/progress", h.handleReportProgress)
			authed.POST("/progress/restart", h.handleRestartProgress)
			authed.GET("/me/watch-summary", h.handleMyWatchSummary)

			authed.POST("/logs/client", h.handleClientLogs)
		}

		admin := v1.Group("/admin")
		admin.Use(h.requireAuth(), h.requireAdmin())
		{
			admin.GET("/stats", h.handleAdminStats)
			admin.GET("/diagnostics", h.handleAdminDiagnostics)
			admin.POST("/scan", h.handleAdminScan)
			admin.POST("/reset", h.handleAdminReset)

			admin.GET("/folders", h.handleAdminFolders)
			admin.PUT("/folders", h.handleAdminSetFolder)
			admin.DELETE("/folders", h.handleAdminDeleteFolder)

			admin.GET("/files", h.handleAdminFiles)
			admin.GET("/files/:key", h.handleAdminFile)
			admin.PUT("/files/:key", h.handleAdminSetFile)
			admin.DELETE("/files/:key", h.handleAdminDeleteFile)

			admin.GET("/users", h.handleAdminUsers)
			admin.PUT("/users/:id", h.handleAdminUpdateUser)
			admin.DELETE("/users/:id", h.handleAdminDeleteUser)
			admin.GET("/users/:id/favorites", h.handleAdminUserFavorites)
			admin.POST("/users/:id/favorites/clean", h.handleAdminCleanFavorites)
			admin.DELETE("/favorites/:id", h.handleAdminDeleteFavorite)

			admin.GET("/audit-logs", h.handleAdminAuditLogs)
			admin.GET("/logs", h.handleAdminLogs)
			admin.GET("/logs/:id", h.handleAdminLogDetail)
			admin.POST("/logs/purge", h.handleAdminPurgeLogs)

			admin.GET("/settings/media", h.handleAdminMediaSettings)
			admin.PUT("/settings/media", h.handleAdminSetMediaRoots)
			admin.DELETE("/settings/media", h.handleAdminResetMediaRoots)
			admin.GET("/fs/dirs", h.handleAdminFSDirs)

			// 观看统计与热度（v0.3.0，供控制台与推荐算法）。
			admin.GET("/watch/heat/media", h.handleAdminMediaHeat)
			admin.GET("/watch/heat/folders", h.handleAdminFolderHeat)
			admin.GET("/watch/heat/users", h.handleAdminUserHeat)
			admin.GET("/recommend", h.handleRecommend)

			admin.GET("/settings/logging", h.handleGetLoggingSettings)
			admin.PUT("/settings/logging", h.handleSetLoggingSettings)
			admin.PUT("/settings/registration", h.handleSetRegistration)
		}
	}

	h.mountSPA(r)
	return r
}

// bodyLimit 限制 JSON 请求体大小，避免异常请求占用内存。
func (h *Handlers) bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil && c.Request.Method != http.MethodGet {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		}
		c.Next()
	}
}

// handleHealth GET /api/v1/health
func (h *Handlers) handleHealth(c *gin.Context) {
	dbOK := true
	if err := h.store.DB().PingContext(c.Request.Context()); err != nil {
		dbOK = false
	}
	snap := h.scanner.Snapshot()
	ok(c, gin.H{
		"status":      "ok",
		"version":     h.version,
		"uptime_sec":  int64(time.Since(h.startedAt).Seconds()),
		"media_total": len(snap.Items),
		"scanned_at":  snap.ScannedAt.Format(time.RFC3339),
		"db_ok":       dbOK,
		"log_dropped": h.log.Dropped(),
	})
}

// mountSPA 挂载前端单页应用：优先使用 app.web_dir，其次使用内嵌资源。
func (h *Handlers) mountSPA(r *gin.Engine) {
	var fsys fs.FS
	if h.cfg.App.WebDir != "" {
		if _, err := os.Stat(filepath.Join(h.cfg.App.WebDir, "index.html")); err == nil {
			fsys = os.DirFS(h.cfg.App.WebDir)
		}
	}
	if fsys == nil {
		if assets, ok := webui.Assets(); ok {
			fsys = assets
		}
	}

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			fail(c, http.StatusNotFound, "not_found", "接口不存在")
			return
		}
		if fsys == nil {
			c.String(http.StatusOK, `<!doctype html><meta charset="utf-8"><title>LanReel</title>
<h1>LanReel 服务已启动</h1>
<p>尚未找到前端构建产物。请先执行 <code>npm --prefix web run build</code>，
或使用 <code>LANREEL_WEB_DIR</code> 指向 <code>web/dist</code>。</p>
<p>API 健康检查：<a href="/api/v1/health">/api/v1/health</a></p>`)
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			fail(c, http.StatusMethodNotAllowed, "method_not_allowed", "方法不被支持")
			return
		}
		name := strings.TrimPrefix(path.Clean(p), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if serveAsset(c, fsys, name) {
			return
		}
		// SPA history 路由回退到 index.html
		if serveAsset(c, fsys, "index.html") {
			return
		}
		c.String(http.StatusNotFound, "资源不存在")
	})
}

func serveAsset(c *gin.Context, fsys fs.FS, name string) bool {
	if strings.Contains(name, "..") {
		return false
	}
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	}
	if strings.HasPrefix(name, "assets/") {
		c.Writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Writer.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(c.Writer, c.Request, name, st.ModTime(), rs)
	return true
}
