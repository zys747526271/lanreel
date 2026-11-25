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

// Change 策略版本变更通知。客户端只关心版本号，收到后重新批量判定媒体状态，
// 通知本身不携带任何用户或媒体隐私信息。
type Change struct {
	Version   uint64    `json:"version"`
	ChangedAt time.Time `json:"changed_at"`
}

// Viewer 判定身份（管理员 / 普通用户 / 游客）。
type Viewer struct {
	IsAdmin bool
	IsGuest bool
}

// ForUser 由用户对象生成身份：nil 视为游客。
func ForUser(u *models.User) Viewer {
	if u == nil {
		return Viewer{IsGuest: true}
	}
	return Viewer{IsAdmin: u.IsAdmin()}
}

// folderKey 文件夹策略的主键：根索引 + 源内相对路径（B-07）。
type folderKey struct {
	Root int
	Path string
}

// Evaluator 可见性判定器。
type Evaluator struct {
	src Source
	loc *time.Location
	log *logging.Logger

	mu       sync.RWMutex
	folders  map[folderKey]models.FolderPolicy
	files    map[string]models.FileOverride
	loadedAt time.Time
	cacheTTL time.Duration

	version     uint64
	subscribers map[chan Change]struct{}
}

// NewEvaluator 构造判定器。
func NewEvaluator(src Source, loc *time.Location, logger *logging.Logger) *Evaluator {
	if loc == nil {
		loc = time.Local
	}
	return &Evaluator{
		src: src, loc: loc, log: logger, cacheTTL: 5 * time.Second,
		version: 1, subscribers: make(map[chan Change]struct{}),
	}
}

// Version 返回当前策略版本。新客户端连接时先读取此版本，避免错过连接前的变更。
func (e *Evaluator) Version() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.version
}

// Subscribe 订阅策略变更。通道缓冲为 1，慢客户端只保留最新版本号，不会阻塞后台保存。
// 调用方必须在退出时执行 cleanup，否则会泄漏订阅通道。
func (e *Evaluator) Subscribe() (<-chan Change, func()) {
	ch := make(chan Change, 1)
	e.mu.Lock()
	if e.subscribers == nil {
		e.subscribers = make(map[chan Change]struct{})
	}
	e.subscribers[ch] = struct{}{}
	e.mu.Unlock()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			e.mu.Lock()
			if _, ok := e.subscribers[ch]; ok {
				delete(e.subscribers, ch)
				close(ch)
			}
			e.mu.Unlock()
		})
	}
	return ch, cleanup
}

// Invalidate 策略变更后使缓存失效并广播新版本（N-10 / v0.3.1 实时同步）。
func (e *Evaluator) Invalidate() {
	e.mu.Lock()
	e.loadedAt = time.Time{}
	e.version++
	change := Change{Version: e.version, ChangedAt: time.Now()}
	for ch := range e.subscribers {
		select {
		case ch <- change:
		default:
			// 客户端卡顿时丢弃中间版本；版本单调递增，重连后可按最新值恢复。
		}
	}
	e.mu.Unlock()
}

func (e *Evaluator) load(ctx context.Context) {
	attempts := 0
	for {
		e.mu.RLock()
		fresh := time.Since(e.loadedAt) < e.cacheTTL && e.loadedAt != (time.Time{})
		generation := e.version
		e.mu.RUnlock()
		if fresh {
			return
		}

		folders := map[folderKey]models.FolderPolicy{}
		files := map[string]models.FileOverride{}
		if e.src != nil {
			if list, err := e.src.ListFolderPolicies(ctx); err == nil {
				for _, p := range list {
					folders[folderKey{Root: p.RootIndex, Path: media.NormalizeFolderPath(p.FolderPath)}] = p
				}
			}
			if m, err := e.src.ListFileOverrides(ctx); err == nil {
				files = m
			}
		}

		e.mu.Lock()
		if generation != e.version && attempts < 3 {
			// 读取期间策略被改动：丢弃旧结果并重新读，避免把过期策略写回缓存。
			e.mu.Unlock()
			attempts++
			continue
		}
		// 极端高频写入时避免调用方活锁：接受本次结果，但不标记缓存新鲜。
		forceReload := generation != e.version
		e.folders = folders
		e.files = files
		if forceReload {
			e.loadedAt = time.Time{}
		} else {
			e.loadedAt = time.Now()
		}
		e.mu.Unlock()
		return
	}
}

