package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zys747526271/lanreel/server/internal/auth"
	"github.com/zys747526271/lanreel/server/internal/config"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/policy"
	"github.com/zys747526271/lanreel/server/internal/repository"
)

// 设置键（日志相关，F-40 / F-41）。
const (
	SettingLogRetentionDays  = "log_retention_days"
	SettingLogMaxEntries     = "log_max_entries"
	SettingLogFrontendUpload = "log_frontend_upload_enabled"
	SettingLogLevel          = "log_level"
)

// AdminService 管理员业务。
type AdminService struct {
	store    *repository.Store
	media    *MediaService
	fav      *FavoriteService
	eval     *policy.Evaluator
	authSvc  *AuthService
	cfg      *config.Config
	log      *logging.Logger
	logStore *logging.SQLiteStore
}

// NewAdminService 构造管理员服务。
func NewAdminService(store *repository.Store, mediaSvc *MediaService, fav *FavoriteService,
	eval *policy.Evaluator, authSvc *AuthService, cfg *config.Config,
	logger *logging.Logger, logStore *logging.SQLiteStore) *AdminService {
	return &AdminService{store: store, media: mediaSvc, fav: fav, eval: eval,
		authSvc: authSvc, cfg: cfg, log: logger, logStore: logStore}
}

// audit 写入审计日志（F-29），同时生成运行日志（9.5 / 10.6）。
func (s *AdminService) audit(ctx context.Context, admin *models.User, targetType, targetID, action string, before, after any) {
	bf, _ := json.Marshal(before)
	af, _ := json.Marshal(after)
	rec := models.AuditLog{
		AdminUserID: admin.ID, TargetType: targetType, TargetID: targetID,
		Action: action, BeforeValue: string(bf), AfterValue: string(af),
	}
	if err := s.store.InsertAudit(ctx, rec); err != nil {
		s.log.Error(logging.CatError, "admin.audit", "写入审计日志失败", withCtx(ctx, map[string]any{"error": err, "action": action}))
	}
	fields := withCtx(ctx, map[string]any{
		"admin_id": admin.ID, "target_type": targetType, "target_id": targetID, "action": action,
		"before": string(bf), "after": string(af),
	})
	cat := logging.CatBusiness
	if targetType == models.AuditTargetFolder || targetType == models.AuditTargetFile {
		cat = logging.CatPolicy
	}
	s.log.Info(cat, "admin."+targetType, "管理员操作："+action, fields)
}

// StatsView 后台概览。
type StatsView struct {
	Users          int               `json:"users"`
	Admins         int               `json:"admins"`
	Favorites      int               `json:"favorites"`
	MediaTotal     int               `json:"media_total"`
	Videos         int               `json:"videos"`
	Images         int               `json:"images"`
	FolderPolicies int               `json:"folder_policies"`
	FileOverrides  int               `json:"file_overrides"`
	Scan           media.ScanSummary `json:"scan"`
	LogCounts      map[string]int    `json:"log_counts"`
	Registration   bool              `json:"registration_open"`
	Logging        LoggingSettings   `json:"logging"`
}

// LoggingSettings 日志配置（可热更新）。
type LoggingSettings struct {
	RetentionDays  int    `json:"retention_days"`
	MaxEntries     int    `json:"max_entries"`
	Level          string `json:"level"`
	FrontendUpload bool   `json:"frontend_upload_enabled"`
}

// Stats 汇总概览数据。
func (s *AdminService) Stats(ctx context.Context) (StatsView, error) {
	var v StatsView
	if n, err := s.store.CountUsers(ctx); err == nil {
		v.Users = n
	} else {
		return v, err
	}
	if n, err := s.store.CountAdmins(ctx); err == nil {
		v.Admins = n
	}
	if n, err := s.store.CountAllFavorites(ctx); err == nil {
		v.Favorites = n
	}
	if pols, err := s.store.ListFolderPolicies(ctx); err == nil {
		v.FolderPolicies = len(pols)
	}
	if n, err := s.store.CountFileOverrides(ctx); err == nil {
		v.FileOverrides = n
	}
	snap := s.media.Snapshot()
	v.MediaTotal = len(snap.Items)
	for _, it := range snap.Items {
		if it.Type == models.MediaVideo {
			v.Videos++
		} else {
			v.Images++
		}
	}
	v.Scan = s.media.scannerSummary()
	v.Registration = s.authSvc.RegistrationOpen(ctx)
	v.Logging = s.LoggingSettings(ctx)
	if s.logStore != nil {
		if m, err := s.logStore.CountByCategory(ctx); err == nil {
			v.LogCounts = m
		}
	}
	return v, nil
}

