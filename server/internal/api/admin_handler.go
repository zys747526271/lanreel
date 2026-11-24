package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/repository"
	"github.com/zys747526271/lanreel/server/internal/service"
)

// handleAdminStats GET /api/v1/admin/stats
func (h *Handlers) handleAdminStats(c *gin.Context) {
	v, err := h.admin.Stats(c.Request.Context())
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, v)
}

// handleAdminScan POST /api/v1/admin/scan
func (h *Handlers) handleAdminScan(c *gin.Context) {
	snap, summary, err := h.scanner.Scan(c.Request.Context())
	if err != nil {
		fail(c, http.StatusConflict, "scan_busy", "已有扫描任务在执行")
		return
	}
	ok(c, gin.H{"scan": summary, "media_total": len(snap.Items)})
}

// ---------- 文件夹策略 ----------

// handleAdminFolders GET /api/v1/admin/folders
func (h *Handlers) handleAdminFolders(c *gin.Context) {
	list, err := h.admin.ListFolders(c.Request.Context())
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, list)
}

// handleAdminSetFolder PUT /api/v1/admin/folders
func (h *Handlers) handleAdminSetFolder(c *gin.Context) {
	var in service.FolderPolicyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	p, err := h.admin.SetFolderPolicy(c.Request.Context(), currentUser(c), in)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, p)
}

// handleAdminDeleteFolder DELETE /api/v1/admin/folders?folder_path=xxx
func (h *Handlers) handleAdminDeleteFolder(c *gin.Context) {
	path := c.Query("folder_path")
	if err := h.admin.DeleteFolderPolicy(c.Request.Context(), currentUser(c), path); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// ---------- 文件级策略 ----------

// handleAdminFiles GET /api/v1/admin/files
func (h *Handlers) handleAdminFiles(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 20)
	list, total, err := h.admin.ListFiles(c.Request.Context(), currentUser(c),
		c.Query("keyword"), c.Query("folder"), page, size)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, list, total, page, size)
}

// handleAdminFile GET /api/v1/admin/files/:key
func (h *Handlers) handleAdminFile(c *gin.Context) {
	v, err := h.admin.GetFile(c.Request.Context(), c.Param("key"))
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, v)
}

// handleAdminSetFile PUT /api/v1/admin/files/:key
func (h *Handlers) handleAdminSetFile(c *gin.Context) {
	var in service.FileOverrideInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	o, err := h.admin.SetFileOverride(c.Request.Context(), currentUser(c), c.Param("key"), in)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, o)
}

// handleAdminDeleteFile DELETE /api/v1/admin/files/:key
func (h *Handlers) handleAdminDeleteFile(c *gin.Context) {
	if err := h.admin.DeleteFileOverride(c.Request.Context(), currentUser(c), c.Param("key")); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// ---------- 用户管理 ----------

// handleAdminUsers GET /api/v1/admin/users
func (h *Handlers) handleAdminUsers(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 20)
	list, total, err := h.admin.ListUsers(c.Request.Context(), c.Query("keyword"), page, size)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, list, total, page, size)
}

// handleAdminUpdateUser PUT /api/v1/admin/users/:id
func (h *Handlers) handleAdminUpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "用户 ID 非法")
		return
	}
	var in service.UserUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	u, err := h.admin.UpdateUser(c.Request.Context(), currentUser(c), id, in)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, toUserPayload(u))
}

// handleAdminDeleteUser DELETE /api/v1/admin/users/:id
func (h *Handlers) handleAdminDeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "用户 ID 非法")
		return
	}
	if err := h.admin.DeleteUser(c.Request.Context(), currentUser(c), id); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// handleAdminUserFavorites GET /api/v1/admin/users/:id/favorites
func (h *Handlers) handleAdminUserFavorites(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "用户 ID 非法")
		return
	}
	list, err := h.admin.UserFavorites(c.Request.Context(), currentUser(c), id)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, list)
}

// handleAdminCleanFavorites POST /api/v1/admin/users/:id/favorites/clean
func (h *Handlers) handleAdminCleanFavorites(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "用户 ID 非法")
		return
	}
	n, err := h.admin.CleanUserFavorites(c.Request.Context(), currentUser(c), id)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"removed": n})
}

// handleAdminDeleteFavorite DELETE /api/v1/admin/favorites/:id
func (h *Handlers) handleAdminDeleteFavorite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "收藏 ID 非法")
		return
	}
	removed, err := h.store.DeleteFavoriteByID(c.Request.Context(), id)
	if err != nil {
		failService(c, err)
		return
	}
	h.log.Info(logging.CatBusiness, "admin.favorites", "管理员删除收藏条目", map[string]any{
		"admin_id": currentUser(c).ID, "favorite_id": id, "removed": removed,
	})
	ok(c, gin.H{"removed": removed})
}

// ---------- 审计与日志 ----------

