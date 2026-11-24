// Package reqctx 在请求上下文中传递请求 ID 与当前用户，便于日志串联（N-29 / 10.4）。
package reqctx

import (
	"context"

	"github.com/zys747526271/lanreel/server/internal/models"
)

type key int

const (
	keyRequestID key = iota
	keyUser
	keyClientIP
)

// WithRequestID 写入请求 ID。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID 读取请求 ID。
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(keyRequestID).(string); ok {
		return v
	}
	return ""
}

// WithUser 写入当前用户。
func WithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, keyUser, u)
}

// User 读取当前用户，未登录返回 nil。
func User(ctx context.Context) *models.User {
	if v, ok := ctx.Value(keyUser).(*models.User); ok {
		return v
	}
	return nil
}

// WithClientIP 写入客户端 IP。
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, keyClientIP, ip)
}

// ClientIP 读取客户端 IP。
func ClientIP(ctx context.Context) string {
	if v, ok := ctx.Value(keyClientIP).(string); ok {
		return v
	}
	return ""
}

// Fields 生成日志上下文字段（request_id / user_id 会被提取为独立列）。
func Fields(ctx context.Context) map[string]any {
	f := map[string]any{}
	if id := RequestID(ctx); id != "" {
		f["request_id"] = id
	}
	if u := User(ctx); u != nil {
		f["user_id"] = u.ID
	}
	return f
}
