package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zys747526271/lanreel/server/internal/auth"
	"github.com/zys747526271/lanreel/server/internal/config"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/repository"
	"github.com/zys747526271/lanreel/server/internal/reqctx"
)

// 业务错误（由 API 层映射为 HTTP 状态码）。
var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserDisabled       = errors.New("账号已被禁用")
	ErrRegistrationClosed = errors.New("注册入口已关闭")
	ErrUsernameTaken      = errors.New("用户名已存在")
	ErrValidation         = errors.New("参数校验失败")
	ErrSetupCompleted     = errors.New("实例已完成初始化，无法重复创建初始管理员")
)

// SettingRegistrationOpen 注册开关设置键。
const SettingRegistrationOpen = "registration_open"

// SettingSetupCompleted 首次初始化完成标志（管理员已创建）。
const SettingSetupCompleted = "setup_completed"

// AuthService 认证与账号业务。
type AuthService struct {
	store  *repository.Store
	tokens *auth.Manager
	log    *logging.Logger
	cfg    *config.Config
	// setupMu 串行化首次初始化，避免并发请求创建出多个管理员。
	setupMu sync.Mutex
}

// NewAuthService 构造认证服务。
func NewAuthService(store *repository.Store, tokens *auth.Manager, logger *logging.Logger, cfg *config.Config) *AuthService {
	return &AuthService{store: store, tokens: tokens, log: logger, cfg: cfg}
}

// RegistrationOpen 读取注册开关（数据库设置优先于配置文件）。
func (s *AuthService) RegistrationOpen(ctx context.Context) bool {
	def := "0"
	if s.cfg.RegistrationAllowed() {
		def = "1"
	}
	return s.store.GetSetting(ctx, SettingRegistrationOpen, def) == "1"
}

// SetRegistrationOpen 设置注册开关（F-11 / F-30）。
func (s *AuthService) SetRegistrationOpen(ctx context.Context, open bool) error {
	v := "0"
	if open {
		v = "1"
	}
	return s.store.SetSetting(ctx, SettingRegistrationOpen, v)
}

// Register 普通用户注册。
func (s *AuthService) Register(ctx context.Context, username, password string) (*models.User, *auth.TokenPair, error) {
	username = strings.TrimSpace(username)
	if err := validateCredentials(username, password); err != nil {
		return nil, nil, err
	}
	if !s.RegistrationOpen(ctx) {
		s.log.Warn(logging.CatAuth, "api.auth", "注册被拒绝：注册入口已关闭", withCtx(ctx, map[string]any{"username": username}))
		return nil, nil, ErrRegistrationClosed
	}
	if _, err := s.store.GetUserByUsername(ctx, username); err == nil {
		return nil, nil, ErrUsernameTaken
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, nil, err
	}
	u, err := s.store.CreateUser(ctx, username, hash, models.RoleUser, true)
	if err != nil {
		if strings.Contains(err.Error(), "已存在") {
			return nil, nil, ErrUsernameTaken
		}
		return nil, nil, err
	}
	pair, err := s.tokens.Issue(u)
	if err != nil {
		return nil, nil, err
	}
	s.log.Info(logging.CatAuth, "api.auth", "用户注册成功", withCtx(ctx, map[string]any{"username": username, "new_user_id": u.ID}))
	return u, pair, nil
}

// Login 用户登录。
func (s *AuthService) Login(ctx context.Context, username, password string) (*models.User, *auth.TokenPair, error) {
	username = strings.TrimSpace(username)
	u, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		s.log.Warn(logging.CatAuth, "api.auth", "登录失败：用户不存在", withCtx(ctx, map[string]any{"username": username}))
		return nil, nil, ErrInvalidCredentials
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		s.log.Warn(logging.CatAuth, "api.auth", "登录失败：密码错误", withCtx(ctx, map[string]any{"username": username}))
		return nil, nil, ErrInvalidCredentials
	}
	if !u.Enabled {
		s.log.Warn(logging.CatAuth, "api.auth", "登录失败：账号被禁用", withCtx(ctx, map[string]any{"username": username, "user_id": u.ID}))
		return nil, nil, ErrUserDisabled
	}
	pair, err := s.tokens.Issue(u)
	if err != nil {
		return nil, nil, err
	}
	s.log.Info(logging.CatAuth, "api.auth", "登录成功", withCtx(ctx, map[string]any{"username": username, "user_id": u.ID, "role": u.Role}))
	return u, pair, nil
}

