// Package policy 实现内容可见性与时间窗口判断（3.3 / 9.4）。
//
// 规则优先级：文件级覆盖 > 文件夹策略（最长前缀匹配）> 默认允许。
// 所有时间判断均在服务端按配置时区完成（F-14 / N-22）。
package policy

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/models"
)

// Source 策略数据来源。
type Source interface {
	ListFolderPolicies(ctx context.Context) ([]models.FolderPolicy, error)
	GetFileOverride(ctx context.Context, videoKey string) (*models.FileOverride, error)
	// ListFileOverrides 返回全部文件级覆盖，用于批量判定。
	ListFileOverrides(ctx context.Context) (map[string]models.FileOverride, error)
}

// Decision 判定结果。
type Decision struct {
	Playable bool
	Reason   string
}

// Evaluator 可见性判定器。
type Evaluator struct {
	src Source
	loc *time.Location
	log *logging.Logger

	mu        sync.RWMutex
	folders   map[string]models.FolderPolicy
	files     map[string]models.FileOverride
	loadedAt  time.Time
	cacheTTL  time.Duration
}

// NewEvaluator 构造判定器。
func NewEvaluator(src Source, loc *time.Location, logger *logging.Logger) *Evaluator {
	if loc == nil {
		loc = time.Local
	}
	return &Evaluator{src: src, loc: loc, log: logger, cacheTTL: 5 * time.Second}
}

// Invalidate 策略变更后使缓存失效（N-10）。
func (e *Evaluator) Invalidate() {
	e.mu.Lock()
	e.loadedAt = time.Time{}
	e.mu.Unlock()
}

func (e *Evaluator) load(ctx context.Context) {
	e.mu.RLock()
	fresh := time.Since(e.loadedAt) < e.cacheTTL && e.loadedAt != (time.Time{})
	e.mu.RUnlock()
	if fresh {
		return
	}
	folders := map[string]models.FolderPolicy{}
	files := map[string]models.FileOverride{}
	if e.src != nil {
		if list, err := e.src.ListFolderPolicies(ctx); err == nil {
			for _, p := range list {
				folders[media.NormalizeFolderPath(p.FolderPath)] = p
			}
		}
		if m, err := e.src.ListFileOverrides(ctx); err == nil {
			files = m
		}
	}
	e.mu.Lock()
	e.folders = folders
	e.files = files
	e.loadedAt = time.Now()
	e.mu.Unlock()
}

// Evaluate 判定某媒体对某用户是否可播放。
func (e *Evaluator) Evaluate(ctx context.Context, item *media.Item, isAdmin bool, now time.Time) Decision {
	if item == nil {
		return Decision{Playable: false, Reason: models.ReasonFileMissing}
	}
	// 管理员不受内容策略限制，便于后台预览与实际排查。
	if isAdmin {
		return Decision{Playable: true}
	}
	e.load(ctx)
	e.mu.RLock()
	defer e.mu.RUnlock()

	if ov, ok := e.files[item.Key]; ok {
		if !ov.Enabled || ov.ScheduleType == models.ScheduleDisabled {
			return Decision{Playable: false, Reason: models.ReasonDisabledByAdmin}
		}
		if ov.ScheduleType == models.ScheduleTimeWindow && !InWindow(now.In(e.loc), ov.TimeStart, ov.TimeEnd, e.loc) {
			return Decision{Playable: false, Reason: models.ReasonOutOfWindow}
		}
		return Decision{Playable: true}
	}

	if p, ok := e.matchFolder(item.Folder()); ok {
		if !p.Enabled || p.ScheduleType == models.ScheduleDisabled {
			return Decision{Playable: false, Reason: models.ReasonDisabledByAdmin}
		}
		if p.ScheduleType == models.ScheduleTimeWindow && !InWindow(now.In(e.loc), p.TimeStart, p.TimeEnd, e.loc) {
			return Decision{Playable: false, Reason: models.ReasonOutOfWindow}
		}
		return Decision{Playable: true}
	}
	return Decision{Playable: true}
}

// MatchFolder 最长前缀匹配文件夹策略。
// 根目录策略（folder_path 为空）作为全局默认策略，对未单独配置的子目录生效。
func (e *Evaluator) MatchFolder(ctx context.Context, folder string) (models.FolderPolicy, bool) {
	e.load(ctx)
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.matchFolder(folder)
}

// matchFolder 最长前缀匹配文件夹策略（须在持锁状态下调用）。
func (e *Evaluator) matchFolder(folder string) (models.FolderPolicy, bool) {
	folder = media.NormalizeFolderPath(folder)
	best := ""
	var bestPolicy models.FolderPolicy
	found := false
	for path, p := range e.folders {
		if path == "" {
			// 根目录策略：匹配任何层级，优先级最低。
			if !found {
				best, bestPolicy, found = "", p, true
			}
			continue
		}
		if folder == path || strings.HasPrefix(folder, path+"/") {
			if len(path) >= len(best) {
				best, bestPolicy, found = path, p, true
			}
		}
	}
	return bestPolicy, found
}

// InWindow 判断当前时间是否落在时间窗口内，支持跨零点（如 22:00–02:00）。
func InWindow(now time.Time, start, end string, loc *time.Location) bool {
	sh, sm, ok1 := parseHHMM(start)
	eh, em, ok2 := parseHHMM(end)
	if !ok1 || !ok2 {
		// 时间格式非法时不额外限制，避免误伤；管理员可在后台修正。
		return true
	}
	if sh == eh && sm == em {
		return true
	}
	cur := now.Hour()*60 + now.Minute()
	s := sh*60 + sm
	en := eh*60 + em
	if s < en {
		return cur >= s && cur < en
	}
	// 跨零点窗口
	return cur >= s || cur < en
}

// parseHHMM 解析 "HH:MM"。
func parseHHMM(v string) (int, int, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, 0, false
	}
	parts := strings.Split(v, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := atoi(parts[0])
	m, err2 := atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

func atoi(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errInvalid
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var errInvalid = &invalidError{}

type invalidError struct{}

func (e *invalidError) Error() string { return "非法数字" }
