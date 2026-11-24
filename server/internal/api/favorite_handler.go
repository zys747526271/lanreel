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