// Refresh 用刷新令牌换取新的令牌对（F-09）。
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*models.User, *auth.TokenPair, error) {
	claims, err := s.tokens.Parse(refreshToken, auth.TokenRefresh)
	if err != nil {
		return nil, nil, ErrInvalidCredentials
	}
	u, err := s.store.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, nil, ErrInvalidCredentials
	}
	if !u.Enabled {
		return nil, nil, ErrUserDisabled
	}
	pair, err := s.tokens.Issue(u)
	if err != nil {
		return nil, nil, err
	}
	s.log.Debug(logging.CatAuth, "api.auth", "刷新令牌", withCtx(ctx, map[string]any{"user_id": u.ID}))
	return u, pair, nil
}

// Authenticate 校验访问令牌并返回用户（供中间件使用）。
func (s *AuthService) Authenticate(ctx context.Context, token string) (*models.User, error) {
	claims, err := s.tokens.Parse(token, auth.TokenAccess)
	if err != nil {
		return nil, err
	}
	u, err := s.store.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}
	if !u.Enabled {
		return nil, ErrUserDisabled
	}
	return u, nil
}

// Logout 记录登出行为；客户端负责清理本地缓存（F-10 / N-09）。
func (s *AuthService) Logout(ctx context.Context, u *models.User) {
	s.log.Info(logging.CatAuth, "api.auth", "用户登出", withCtx(ctx, map[string]any{"user_id": u.ID, "username": u.Username}))
}

// SetupRequired 判断实例是否仍需完成首次初始化（网页引导创建管理员）。
//
// 判定以 setup_completed 标志为准，并在缺省时用「是否已有启用的管理员」兜底，
// 这样从旧版本升级上来的实例不会被要求重新初始化。
func (s *AuthService) SetupRequired(ctx context.Context) bool {
	if s.store.GetSetting(ctx, SettingSetupCompleted, "") == "1" {
		return false
	}
	if n, err := s.store.CountAdmins(ctx); err == nil && n > 0 {
		return false
	}
	return true
}

// markSetupCompleted 写入初始化完成标志（失败只记录，不影响启动）。
func (s *AuthService) markSetupCompleted(ctx context.Context) {
	if s.store.GetSetting(ctx, SettingSetupCompleted, "") == "1" {
		return
	}
	if err := s.store.SetSetting(ctx, SettingSetupCompleted, "1"); err != nil {
		s.log.Warn(logging.CatSystem, "app.bootstrap", "写入初始化标志失败", map[string]any{"error": err})
	}
}

// CreateInitialAdmin 首次初始化：创建管理员账号并直接签发令牌（自动登录）。
//
// 仅在实例尚未初始化时可用；并发调用由 setupMu 串行化。
func (s *AuthService) CreateInitialAdmin(ctx context.Context, username, password string) (*models.User, *auth.TokenPair, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()

	if !s.SetupRequired(ctx) {
		return nil, nil, ErrSetupCompleted
	}
	username = strings.TrimSpace(username)
	if err := validateCredentials(username, password); err != nil {
		return nil, nil, err
	}
	if _, err := s.store.GetUserByUsername(ctx, username); err == nil {
		return nil, nil, ErrUsernameTaken
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, nil, err
	}
	u, err := s.store.CreateUser(ctx, username, hash, models.RoleAdmin, true)
	if err != nil {
		if strings.Contains(err.Error(), "已存在") {
			return nil, nil, ErrUsernameTaken
		}
		return nil, nil, err
	}
	// 初始化完成标志先落库，避免并发窗口内被重复初始化。
	if err := s.store.SetSetting(ctx, SettingSetupCompleted, "1"); err != nil {
		return nil, nil, err
	}
	pair, err := s.tokens.Issue(u)
	if err != nil {
		return nil, nil, err
	}

	after, _ := json.Marshal(map[string]any{"username": u.Username, "role": u.Role})
	if err := s.store.InsertAudit(ctx, models.AuditLog{
		AdminUserID: u.ID, TargetType: models.AuditTargetUser, TargetID: fmt.Sprint(u.ID),
		Action: "首次部署创建管理员", AfterValue: string(after),
	}); err != nil {
		s.log.Warn(logging.CatSystem, "app.setup", "写入初始化审计失败", withCtx(ctx, map[string]any{"error": err}))
	}
	s.log.Info(logging.CatSystem, "app.setup", "首次初始化完成：已创建管理员", withCtx(ctx, map[string]any{
		"username": u.Username, "user_id": u.ID,
	}))
	return u, pair, nil
}

