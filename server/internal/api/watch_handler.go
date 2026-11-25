package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/repository"
)

// handleListHistory GET /api/v1/history — 当前用户观看历史（服务端隔离）。
func (h *Handlers) handleListHistory(c *gin.Context) {
	u := currentUser(c)
	limit := queryInt(c, "limit", 100)
	list, err := h.watch.ListHistory(c.Request.Context(), u, limit)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, list)
}

// handleClearHistory DELETE /api/v1/history
func (h *Handlers) handleClearHistory(c *gin.Context) {
	if err := h.watch.ClearHistory(c.Request.Context(), currentUser(c)); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// handleRemoveHistory DELETE /api/v1/history/:key
func (h *Handlers) handleRemoveHistory(c *gin.Context) {
	if err := h.watch.RemoveHistory(c.Request.Context(), currentUser(c), c.Param("key")); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// handleGetProgress GET /api/v1/progress — 当前用户所有条目的进度快照。
func (h *Handlers) handleGetProgress(c *gin.Context) {
	m, err := h.watch.ProgressMap(c.Request.Context(), currentUser(c))
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, m)
}

// handleReportProgress POST /api/v1/progress — 上报观看进度。
func (h *Handlers) handleReportProgress(c *gin.Context) {
	var in repository.WatchProgressInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	if in.VideoKey == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "video_key 不能为空")
		return
	}
	if err := h.watch.ReportProgress(c.Request.Context(), currentUser(c), in); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// handleRestartProgress POST /api/v1/progress/restart — 从头开始播放（清零进度）。
func (h *Handlers) handleRestartProgress(c *gin.Context) {
	var req struct {
		VideoKey string `json:"video_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	if err := h.watch.RestartProgress(c.Request.Context(), currentUser(c), req.VideoKey); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// handleMyWatchSummary GET /api/v1/me/watch-summary — 当前用户观看统计。
func (h *Handlers) handleMyWatchSummary(c *gin.Context) {
	v, err := h.watch.UserSummary(c.Request.Context(), currentUser(c))
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, v)
}

// ---------- 管理端统计与推荐 ----------

// handleAdminMediaHeat GET /api/v1/admin/watch/heat/media
// 全量排序后再分页：sort=heat|play|complete|watched|favorite|name，order=asc|desc。
func (h *Handlers) handleAdminMediaHeat(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 10)
	hideMissing := queryBool(c, "hide_missing", true)
	pageData, err := h.watch.MediaHeatPaged(c.Request.Context(), c.Query("sort"), queryDesc(c, true), page, size, hideMissing)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, pageData.Items, pageData.Total, page, size)
}

// handleAdminFolderHeat GET /api/v1/admin/watch/heat/folders
func (h *Handlers) handleAdminFolderHeat(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 10)
	pageData, err := h.watch.FolderHeatPaged(c.Request.Context(), c.Query("sort"), queryDesc(c, true), page, size)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, pageData.Items, pageData.Total, page, size)
}

// handleAdminUserHeat GET /api/v1/admin/watch/heat/users — 用户观看排行。
func (h *Handlers) handleAdminUserHeat(c *gin.Context) {
	page := queryInt(c, "page", 1)
	size := queryInt(c, "page_size", 10)
	pageData, err := h.watch.UserHeatPaged(c.Request.Context(), c.Query("sort"), queryDesc(c, true), page, size)
	if err != nil {
		failService(c, err)
		return
	}
	paginated(c, pageData.Items, pageData.Total, page, size)
}

// handleRecommend GET /api/v1/admin/recommend — 推荐候选（预留接口，基于热度评分）。
// 未来可接入协同过滤 / 向量召回；当前以热度分为基线，返回可播放条目。
func (h *Handlers) handleRecommend(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	exclude := c.Query("exclude")
	list, err := h.watch.MediaHeat(c.Request.Context(), limit*2)
	if err != nil {
		failService(c, err)
		return
	}
	skip := map[string]bool{}
	if exclude != "" {
		for _, k := range splitComma(exclude) {
			skip[k] = true
		}
	}
	out := make([]gin.H, 0, limit)
	for _, v := range list {
		if skip[v.Key] {
			continue
		}
		out = append(out, gin.H{
			"key": v.Key, "score": v.HeatScore,
			"play_count": v.PlayCount, "favorite_count": v.FavoriteCount,
			"folder": v.Folder, "root_index": v.RootIndex,
		})
		if len(out) >= limit {
			break
		}
	}
	ok(c, out)
}

func splitComma(v string) []string {
	out := []string{}
	cur := ""
	for _, r := range v {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

var _ = http.StatusOK
var _ = strconv.Itoa