// LoggingSettings 读取日志配置。
func (s *AdminService) LoggingSettings(ctx context.Context) LoggingSettings {
	return LoggingSettings{
		RetentionDays:  atoiDefault(s.store.GetSetting(ctx, SettingLogRetentionDays, strconv.Itoa(s.cfg.Logging.RetentionDays)), s.cfg.Logging.RetentionDays),
		MaxEntries:     atoiDefault(s.store.GetSetting(ctx, SettingLogMaxEntries, strconv.Itoa(s.cfg.Logging.MaxEntries)), s.cfg.Logging.MaxEntries),
		Level:          s.store.GetSetting(ctx, SettingLogLevel, s.cfg.LogLevel()),
		FrontendUpload: s.store.GetSetting(ctx, SettingLogFrontendUpload, boolStr(s.cfg.Logging.FrontendUploadEnabled)) == "1",
	}
}

// UpdateLoggingSettings 更新日志配置并即时生效。
func (s *AdminService) UpdateLoggingSettings(ctx context.Context, admin *models.User, in LoggingSettings) (LoggingSettings, error) {
	before := s.LoggingSettings(ctx)
	if in.RetentionDays < 1 || in.RetentionDays > 3650 {
		return before, fmt.Errorf("%w: 日志保留天数需在 1-3650 之间", ErrValidation)
	}
	if in.MaxEntries < 1000 || in.MaxEntries > 5000000 {
		return before, fmt.Errorf("%w: 日志最大条数需在 1000-5000000 之间", ErrValidation)
	}
	switch in.Level {
	case "debug", "info", "warn", "error":
	default:
		return before, fmt.Errorf("%w: 日志级别非法", ErrValidation)
	}
	if err := s.store.SetSetting(ctx, SettingLogRetentionDays, strconv.Itoa(in.RetentionDays)); err != nil {
		return before, err
	}
	if err := s.store.SetSetting(ctx, SettingLogMaxEntries, strconv.Itoa(in.MaxEntries)); err != nil {
		return before, err
	}
	if err := s.store.SetSetting(ctx, SettingLogLevel, in.Level); err != nil {
		return before, err
	}
	if err := s.store.SetSetting(ctx, SettingLogFrontendUpload, boolStr(in.FrontendUpload)); err != nil {
		return before, err
	}
	s.log.SetLevel(in.Level)
	s.audit(ctx, admin, models.AuditTargetSetting, "logging", "更新日志配置", before, in)
	return s.LoggingSettings(ctx), nil
}

// UserView 后台用户视图。
type UserView struct {
	models.User
	FavoriteCount int `json:"favorite_count"`
}

// ListUsers 用户列表。
func (s *AdminService) ListUsers(ctx context.Context, keyword string, page, size int) ([]UserView, int, error) {
	users, total, err := s.store.ListUsers(ctx, repository.UserListFilter{Keyword: keyword, Page: page, PageSize: size})
	if err != nil {
		return nil, 0, err
	}
	out := make([]UserView, 0, len(users))
	for _, u := range users {
		n, _ := s.store.CountFavorites(ctx, u.ID)
		out = append(out, UserView{User: u, FavoriteCount: n})
	}
	return out, total, nil
}