// InitializeAdmin 启动时校正管理员账号。
//
// 行为约定（与首次部署网页引导配合）：
//   - 已有管理员：保证角色/状态正确，resetPassword 时用配置密码覆盖，并标记初始化完成；
//   - 全新实例且显式提供了初始密码（ADMIN_INIT_PASSWORD）：按配置创建，便于无浏览器部署；
//   - 全新实例且未提供密码：不创建任何账号，留给网页引导创建，避免默认弱口令。
func (s *AuthService) InitializeAdmin(ctx context.Context, resetPassword bool) error {
	username := strings.TrimSpace(s.cfg.Auth.Username)
	if username == "" {
		username = "admin"
	}
	initPassword := strings.TrimSpace(s.cfg.Auth.InitPassword)

	existing, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, repository.ErrNotFound) {
		existing = nil
	} else if err != nil {
		return err
	}

	if existing != nil {
		role := models.RoleAdmin
		enabled := true
		if existing.Role != models.RoleAdmin || !existing.Enabled {
			if err := s.store.UpdateUser(ctx, existing.ID, nil, &role, &enabled); err != nil {
				return err
			}
			s.log.Info(logging.CatSystem, "app.bootstrap", "已校正管理员账号角色/状态", map[string]any{"username": username})
		}
		if resetPassword {
			if initPassword == "" {
				s.log.Warn(logging.CatSystem, "app.bootstrap", "请求重置管理员密码但未提供初始密码", map[string]any{"username": username})
			} else {
				hash, herr := auth.HashPassword(initPassword)
				if herr != nil {
					return herr
				}
				if err := s.store.UpdateUser(ctx, existing.ID, &hash, nil, nil); err != nil {
					return err
				}
				s.log.Info(logging.CatSystem, "app.bootstrap", "已重置管理员密码（来自启动配置）", map[string]any{"username": username})
			}
		}
		s.markSetupCompleted(ctx)
		return nil
	}

	// 同名账号不存在：库里已有其他管理员说明是既有实例，无需再引导初始化。
	// 此时 --reset-admin-password 兜底作用于最早创建的管理员，因为网页初始化的
	// 用户名可能与 auth.username 不同。
	if n, cerr := s.store.CountAdmins(ctx); cerr == nil && n > 0 {
		if resetPassword && initPassword != "" {
			if err := s.resetFirstAdminPassword(ctx, initPassword); err != nil {
				return err
			}
		}
		s.markSetupCompleted(ctx)
		return nil
	}

	if initPassword == "" {
		s.log.Info(logging.CatSystem, "app.bootstrap", "实例尚未初始化，等待通过网页创建管理员", nil)
		return nil
	}
	hash, err := auth.HashPassword(initPassword)
	if err != nil {
		return err
	}
	u, err := s.store.CreateUser(ctx, username, hash, models.RoleAdmin, true)
	if err != nil {
		return err
	}
	s.markSetupCompleted(ctx)
	s.log.Info(logging.CatSystem, "app.bootstrap", "已按启动配置创建管理员账号", map[string]any{
		"username": username, "user_id": u.ID,
	})
	return nil
}

// resetFirstAdminPassword 重置最早创建的管理员密码（无浏览器时的密码恢复路径）。
func (s *AuthService) resetFirstAdminPassword(ctx context.Context, password string) error {
	u, err := s.store.FirstAdmin(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.store.UpdateUser(ctx, u.ID, &hash, nil, nil); err != nil {
		return err
	}
	s.log.Info(logging.CatSystem, "app.bootstrap", "已重置管理员密码（来自启动配置）", map[string]any{
		"username": u.Username, "user_id": u.ID,
	})
	return nil
}

func validateCredentials(username, password string) error {
	if len(username) < 3 || len(username) > 32 {
		return fmt.Errorf("%w: 用户名长度需为 3-32 个字符", ErrValidation)
	}
	for _, r := range username {
		if !(r == '_' || r == '-' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return fmt.Errorf("%w: 用户名只能包含字母、数字、下划线、短横线、点", ErrValidation)
		}
	}
	if len(password) < 6 || len(password) > 72 {
		return fmt.Errorf("%w: 密码长度需为 6-72 个字符", ErrValidation)
	}
	return nil
}

// withCtx 合并请求上下文字段与业务字段。
func withCtx(ctx context.Context, fields map[string]any) map[string]any {
	out := reqctx.Fields(ctx)
	if ip := reqctx.ClientIP(ctx); ip != "" {
		out["ip"] = ip
	}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

var _ = time.Now
