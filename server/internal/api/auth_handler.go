package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/service"
)

type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type userPayload struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

func toUserPayload(u *models.User) userPayload {
	return userPayload{
		ID: u.ID, Username: u.Username, Role: u.Role, Enabled: u.Enabled,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// handleRegister POST /api/v1/auth/register
func (h *Handlers) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	u, pair, err := h.auth.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		failService(c, err)
		return
	}
	created(c, gin.H{"user": toUserPayload(u), "tokens": pair})
}

// handleLogin POST /api/v1/auth/login
func (h *Handlers) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	u, pair, err := h.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"user": toUserPayload(u), "tokens": pair})
}

// handleRefresh POST /api/v1/auth/refresh
func (h *Handlers) handleRefresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	u, pair, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"user": toUserPayload(u), "tokens": pair})
}

// handleLogout POST /api/v1/auth/logout
func (h *Handlers) handleLogout(c *gin.Context) {
	if u := currentUser(c); u != nil {
		h.auth.Logout(c.Request.Context(), u)
	}
	ok(c, gin.H{"success": true})
}

// handleMe GET /api/v1/auth/me
func (h *Handlers) handleMe(c *gin.Context) {
	u := currentUser(c)
	if u == nil {
		fail(c, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}
	ok(c, gin.H{"user": toUserPayload(u), "registration_open": h.auth.RegistrationOpen(c.Request.Context())})
}

var _ = service.SettingRegistrationOpen
