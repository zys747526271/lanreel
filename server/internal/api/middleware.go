package api

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/auth"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/reqctx"
	"github.com/zys747526271/lanreel/server/internal/service"
)

// middleware 生成请求级中间件。
func (h *Handlers) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" || len(id) > 128 {
			id = randomHex(12)
		}
		c.Writer.Header().Set("X-Request-ID", id)
		ctx := reqctx.WithRequestID(c.Request.Context(), id)
		ctx = reqctx.WithClientIP(ctx, c.ClientIP())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// recovery 捕获 panic 并记录 error 分类日志（10.2）。
func (h *Handlers) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				if len(stack) > 4096 {
					stack = stack[:4096]
				}
				h.log.Error(logging.CatError, "api.recovery", "请求处理发生 panic", map[string]any{
					"request_id": reqctx.RequestID(c.Request.Context()),
					"path":       c.FullPath(),
					"error":      fmt.Sprint(r),
					"stack":      stack,
				})
				fail(c, http.StatusInternalServerError, "internal_error", "服务内部错误")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// accessLog 记录 HTTP 访问摘要（category=access）。
// 媒体流请求按 debug 级别记录，避免播放时 Range 请求刷满日志库。
func (h *Handlers) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		fields := map[string]any{
			"request_id": reqctx.RequestID(c.Request.Context()),
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": latency.Milliseconds(),
			"ip":         c.ClientIP(),
			"bytes":      c.Writer.Size(),
		}
		if u := reqctx.User(c.Request.Context()); u != nil {
			fields["user_id"] = u.ID
		}
		level := logging.LevelInfo
		if strings.HasSuffix(c.Request.URL.Path, "/stream") {
			level = logging.LevelDebug
		}
		// 消息体带方法与路径，便于在日志列表直接定位（结构化字段仍在上下文中）。
		msg := c.Request.Method + " " + c.Request.URL.Path + " " + strconv.Itoa(c.Writer.Status())
		h.log.Log(level, logging.CatAccess, "api.access", msg, fields)
	}
}

// cors 允许局域网内前端开发服务器跨域调试。
func (h *Handlers) cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// extractToken 从 Authorization 头或 token 查询参数中读取访问令牌。
// 媒体流由 <video>/<img> 直接发起，无法附加自定义头，故支持 query token（N-14）。
func extractToken(c *gin.Context) string {
	if v := c.GetHeader("Authorization"); v != "" {
		if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
			return strings.TrimSpace(v[7:])
		}
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(c.Query("token"))
}

// requireAuth 鉴权中间件。
func (h *Handlers) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			fail(c, http.StatusUnauthorized, "unauthorized", "请先登录")
			c.Abort()
			return
		}
		u, err := h.auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			code := "unauthorized"
			msg := "登录状态已失效，请重新登录"
			if errors.Is(err, service.ErrUserDisabled) {
				code, msg = "user_disabled", "账号已被禁用"
			}
			h.log.Warn(logging.CatAuth, "api.auth", "鉴权失败", map[string]any{
				"request_id": reqctx.RequestID(c.Request.Context()),
				"path":       c.Request.URL.Path,
				"reason":     msg,
			})
			fail(c, http.StatusUnauthorized, code, msg)
			c.Abort()
			return
		}
		ctx := reqctx.WithUser(c.Request.Context(), u)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// requireAdmin 管理员权限中间件（N-16）。
func (h *Handlers) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil || !u.IsAdmin() {
			h.log.Warn(logging.CatAuth, "api.auth", "管理员接口权限拒绝", map[string]any{
				"request_id": reqctx.RequestID(c.Request.Context()),
				"path":       c.Request.URL.Path,
				"user_id":    userIDOrZero(u),
			})
			fail(c, http.StatusForbidden, "forbidden", "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// publicConfig 无需登录即可读取的配置。
func (h *Handlers) publicConfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		ok(c, gin.H{
			"app":                    "lanreel",
			"version":                h.version,
			"registration_open":      h.auth.RegistrationOpen(c.Request.Context()),
			"image_display_seconds":  h.cfg.Media.ImageDisplaySeconds,
			"timezone":               h.cfg.App.Timezone,
			"max_random_limit":       20,
			"scan_interval_seconds":  h.cfg.Media.ScanIntervalSeconds,
			"frontend_log_upload":    h.admin.LoggingSettings(c.Request.Context()).FrontendUpload,
		})
	}
}

func currentUser(c *gin.Context) *models.User {
	return reqctx.User(c.Request.Context())
}

func userIDOrZero(u *models.User) int64 {
	if u == nil {
		return 0
	}
	return u.ID
}

var _ = auth.TokenAccess