// UserUpdate 用户变更请求。
type UserUpdate struct {
	Enabled  *bool   `json:"enabled"`
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

// UpdateUser 更新用户（禁用/启用、改角色、重置密码，F-25）。
func (s *AdminService) UpdateUser(ctx context.Context, admin *models.User, id int64, in UserUpdate) (*models.User, error) {
	target, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before := map[string]any{"enabled": target.Enabled, "role": target.Role}

	if in.Role != nil {
		if *in.Role != models.RoleAdmin && *in.Role != models.RoleUser {
			return nil, fmt.Errorf("%w: 角色非法", ErrValidation)
		}
		if target.ID == admin.ID && *in.Role != models.RoleAdmin {
			return nil, fmt.Errorf("%w: 不能取消自己的管理员角色", ErrValidation)
		}
		if target.Role == models.RoleAdmin && *in.Role == models.RoleUser {
			if n, _ := s.store.CountAdmins(ctx); n <= 1 {
				return nil, fmt.Errorf("%w: 至少需要保留一个启用的管理员", ErrValidation)
			}
		}
	}
	if in.Enabled != nil {
		if target.ID == admin.ID && !*in.Enabled {
			return nil, fmt.Errorf("%w: 不能禁用自己的账号", ErrValidation)
		}
		if target.Role == models.RoleAdmin && !*in.Enabled {
			if n, _ := s.store.CountAdmins(ctx); n <= 1 {
				return nil, fmt.Errorf("%w: 至少需要保留一个启用的管理员", ErrValidation)
			}
		}
	}
	var hash *string
	if in.Password != nil {
		if len(*in.Password) < 6 || len(*in.Password) > 72 {
			return nil, fmt.Errorf("%w: 密码长度需为 6-72 个字符", ErrValidation)
		}
		h, herr := auth.HashPassword(*in.Password)
		if herr != nil {
			return nil, herr
		}
		hash = &h
	}
	if err := s.store.UpdateUser(ctx, id, hash, in.Role, in.Enabled); err != nil {
		return nil, err
	}
	updated, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	after := map[string]any{"enabled": updated.Enabled, "role": updated.Role}
	if hash != nil {
		after["password"] = "reset"
	}
	s.audit(ctx, admin, models.AuditTargetUser, strconv.FormatInt(id, 10), "更新用户", before, after)
	if in.Password != nil {
		s.log.Info(logging.CatAuth, "admin.users", "管理员重置用户密码", withCtx(ctx, map[string]any{"target_user_id": id}))
	}
	return updated, nil
}

// DeleteUser 删除用户并级联清理收藏（F-26）。
func (s *AdminService) DeleteUser(ctx context.Context, admin *models.User, id int64) error {
	target, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if target.ID == admin.ID {
		return fmt.Errorf("%w: 不能删除自己的账号", ErrValidation)
	}
	if target.Role == models.RoleAdmin {
		if n, _ := s.store.CountAdmins(ctx); n <= 1 {
			return fmt.Errorf("%w: 至少需要保留一个启用的管理员", ErrValidation)
		}
	}
	if err := s.store.DeleteUser(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, admin, models.AuditTargetUser, strconv.FormatInt(id, 10), "删除用户",
		map[string]any{"username": target.Username, "role": target.Role}, nil)
	return nil
}

// UserFavorites 只读查看某用户收藏（F-27）。
func (s *AdminService) UserFavorites(ctx context.Context, admin *models.User, userID int64) ([]MediaView, error) {
	target, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, admin, models.AuditTargetUser, strconv.FormatInt(userID, 10), "查看用户收藏", nil,
		map[string]any{"username": target.Username})
	return s.fav.List(ctx, target)
}

// CleanUserFavorites 清理失效收藏（F-28）。
func (s *AdminService) CleanUserFavorites(ctx context.Context, admin *models.User, userID int64) (int, error) {
	n, err := s.fav.CleanInvalid(ctx, userID)
	if err != nil {
		return 0, err
	}
	s.audit(ctx, admin, models.AuditTargetUser, strconv.FormatInt(userID, 10), "清理失效收藏", nil, map[string]any{"removed": n})
	return n, nil
}

