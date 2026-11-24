package media

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zys747526271/lanreel/server/internal/config"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/models"
)

// MetaStore 媒体元数据仓储（9.3 存储适配层）。
type MetaStore interface {
	ListAll(ctx context.Context) (map[string]models.MediaMeta, error)
	UpsertBatch(ctx context.Context, metas []models.MediaMeta) error
	DeleteByKeys(ctx context.Context, keys []string) error
}

// Snapshot 一次扫描结果的只读快照。
type Snapshot struct {
	Items     []*Item
	ByKey     map[string]*Item
	ScannedAt time.Time
}

// Get 按 key 取条目。
func (s *Snapshot) Get(key string) (*Item, bool) {
	if s == nil {
		return nil, false
	}
	it, ok := s.ByKey[key]
	return it, ok
}

// ScanSummary 扫描统计（用于日志与后台展示）。
type ScanSummary struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
	Roots      int       `json:"roots"`
	Total      int       `json:"total"`
	Videos     int       `json:"videos"`
	Images     int       `json:"images"`
	Skipped    int       `json:"skipped"`
	Added      int       `json:"added"`
	Removed    int       `json:"removed"`
	Errors     []string  `json:"errors"`
}

// Scanner 递归扫描媒体根目录并维护内存快照（F-01 / F-05）。
type Scanner struct {
	cfg  *config.Config
	log  *logging.Logger
	meta MetaStore

	snap     atomic.Pointer[Snapshot]
	mu       sync.Mutex
	summary  ScanSummary
	lastErr  error
	scanning atomic.Bool

	// rootsMu 保护可变媒体根目录（后台可热更新，见 CR-003）。
	rootsMu sync.RWMutex
	roots   []string

	// onScan 扫描完成后的回调（例如把条目交给缩略图生成器）。
	onScanMu sync.RWMutex
	onScan   func([]*Item)
}

// NewScanner 构造扫描器。
func NewScanner(cfg *config.Config, store MetaStore, logger *logging.Logger) *Scanner {
	s := &Scanner{cfg: cfg, meta: store, log: logger, roots: append([]string(nil), cfg.Media.Roots...)}
	s.snap.Store(&Snapshot{ByKey: map[string]*Item{}})
	return s
}

// SetOnScan 注册扫描完成回调。
func (s *Scanner) SetOnScan(fn func([]*Item)) {
	s.onScanMu.Lock()
	s.onScan = fn
	s.onScanMu.Unlock()
}

// Roots 返回当前生效的媒体根目录快照。
func (s *Scanner) Roots() []string {
	s.rootsMu.RLock()
	defer s.rootsMu.RUnlock()
	return append([]string(nil), s.roots...)
}

// SetRoots 热更新媒体根目录（调用方负责随后触发一次扫描）。
func (s *Scanner) SetRoots(roots []string) {
	clean := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, r := range roots {
		out, err := NormalizeRoot(r)
		if err != nil || seen[out] {
			continue
		}
		seen[out] = true
		clean = append(clean, out)
	}
	s.rootsMu.Lock()
	s.roots = clean
	s.rootsMu.Unlock()
}

// RootDisplayName 返回媒体根目录的展示名（取路径最后一段，盘符根保留盘符）。
func RootDisplayName(root string) string {
	clean := filepath.Clean(root)
	name := filepath.Base(clean)
	if name == "." || name == string(filepath.Separator) || name == "" {
		if v := filepath.VolumeName(clean); v != "" {
			return v
		}
		return clean
	}
	return name
}

// NormalizeRoot 校验并归一化一个媒体根目录：必须是存在的绝对路径目录。
func NormalizeRoot(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("目录不能为空")
	}
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", fmt.Errorf("解析路径失败: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", errors.New("目录不存在或不可访问")
	}
	if !st.IsDir() {
		return "", errors.New("不是目录")
	}
	return abs, nil
}

// Snapshot 返回当前快照。
func (s *Scanner) Snapshot() *Snapshot { return s.snap.Load() }

// Summary 返回最近一次扫描统计。
func (s *Scanner) Summary() ScanSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summary
}

// Scanning 是否正在扫描（后台用于轮询进度）。
func (s *Scanner) Scanning() bool { return s.scanning.Load() }

// ScanAsync 在后台触发一次扫描，不阻塞请求方。
func (s *Scanner) ScanAsync() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if _, _, err := s.Scan(ctx); err != nil {
			s.log.Warn(logging.CatScan, "media.scanner", "后台扫描未执行", map[string]any{"error": err.Error()})
		}
	}()
}

// LastError 返回最近一次扫描错误。
func (s *Scanner) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// Start 立即扫描一次，并按配置周期重扫（F-05）。
func (s *Scanner) Start(ctx context.Context) {
	if _, _, err := s.Scan(ctx); err != nil {
		s.log.Error(logging.CatScan, "media.scanner", "启动扫描失败", map[string]any{"error": err})
	}
	interval := time.Duration(s.cfg.Media.ScanIntervalSeconds) * time.Second
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, _, err := s.Scan(ctx); err != nil {
					s.log.Error(logging.CatScan, "media.scanner", "定时扫描失败", map[string]any{"error": err})
				}
			}
		}
	}()
}

