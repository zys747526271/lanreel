// Package media：文件系统事件监听（B-04）。
//
// 递归监听媒体根目录，新增/删除/重命名后去抖触发一次扫描；新增子目录会自动加入监听。
// 监听目录数量超过上限时主动退化为定时扫描，避免 inotify/watch 句柄耗尽。
package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/zys747526271/lanreel/server/internal/logging"
)

// Watcher 媒体目录监听器。
type Watcher struct {
	scanner *Scanner
	log     *logging.Logger

	// Debounce 事件去抖时长：批量拷贝文件时只触发一次扫描。
	debounce time.Duration
	// MaxWatches 监听目录上限，0 表示不限制。
	maxWatches int

	mu      sync.Mutex
	watched map[string]struct{}
	trigger chan struct{}
}

// NewWatcher 构造监听器。
func NewWatcher(scanner *Scanner, logger *logging.Logger, debounce time.Duration, maxWatches int) *Watcher {
	if debounce <= 0 {
		debounce = 2 * time.Second
	}
	if maxWatches <= 0 {
		maxWatches = 4096
	}
	return &Watcher{
		scanner: scanner, log: logger,
		debounce: debounce, maxWatches: maxWatches,
		watched: map[string]struct{}{},
		trigger: make(chan struct{}, 1),
	}
}

// Run 启动监听，阻塞直到 ctx 取消。
func (w *Watcher) Run(ctx context.Context) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.log.Warn(logging.CatScan, "media.watch", "文件监听不可用，回退定时扫描", map[string]any{"error": err.Error()})
		return
	}
	defer fsw.Close()

	w.addRoots(fsw)
	w.log.Info(logging.CatScan, "media.watch", "文件监听已启动", map[string]any{
		"roots": len(w.scanner.Roots()), "dirs": w.count(),
	})

	go w.debounceLoop(ctx)
	// 后台媒体目录可在管理页随时增删，定期把新目录纳入监听。
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.addRoots(fsw)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			w.handle(fsw, event)
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			w.log.Warn(logging.CatScan, "media.watch", "文件监听错误", map[string]any{"error": err.Error()})
		}
	}
}

// addRoots 把每个媒体根及其子目录加入监听。
func (w *Watcher) addRoots(fsw *fsnotify.Watcher) {
	for _, root := range w.scanner.Roots() {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if p != root {
				if strings.HasPrefix(d.Name(), ".") || w.scanner.skipDir(d.Name()) {
					return filepath.SkipDir
				}
			}
			w.add(fsw, p)
			if w.count() >= w.maxWatches {
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil && err != filepath.SkipAll {
			w.log.Warn(logging.CatScan, "media.watch", "监听目录失败", map[string]any{
				"root": logging.RedactPath(root), "error": err.Error(),
			})
		}
	}
	if w.count() >= w.maxWatches {
		w.log.Warn(logging.CatScan, "media.watch",
			"监听目录数达到上限，仅监听已加入的目录；其余目录依赖定时扫描", map[string]any{"limit": w.maxWatches})
	}
}

// handle 处理单个文件系统事件。
func (w *Watcher) handle(fsw *fsnotify.Watcher, event fsnotify.Event) {
	// 新建目录：立刻加入监听，避免遗漏其内部文件。
	if event.Op&fsnotify.Create != 0 {
		if st, err := os.Stat(event.Name); err == nil && st.IsDir() {
			if !w.isIgnoredDir(event.Name) {
				w.add(fsw, event.Name)
				w.addSubdirs(fsw, event.Name)
			}
		}
	}
	if w.isIgnored(event.Name) {
		return
	}
	// 只关心可能影响媒体集合的操作。
	if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Write) == 0 {
		return
	}
	select {
	case w.trigger <- struct{}{}:
	default:
		// 已有待处理的触发，合并事件。
	}
}

// addSubdirs 新增目录时把其下已存在的子目录一并加入监听。
func (w *Watcher) addSubdirs(fsw *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != root && (strings.HasPrefix(d.Name(), ".") || w.scanner.skipDir(d.Name())) {
			return filepath.SkipDir
		}
		w.add(fsw, p)
		if w.count() >= w.maxWatches {
			return filepath.SkipAll
		}
		return nil
	})
}

// debounceLoop 合并短时间内的多次事件，只触发一次扫描。
func (w *Watcher) debounceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.trigger:
			timer := time.NewTimer(w.debounce)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			w.log.Info(logging.CatScan, "media.watch", "检测到媒体目录变化，触发扫描", nil)
			if _, _, err := w.scanner.Scan(ctx); err != nil {
				// 扫描进行中会返回“已有扫描任务在执行”，属正常合并结果。
				w.log.Debug(logging.CatScan, "media.watch", "监听触发的扫描未执行", map[string]any{"reason": err.Error()})
			}
		}
	}
}

func (w *Watcher) add(fsw *fsnotify.Watcher, dir string) {
	w.mu.Lock()
	if _, ok := w.watched[dir]; ok {
		w.mu.Unlock()
		return
	}
	if len(w.watched) >= w.maxWatches {
		w.mu.Unlock()
		return
	}
	w.watched[dir] = struct{}{}
	w.mu.Unlock()
	if err := fsw.Add(dir); err != nil {
		w.mu.Lock()
		delete(w.watched, dir)
		w.mu.Unlock()
	}
}

func (w *Watcher) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.watched)
}

// isIgnoredDirs 判断目录名是否应跳过监听。
func (w *Watcher) isIgnoredDir(name string) bool {
	base := filepath.Base(name)
	return strings.HasPrefix(base, ".") || w.scanner.skipDir(base)
}

// isIgnored 判断事件路径是否落在忽略目录或非媒体文件上。
func (w *Watcher) isIgnored(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if strings.HasPrefix(part, ".") || w.scanner.skipDir(part) {
			return true
		}
	}
	ext := filepath.Ext(name)
	if ext == "" {
		return false
	}
	return KindByExt(ext, w.scanner.cfg.Media.VideoExtensions, w.scanner.cfg.Media.ImageExtensions) == ""
}