// Evaluate 判定某媒体对某身份是否可播放。
//
// 可见性维度：文件级覆盖 > 文件夹策略（同根最长前缀）> 全局默认策略（RootGlobal）> 允许。
// 每个策略分别控制普通用户（Enabled）与游客（GuestEnabled）是否可见，时间窗口共用。
func (e *Evaluator) Evaluate(ctx context.Context, item *media.Item, v Viewer, now time.Time) Decision {
	if item == nil {
		return Decision{Playable: false, Reason: models.ReasonFileMissing}
	}
	// 管理员不受内容策略限制，便于后台预览与实际排查。
	if v.IsAdmin {
		return Decision{Playable: true}
	}
	e.load(ctx)
	e.mu.RLock()
	defer e.mu.RUnlock()

	if ov, ok := e.files[item.Key]; ok {
		return e.decide(ov.Enabled, ov.GuestEnabled,
			ov.ScheduleType, ov.TimeStart, ov.TimeEnd,
			ov.GuestScheduleType, ov.GuestTimeStart, ov.GuestTimeEnd, v, now)
	}
	if p, ok := e.matchFolder(item.RootIndex, item.Folder()); ok {
		return e.decide(p.Enabled, p.GuestEnabled,
			p.ScheduleType, p.TimeStart, p.TimeEnd,
			p.GuestScheduleType, p.GuestTimeStart, p.GuestTimeEnd, v, now)
	}
	return Decision{Playable: true}
}

// decide 依据受众与时间窗给出判定（v0.3.0：用户与游客的播放时段互相独立）。
// 游客优先使用 GuestSchedule*，未配置（空）时回落到普通用户时段，兼容旧数据。
func (e *Evaluator) decide(enabled, guestEnabled bool, scheduleType, start, end string,
	guestScheduleType, guestStart, guestEnd string, v Viewer, now time.Time) Decision {
	visible := enabled
	if v.IsGuest {
		visible = guestEnabled
		if guestScheduleType != "" {
			scheduleType, start, end = guestScheduleType, guestStart, guestEnd
		}
	}
	if !visible || scheduleType == models.ScheduleDisabled {
		return Decision{Playable: false, Reason: models.ReasonDisabledByAdmin}
	}
	if scheduleType == models.ScheduleTimeWindow && !InWindow(now.In(e.loc), start, end, e.loc) {
		return Decision{Playable: false, Reason: models.ReasonOutOfWindow}
	}
	return Decision{Playable: true}
}

// MatchFolder 按根索引做最长前缀匹配，未命中时回落到全局默认策略。
func (e *Evaluator) MatchFolder(ctx context.Context, rootIndex int, folder string) (models.FolderPolicy, bool) {
	e.load(ctx)
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.matchFolder(rootIndex, folder)
}

// matchFolder 最长前缀匹配文件夹策略（须在持锁状态下调用）。
// 同一根内，目录路径越长优先级越高；根目录策略（空串）在该根内优先级最低。
// 该根没有任何策略时，使用 RootGlobal 的全局默认策略。
func (e *Evaluator) matchFolder(rootIndex int, folder string) (models.FolderPolicy, bool) {
	folder = media.NormalizeFolderPath(folder)
	best := ""
	var bestPolicy models.FolderPolicy
	found := false
	for k, p := range e.folders {
		if k.Root != rootIndex {
			continue
		}
		if k.Path == "" {
			if !found {
				best, bestPolicy, found = "", p, true
			}
			continue
		}
		if folder == k.Path || strings.HasPrefix(folder, k.Path+"/") {
			if len(k.Path) >= len(best) {
				best, bestPolicy, found = k.Path, p, true
			}
		}
	}
	if found {
		return bestPolicy, true
	}
	// 回落到全局默认策略（对所有根生效）。
	best = ""
	for k, p := range e.folders {
		if k.Root != models.RootGlobal {
			continue
		}
		if k.Path == "" {
			if !found {
				best, bestPolicy, found = "", p, true
			}
			continue
		}
		if folder == k.Path || strings.HasPrefix(folder, k.Path+"/") {
			if len(k.Path) >= len(best) {
				best, bestPolicy, found = k.Path, p, true
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