// handleAdminAuditLogs GET /api/v1/admin/audit-logs
func (h *Handlers) handleAdminAuditLogs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 20)
	f := repository.AuditFilter{
		TargetType: c.Query("target_type"),
		Action:     c.Query("action"),
		Keyword:    c.Query("keyword"),
		AdminID:    queryInt64Ptr(c, "admin_id"),
		Page:       page,
		PageSize:   size,
	}
	if from := parseQueryTime(c.Query("from"), h.cfg.Location); from != nil {
		f.From = from
	}
	if to := parseQueryTime(c.Query("to"), h.cfg.Location); to != nil {
		f.To = to
	}
	list, total, err := h.admin.ListAuditLogs(c.Request.Context(), currentUser(c), f)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, list, total, page, size)
}

// handleAdminLogs GET /api/v1/admin/logs
func (h *Handlers) handleAdminLogs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 50)
	f := logging.QueryFilter{
		Category:  c.Query("category"),
		Level:     c.Query("level"),
		Keyword:   c.Query("keyword"),
		RequestID: c.Query("request_id"),
		UserID:    queryInt64Ptr(c, "user_id"),
		Page:      page,
		PageSize:  size,
	}
	if from := parseQueryTime(c.Query("from"), h.cfg.Location); from != nil {
		f.From = from
	}
	if to := parseQueryTime(c.Query("to"), h.cfg.Location); to != nil {
		f.To = to
	}
	list, total, err := h.admin.QueryLogs(c.Request.Context(), currentUser(c), f)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, list, total, page, size)
}

// handleAdminLogDetail GET /api/v1/admin/logs/:id
func (h *Handlers) handleAdminLogDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "日志 ID 非法")
		return
	}
	e, err := h.admin.LogDetail(c.Request.Context(), id)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, e)
}

// handleAdminPurgeLogs POST /api/v1/admin/logs/purge
func (h *Handlers) handleAdminPurgeLogs(c *gin.Context) {
	n, err := h.admin.PurgeLogs(c.Request.Context(), currentUser(c))
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"removed": n})
}

// ---------- 设置 ----------

// handleGetLoggingSettings GET /api/v1/admin/settings/logging
func (h *Handlers) handleGetLoggingSettings(c *gin.Context) {
	ok(c, h.admin.LoggingSettings(c.Request.Context()))
}

// handleSetLoggingSettings PUT /api/v1/admin/settings/logging
func (h *Handlers) handleSetLoggingSettings(c *gin.Context) {
	var in service.LoggingSettings
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	v, err := h.admin.UpdateLoggingSettings(c.Request.Context(), currentUser(c), in)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, v)
}

// handleSetRegistration PUT /api/v1/admin/settings/registration
func (h *Handlers) handleSetRegistration(c *gin.Context) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	if err := h.admin.SetRegistration(c.Request.Context(), currentUser(c), in.Enabled); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"registration_open": in.Enabled})
}

// handleClientLogs POST /api/v1/logs/client
// 前端日志上报（F-41，默认关闭）。
func (h *Handlers) handleClientLogs(c *gin.Context) {
	settings := h.admin.LoggingSettings(c.Request.Context())
	if !settings.FrontendUpload {
		fail(c, http.StatusForbidden, "upload_disabled", "前端日志上报未开启")
		return
	}
	var in struct {
		Entries []struct {
			TS          string         `json:"ts"`
			Level       string         `json:"level"`
			Category    string         `json:"category"`
			Module      string         `json:"module"`
			Message     string         `json:"message"`
			Context     map[string]any `json:"context"`
			RequestID   string         `json:"request_id"`
		} `json:"entries"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	if len(in.Entries) > 200 {
		in.Entries = in.Entries[:200]
	}
	u := currentUser(c)
	for _, e := range in.Entries {
		fields := e.Context
		if fields == nil {
			fields = map[string]any{}
		}
		fields["request_id"] = e.RequestID
		fields["user_id"] = u.ID
		fields["origin"] = "frontend"
		cat := e.Category
		if cat == "" {
			cat = logging.CatFrontend
		}
		// 统一落入 frontend 分类，保留原始 category 便于区分（10.4 / F-41）。
		fields["frontend_category"] = cat
		h.log.Log(normalizeLevel(e.Level), logging.CatFrontend, "frontend."+e.Module, e.Message, fields)
	}
	ok(c, gin.H{"accepted": len(in.Entries)})
}

func normalizeLevel(v string) string {
	switch strings.ToLower(v) {
	case "debug":
		return logging.LevelDebug
	case "warn", "warning":
		return logging.LevelWarn
	case "error":
		return logging.LevelError
	default:
		return logging.LevelInfo
	}
}

func parseQueryTime(v string, loc *time.Location) *time.Time {
	if v == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return &t
		}
	}
	return nil
}

var _ = models.RoleAdmin
