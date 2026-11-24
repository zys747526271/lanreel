package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// setupAdminRequest 首次初始化请求体。
type setupAdminRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// handleSetupStatus GET /api/v1/setup/status
//
// 前端在首个路由守卫里调用：required=true 时强制进入初始化页面。
// 已初始化后该接口恒返回 false，后续每次启动直接进入主页。
func (h *Handlers) handleSetupStatus(c *gin.Context) {
	ok(c, gin.H{
		"setup_required": h.auth.SetupRequired(c.Request.Context()),
		"version":        h.version,
	})
}

// handleSetupAdmin POST /api/v1/setup/admin
//
// 仅在未初始化时可用；创建成功后自动签发令牌，前端直接进入主页。
func (h *Handlers) handleSetupAdmin(c *gin.Context) {
	var req setupAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	u, pair, err := h.auth.CreateInitialAdmin(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		failService(c, err)
		return
	}
	created(c, gin.H{"user": toUserPayload(u), "tokens": pair})
}
