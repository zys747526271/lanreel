package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type favoriteRequest struct {
	VideoKey string `json:"video_key" binding:"required"`
}

// handleListFavorites GET /api/v1/favorites
func (h *Handlers) handleListFavorites(c *gin.Context) {
	u := currentUser(c)
	items, err := h.fav.List(c.Request.Context(), u)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, items)
}

// handleAddFavorite POST /api/v1/favorites
func (h *Handlers) handleAddFavorite(c *gin.Context) {
	u := currentUser(c)
	var req favoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	item, err := h.fav.Add(c.Request.Context(), u, req.VideoKey)
	if err != nil {
		failService(c, err)
		return
	}
	created(c, item)
}

// handleRemoveFavorite DELETE /api/v1/favorites/:key
func (h *Handlers) handleRemoveFavorite(c *gin.Context) {
	u := currentUser(c)
	if err := h.fav.Remove(c.Request.Context(), u, c.Param("key")); err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"success": true})
}

// handleBatchRemoveFavorites POST /api/v1/favorites/batch-remove
func (h *Handlers) handleBatchRemoveFavorites(c *gin.Context) {
	var req struct {
		VideoKeys []string `json:"video_keys" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	n, err := h.fav.RemoveBatch(c.Request.Context(), currentUser(c), req.VideoKeys)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"removed": n})
}

// handleCleanMissingFavorites POST /api/v1/favorites/clean-missing
// 只清理文件已丢失的收藏，不触碰被管理员屏蔽但仍存在的条目。
func (h *Handlers) handleCleanMissingFavorites(c *gin.Context) {
	n, err := h.fav.CleanMissing(c.Request.Context(), currentUser(c))
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"removed": n})
}