// Scan 执行一次完整扫描。
func (s *Scanner) Scan(ctx context.Context) (*Snapshot, ScanSummary, error) {
	if !s.scanning.CompareAndSwap(false, true) {
		return s.Snapshot(), s.Summary(), errors.New("已有扫描任务在执行")
	}
	defer s.scanning.Store(false)

	started := time.Now()
	roots := s.Roots()
	summary := ScanSummary{StartedAt: started, Roots: len(roots)}
	s.log.Info(logging.CatScan, "media.scanner", "开始扫描媒体目录", map[string]any{
		"roots": len(roots), "paths": roots,
	})

	cached := map[string]models.MediaMeta{}
	if s.meta != nil {
		if m, err := s.meta.ListAll(ctx); err == nil {
			cached = m
		} else {
			s.log.Warn(logging.CatScan, "media.scanner", "读取媒体元数据缓存失败", map[string]any{"error": err})
		}
	}

	items := make([]*Item, 0, 256)
	seen := map[string]bool{}
	var metas []models.MediaMeta

	for idx, root := range roots {
		root := root
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				// 单个目录不可读不应中断整体扫描。
				summary.Errors = append(summary.Errors, logging.RedactPath(p)+": "+walkErr.Error())
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			name := d.Name()
			if d.IsDir() {
				if p == root {
					return nil
				}
				if s.skipDir(name) {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(name, ".") {
				summary.Skipped++
				return nil
			}
			kind := KindByExt(filepath.Ext(name), s.cfg.Media.VideoExtensions, s.cfg.Media.ImageExtensions)
			if kind == "" {
				summary.Skipped++
				return nil
			}
			info, err := d.Info()
			if err != nil {
				summary.Errors = append(summary.Errors, logging.RedactPath(p)+": "+err.Error())
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				summary.Errors = append(summary.Errors, logging.RedactPath(p)+": "+err.Error())
				return nil
			}
			rel = NormalizeRel(rel)
			key := MakeKey(s.cfg.App.SourceID, idx, rel)
			if seen[key] {
				summary.Skipped++
				return nil
			}
			seen[key] = true

			item := &Item{
				Key:       key,
				Source:    s.cfg.App.SourceID,
				RootIndex: idx,
				RootName:  RootDisplayName(root),
				RelPath:   rel,
				AbsPath:   p,
				Type:      kind,
				Size:      info.Size(),
				ModTime:   info.ModTime(),
			}
			if kind == models.MediaVideo {
				if m, ok := cached[key]; ok && m.MediaType == models.MediaVideo {
					item.Duration = m.Duration
				} else {
					item.Duration = ProbeDuration(p)
				}
				summary.Videos++
			} else {
				summary.Images++
			}
			items = append(items, item)
			metas = append(metas, models.MediaMeta{
				VideoKey:  key,
				MediaType: kind,
				Duration:  item.Duration,
				ThumbPath: cached[key].ThumbPath,
				ScannedAt: started,
			})
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			summary.Errors = append(summary.Errors, logging.RedactPath(root)+": "+err.Error())
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].RelPath < items[j].RelPath })

	newSnap := &Snapshot{
		Items:     items,
		ByKey:     make(map[string]*Item, len(items)),
		ScannedAt: started,
	}
	for _, it := range items {
		newSnap.ByKey[it.Key] = it
	}
	prev := s.snap.Load()
	prevKeys := map[string]bool{}
	if prev != nil {
		for k := range prev.ByKey {
			prevKeys[k] = true
		}
	}
	var removed []string
	for k := range prevKeys {
		if !seen[k] {
			removed = append(removed, k)
		}
	}
	for _, it := range items {
		if !prevKeys[it.Key] {
			summary.Added++
		}
	}
	summary.Removed = len(removed)
	summary.Total = len(items)

	if s.meta != nil {
		if err := s.meta.UpsertBatch(ctx, metas); err != nil {
			s.log.Warn(logging.CatScan, "media.scanner", "写入媒体元数据失败", map[string]any{"error": err})
		}
		if len(removed) > 0 {
			if err := s.meta.DeleteByKeys(ctx, removed); err != nil {
				s.log.Warn(logging.CatScan, "media.scanner", "清理失效媒体元数据失败", map[string]any{"error": err})
			}
		}
	}

	s.snap.Store(newSnap)
	s.onScanMu.RLock()
	hook := s.onScan
	s.onScanMu.RUnlock()
	if hook != nil {
		hook(items)
	}
	summary.FinishedAt = time.Now()
	summary.DurationMS = summary.FinishedAt.Sub(started).Milliseconds()

	s.mu.Lock()
	s.summary = summary
	s.lastErr = nil
	s.mu.Unlock()

	fields := map[string]any{
		"total": summary.Total, "videos": summary.Videos, "images": summary.Images,
		"skipped": summary.Skipped, "added": summary.Added, "removed": summary.Removed,
		"duration_ms": summary.DurationMS,
	}
	if len(summary.Errors) > 0 {
		fields["errors"] = len(summary.Errors)
		s.log.Warn(logging.CatScan, "media.scanner", "扫描完成（存在部分错误）", fields)
	} else {
		s.log.Info(logging.CatScan, "media.scanner", "扫描完成", fields)
	}
	return newSnap, summary, nil
}

func (s *Scanner) skipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, ig := range s.cfg.Media.IgnoredDirs {
		if strings.EqualFold(name, ig) {
			return true
		}
	}
	return false
}

// ResolveVerified 校验并返回媒体绝对路径（N-15）。
// 入参只能来自扫描器生成的条目（客户端仅提交 video_key），因此不存在路径穿越；
// 这里再确认文件仍然存在，避免删除后的陈旧请求读到异常路径。
func ResolveVerified(item *Item) (string, error) {
	if item == nil {
		return "", errors.New("媒体不存在")
	}
	abs, err := filepath.Abs(item.AbsPath)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err != nil || st.IsDir() {
		return "", fs.ErrNotExist
	}
	return abs, nil
}