// FolderPolicyView 文件夹策略视图（含扫描统计）。
type FolderPolicyView struct {
	FolderPath    string     `json:"folder_path"`
	Total         int        `json:"total"`
	Videos        int        `json:"videos"`
	Images        int        `json:"images"`
	Enabled       bool       `json:"enabled"`
	ScheduleType  string     `json:"schedule_type"`
	TimeStart     string     `json:"time_start"`
	TimeEnd       string     `json:"time_end"`
	Configured    bool       `json:"configured"`
	Inherited     bool       `json:"inherited"`
	InheritedFrom string     `json:"inherited_from"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

// ListFolders 列出扫描到的文件夹及现有策略。
func (s *AdminService) ListFolders(ctx context.Context) ([]FolderPolicyView, error) {
	pols, err := s.store.ListFolderPolicies(ctx)
	if err != nil {
		return nil, err
	}
	pmap := map[string]models.FolderPolicy{}
	for _, p := range pols {
		pmap[media.NormalizeFolderPath(p.FolderPath)] = p
	}
	stats := s.media.Folders()
	out := make([]FolderPolicyView, 0, len(stats)+len(pols))
	seen := map[string]bool{}
	for _, st := range stats {
		v := FolderPolicyView{
			FolderPath: st.FolderPath, Total: st.Total, Videos: st.Videos, Images: st.Images,
			Enabled: true, ScheduleType: models.ScheduleAlways,
		}
		if p, ok := pmap[st.FolderPath]; ok {
			v.Enabled = p.Enabled
			v.ScheduleType = p.ScheduleType
			v.TimeStart = p.TimeStart
			v.TimeEnd = p.TimeEnd
			v.Configured = true
			t := p.UpdatedAt
			v.UpdatedAt = &t
		} else if p, ok := s.eval.MatchFolder(ctx, st.FolderPath); ok {
			// 未单独配置但被上级目录策略覆盖时，展示生效状态，避免后台误读。
			v.Enabled = p.Enabled
			v.ScheduleType = p.ScheduleType
			v.TimeStart = p.TimeStart
			v.TimeEnd = p.TimeEnd
			v.Inherited = true
			v.InheritedFrom = media.NormalizeFolderPath(p.FolderPath)
		}
		seen[st.FolderPath] = true
		out = append(out, v)
	}
	// 已配置但当前无文件的文件夹（例如临时删除后仍保留策略）也要可见。
	for _, p := range pols {
		path := media.NormalizeFolderPath(p.FolderPath)
		if seen[path] {
			continue
		}
		t := p.UpdatedAt
		out = append(out, FolderPolicyView{
			FolderPath: path, Enabled: p.Enabled, ScheduleType: p.ScheduleType,
			TimeStart: p.TimeStart, TimeEnd: p.TimeEnd, Configured: true, UpdatedAt: &t,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FolderPath < out[j].FolderPath })
	return out, nil
}

// FolderPolicyInput 文件夹策略输入。
type FolderPolicyInput struct {
	FolderPath   string `json:"folder_path"`
	Enabled      bool   `json:"enabled"`
	ScheduleType string `json:"schedule_type"`
	TimeStart    string `json:"time_start"`
	TimeEnd      string `json:"time_end"`
}

// SetFolderPolicy 设置文件夹策略（F-12 / F-14）。
func (s *AdminService) SetFolderPolicy(ctx context.Context, admin *models.User, in FolderPolicyInput) (*models.FolderPolicy, error) {
	in.FolderPath = repository.NormalizeFolder(in.FolderPath)
	switch in.ScheduleType {
	case "", models.ScheduleAlways:
		in.ScheduleType = models.ScheduleAlways
	case models.ScheduleTimeWindow, models.ScheduleDisabled:
	default:
		return nil, fmt.Errorf("%w: 调度类型非法", ErrValidation)
	}
	if in.ScheduleType == models.ScheduleTimeWindow {
		if !validHHMM(in.TimeStart) || !validHHMM(in.TimeEnd) {
			return nil, fmt.Errorf("%w: 时间窗口格式应为 HH:MM", ErrValidation)
		}
	}
	var before any
	if pols, err := s.store.ListFolderPolicies(ctx); err == nil {
		for _, p := range pols {
			if media.NormalizeFolderPath(p.FolderPath) == in.FolderPath {
				before = p
				break
			}
		}
	}
	p, err := s.store.UpsertFolderPolicy(ctx, models.FolderPolicy{
		FolderPath: in.FolderPath, Enabled: in.Enabled, ScheduleType: in.ScheduleType,
		TimeStart: in.TimeStart, TimeEnd: in.TimeEnd,
	})
	if err != nil {
		return nil, err
	}
	s.eval.Invalidate()
	s.audit(ctx, admin, models.AuditTargetFolder, in.FolderPath, "设置文件夹策略", before, p)
	return p, nil
}

// DeleteFolderPolicy 删除文件夹策略。
func (s *AdminService) DeleteFolderPolicy(ctx context.Context, admin *models.User, folderPath string) error {
	folderPath = repository.NormalizeFolder(folderPath)
	removed, err := s.store.DeleteFolderPolicy(ctx, folderPath)
	if err != nil {
		return err
	}
	if removed {
		s.eval.Invalidate()
		s.audit(ctx, admin, models.AuditTargetFolder, folderPath, "删除文件夹策略", nil, nil)
	}
	return nil
}

// FileView 文件策略视图。
type FileView struct {
	Key       string               `json:"key"`
	RelPath   string               `json:"rel_path"`
	MediaType string               `json:"media_type"`
	Folder    string               `json:"folder"`
	Duration  float64              `json:"duration"`
	Size      int64                `json:"size"`
	Playable  bool                 `json:"playable"`
	Reason    string               `json:"reason,omitempty"`
	Override  *models.FileOverride `json:"override,omitempty"`
}

// ListFiles 文件列表（支持关键字与文件夹过滤）。
func (s *AdminService) ListFiles(ctx context.Context, admin *models.User, keyword, folder string, page, size int) ([]FileView, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	folder = repository.NormalizeFolder(folder)
	overrides, err := s.store.ListFileOverrides(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := s.media.SearchItems(keyword, 0)
	if folder != "" {
		filtered := make([]*media.Item, 0, len(items))
		for _, it := range items {
			f := it.Folder()
			if f == folder || strings.HasPrefix(f, folder+"/") {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	total := len(items)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	out := make([]FileView, 0, end-start)
	for _, it := range items[start:end] {
		// 以普通用户视角判定，便于管理员核对最终可见性。
		d := s.eval.Evaluate(ctx, it, false, time.Now())
		v := FileView{
			Key: it.Key, RelPath: it.RelPath, MediaType: it.Type, Folder: it.Folder(),
			Duration: it.Duration, Size: it.Size, Playable: d.Playable, Reason: d.Reason,
		}
		if ov, ok := overrides[it.Key]; ok {
			o := ov
			v.Override = &o
			// 后台以「策略本身是否放行」呈现，不受管理员绕过影响。
			v.Playable, v.Reason = evaluateOverride(&o, s.cfg.Location)
		}
		out = append(out, v)
	}
	return out, total, nil
}

// GetFile 查询单文件策略与可见性。
func (s *AdminService) GetFile(ctx context.Context, key string) (*FileView, error) {
	it, ok := s.media.Get(key)
	if !ok {
		return nil, ErrMediaNotFound
	}
	ov, err := s.store.GetFileOverride(ctx, key)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	d := s.eval.Evaluate(ctx, it, false, time.Now())
	v := &FileView{
		Key: it.Key, RelPath: it.RelPath, MediaType: it.Type, Folder: it.Folder(),
		Duration: it.Duration, Size: it.Size, Playable: d.Playable, Reason: d.Reason,
	}
	if ov != nil {
		v.Override = ov
		v.Playable, v.Reason = evaluateOverride(ov, s.cfg.Location)
	}
	return v, nil
}

// FileOverrideInput 文件级策略输入。
type FileOverrideInput struct {
	Enabled      bool   `json:"enabled"`
	ScheduleType string `json:"schedule_type"`
	TimeStart    string `json:"time_start"`
	TimeEnd      string `json:"time_end"`
	Note         string `json:"note"`
}

// SetFileOverride 设置文件级覆盖（F-13）。
func (s *AdminService) SetFileOverride(ctx context.Context, admin *models.User, key string, in FileOverrideInput) (*models.FileOverride, error) {
	it, ok := s.media.Get(key)
	if !ok {
		return nil, ErrMediaNotFound
	}
	switch in.ScheduleType {
	case "", models.ScheduleAlways:
		in.ScheduleType = models.ScheduleAlways
	case models.ScheduleTimeWindow, models.ScheduleDisabled:
	default:
		return nil, fmt.Errorf("%w: 调度类型非法", ErrValidation)
	}
	if in.ScheduleType == models.ScheduleTimeWindow {
		if !validHHMM(in.TimeStart) || !validHHMM(in.TimeEnd) {
			return nil, fmt.Errorf("%w: 时间窗口格式应为 HH:MM", ErrValidation)
		}
	}
	var before any
	if b, err := s.store.GetFileOverride(ctx, key); err == nil {
		before = b
	}
	o, err := s.store.UpsertFileOverride(ctx, models.FileOverride{
		VideoKey: key, Enabled: in.Enabled, ScheduleType: in.ScheduleType,
		TimeStart: in.TimeStart, TimeEnd: in.TimeEnd, Note: in.Note,
	})
	if err != nil {
		return nil, err
	}
	s.eval.Invalidate()
	s.audit(ctx, admin, models.AuditTargetFile, key, "设置文件级策略（"+it.RelPath+"）", before, o)
	return o, nil
}

// DeleteFileOverride 取消文件级覆盖。
func (s *AdminService) DeleteFileOverride(ctx context.Context, admin *models.User, key string) error {
	removed, err := s.store.DeleteFileOverride(ctx, key)
	if err != nil {
		return err
	}
	if removed {
		s.eval.Invalidate()
		s.audit(ctx, admin, models.AuditTargetFile, key, "删除文件级策略", nil, nil)
	}
	return nil
}

// ListAuditLogs 审计日志查询。
func (s *AdminService) ListAuditLogs(ctx context.Context, admin *models.User, f repository.AuditFilter) ([]models.AuditLog, int, error) {
	out, total, err := s.store.ListAudit(ctx, f)
	if err == nil {
		s.log.Info(logging.CatBusiness, "admin.audit", "查询审计日志", withCtx(ctx, map[string]any{"page": f.Page, "total": total}))
	}
	return out, total, err
}

// QueryLogs 运行日志查询（F-43）。
func (s *AdminService) QueryLogs(ctx context.Context, admin *models.User, f logging.QueryFilter) ([]models.SystemLogEntry, int, error) {
	if s.logStore == nil {
		return nil, 0, errors.New("日志存储未启用")
	}
	out, total, err := s.logStore.Query(ctx, f)
	if err == nil {
		s.log.Info(logging.CatBusiness, "admin.logs", "查询运行日志", withCtx(ctx, map[string]any{
			"category": f.Category, "level": f.Level, "total": total,
		}))
	}
	return out, total, err
}

// LogDetail 单条运行日志。
func (s *AdminService) LogDetail(ctx context.Context, id int64) (*models.SystemLogEntry, error) {
	if s.logStore == nil {
		return nil, errors.New("日志存储未启用")
	}
	return s.logStore.GetByID(ctx, id)
}

// SetRegistration 开关注册入口（F-30）。
func (s *AdminService) SetRegistration(ctx context.Context, admin *models.User, open bool) error {
	before := s.authSvc.RegistrationOpen(ctx)
	if err := s.authSvc.SetRegistrationOpen(ctx, open); err != nil {
		return err
	}
	s.audit(ctx, admin, models.AuditTargetSetting, "registration", "切换注册开关",
		map[string]any{"registration_open": before}, map[string]any{"registration_open": open})
	return nil
}

// PurgeLogs 手动清理运行日志。
func (s *AdminService) PurgeLogs(ctx context.Context, admin *models.User) (int, error) {
	if s.logStore == nil {
		return 0, errors.New("日志存储未启用")
	}
	cfg := s.LoggingSettings(ctx)
	before := time.Now().AddDate(0, 0, -cfg.RetentionDays)
	n, err := s.logStore.Purge(ctx, before, cfg.MaxEntries)
	if err != nil {
		return 0, err
	}
	s.audit(ctx, admin, models.AuditTargetLog, "system_logs", "手动清理运行日志", nil, map[string]any{"removed": n})
	return n, nil
}

func evaluateOverride(o *models.FileOverride, loc *time.Location) (bool, string) {
	if !o.Enabled || o.ScheduleType == models.ScheduleDisabled {
		return false, models.ReasonDisabledByAdmin
	}
	if o.ScheduleType == models.ScheduleTimeWindow && !policy.InWindow(time.Now().In(loc), o.TimeStart, o.TimeEnd, loc) {
		return false, models.ReasonOutOfWindow
	}
	return true, ""
}

func validHHMM(v string) bool {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	return err1 == nil && err2 == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59
}

func atoiDefault(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// scannerSummary 由媒体服务暴露扫描统计。
func (s *MediaService) scannerSummary() media.ScanSummary {
	return s.scanner.Summary()
}

// GetStats 便捷方法（供 API 层调用）。
func (s *AdminService) GetStats(ctx context.Context) (StatsView, error) { return s.Stats(ctx) }

// ---------- 媒体目录（CR-003：后台可配置） ----------

// SettingMediaRoots 媒体根目录设置键（JSON 数组；空串表示回落到配置文件）。
const SettingMediaRoots = "media_roots"

// MediaSettingsView 媒体目录设置与扫描状态。
type MediaSettingsView struct {
	Roots        []string          `json:"roots"`
	Source       string            `json:"source"` // database=后台设置，config=配置文件
	ConfigRoots  []string          `json:"config_roots"`
	ScanInterval int               `json:"scan_interval_seconds"`
	ImageSeconds int               `json:"image_display_seconds"`
	Scanning     bool              `json:"scanning"`
	MediaTotal   int               `json:"media_total"`
	Scan         media.ScanSummary `json:"scan"`
}

// EffectiveMediaRoots 返回当前生效的媒体目录：后台设置优先，缺失时用配置文件。
func (s *AdminService) EffectiveMediaRoots(ctx context.Context) []string {
	raw := s.store.GetSetting(ctx, SettingMediaRoots, "")
	if raw != "" {
		var stored []string
		if err := json.Unmarshal([]byte(raw), &stored); err == nil {
			clean := make([]string, 0, len(stored))
			for _, p := range stored {
				if out, err := media.NormalizeRoot(p); err == nil {
					clean = append(clean, out)
				}
			}
			if len(clean) > 0 {
				return clean
			}
		}
	}
	out := make([]string, 0, len(s.cfg.Media.Roots))
	for _, p := range s.cfg.Media.Roots {
		out = append(out, p)
	}
	return out
}

// MediaSettings 读取媒体目录设置。
func (s *AdminService) MediaSettings(ctx context.Context) MediaSettingsView {
	roots := s.EffectiveMediaRoots(ctx)
	source := "config"
	if s.store.GetSetting(ctx, SettingMediaRoots, "") != "" {
		source = "database"
	}
	return MediaSettingsView{
		Roots:        roots,
		Source:       source,
		ConfigRoots:  append([]string(nil), s.cfg.Media.Roots...),
		ScanInterval: s.cfg.Media.ScanIntervalSeconds,
		ImageSeconds: s.cfg.Media.ImageDisplaySeconds,
		Scanning:     s.media.Scanning(),
		MediaTotal:   len(s.media.Snapshot().Items),
		Scan:         s.media.ScanSummary(),
	}
}

// UpdateMediaRoots 保存媒体目录并触发后台重扫（F-06 的运行时扩展）。
// 目录必须是存在的绝对路径；更换目录会重新计算 video_key，旧收藏可能失效。
func (s *AdminService) UpdateMediaRoots(ctx context.Context, admin *models.User, roots []string) (MediaSettingsView, error) {
	if len(roots) == 0 {
		return MediaSettingsView{}, fmt.Errorf("%w: 至少保留一个媒体目录", ErrValidation)
	}
	if len(roots) > 16 {
		return MediaSettingsView{}, fmt.Errorf("%w: 最多配置 16 个媒体目录", ErrValidation)
	}
	clean := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, p := range roots {
		out, err := media.NormalizeRoot(p)
		if err != nil {
			return MediaSettingsView{}, fmt.Errorf("%w: %s（%v）", ErrValidation, strings.TrimSpace(p), err)
		}
		if seen[out] {
			continue
		}
		seen[out] = true
		clean = append(clean, out)
	}
	if len(clean) == 0 {
		return MediaSettingsView{}, fmt.Errorf("%w: 至少保留一个媒体目录", ErrValidation)
	}

	before := s.MediaSettings(ctx)
	raw, err := json.Marshal(clean)
	if err != nil {
		return MediaSettingsView{}, err
	}
	if err := s.store.SetSetting(ctx, SettingMediaRoots, string(raw)); err != nil {
		return MediaSettingsView{}, err
	}
	s.media.Scanner().SetRoots(clean)
	s.media.Scanner().ScanAsync()
	s.audit(ctx, admin, models.AuditTargetSetting, SettingMediaRoots, "更新媒体目录", before.Roots, clean)

	view := s.MediaSettings(ctx)
	view.Scanning = true
	return view, nil
}

// ResetMediaRoots 清除后台设置，回落到配置文件中的 media.roots。
func (s *AdminService) ResetMediaRoots(ctx context.Context, admin *models.User) (MediaSettingsView, error) {
	before := s.MediaSettings(ctx)
	if err := s.store.SetSetting(ctx, SettingMediaRoots, ""); err != nil {
		return MediaSettingsView{}, err
	}
	roots := append([]string(nil), s.cfg.Media.Roots...)
	s.media.Scanner().SetRoots(roots)
	s.media.Scanner().ScanAsync()
	s.audit(ctx, admin, models.AuditTargetSetting, SettingMediaRoots, "恢复媒体目录为配置文件", before.Roots, roots)

	view := s.MediaSettings(ctx)
	view.Scanning = true
	return view, nil
}
