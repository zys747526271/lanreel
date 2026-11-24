package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/repository"
	"github.com/zys747526271/lanreel/server/internal/service"
)

// API 统一响应结构（9.2：新增字段不破坏旧客户端）。
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"data": data})
}

func paginated(c *gin.Context, items any, total, page, size int) {
	if page <= 0 {
		page = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"meta": gin.H{"total": total, "page": page, "page_size": size},
	})
}

func fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": apiError{Code: code, Message: message}})
}

// failService 将业务错误映射为 HTTP 响应。
func failService(c *gin.Context, err error) {
	var lockErr *service.TooManyAttemptsError
	if errors.As(err, &lockErr) {
		sec := int(lockErr.RetryAfter.Seconds())
		if sec < 1 {
			sec = 1
		}
		c.Header("Retry-After", strconv.Itoa(sec))
		fail(c, http.StatusTooManyRequests, "too_many_attempts", lockErr.Error())
		return
	}
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		fail(c, http.StatusUnauthorized, "invalid_credentials", err.Error())
	case errors.Is(err, service.ErrUserDisabled):
		fail(c, http.StatusForbidden, "user_disabled", err.Error())
	case errors.Is(err, service.ErrRegistrationClosed):
		fail(c, http.StatusForbidden, "registration_closed", err.Error())
	case errors.Is(err, service.ErrUsernameTaken):
		fail(c, http.StatusConflict, "username_taken", err.Error())
	case errors.Is(err, service.ErrSetupCompleted):
		fail(c, http.StatusConflict, "setup_completed", err.Error())
	case errors.Is(err, service.ErrValidation):
		fail(c, http.StatusBadRequest, "validation_failed", err.Error())
	case errors.Is(err, service.ErrMediaNotFound):
		fail(c, http.StatusNotFound, "media_not_found", err.Error())
	case errors.Is(err, repository.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "记录不存在")
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "服务内部错误")
	}
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func queryInt64Ptr(c *gin.Context, key string) *int64 {
	v := c.Query(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "req-fallback"
	}
	return hex.EncodeToString(b)
}
