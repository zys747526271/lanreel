// Package logging 实现后端分类运行日志（3.7 / 第 10 章）。
//
// 设计要点：
//   - 写入独立日志库 logs.db，与业务库物理分离（N-24）；
//   - 内存 channel + 单后台 goroutine 批量落库，满 100 条或每 1 秒刷盘（N-23）；
//   - 队列满或落库失败时降级到滚动文件 + stderr，绝不影响主流程（F-39）；
//   - 上下文按白名单字段写入，密码/Token/Cookie 一律脱敏（N-25）。
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// 日志级别。
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// 日志分类（10.4，前后端共用）。
const (
	CatSystem      = "system"
	CatScan        = "scan"
	CatAuth        = "auth"
	CatAccess      = "access"
	CatAPI         = "api"
	CatBusiness    = "business"
	CatPolicy      = "policy"
	CatMedia       = "media"
	CatPlayer      = "player"
	CatInteraction = "interaction"
	CatPerformance = "performance"
	CatError       = "error"
	CatFrontend    = "frontend"
)

// Entry 一条运行日志。
type Entry struct {
	TS          time.Time
	Level       string
	Category    string
	Module      string
	Message     string
	ContextJSON string
	RequestID   string
	UserID      *int64
}

// Store 日志存储抽象（9.3 存储适配层）。
type Store interface {
	InsertBatch(ctx context.Context, entries []Entry) error
	Query(ctx context.Context, f QueryFilter) ([]models.SystemLogEntry, int, error)
	Purge(ctx context.Context, before time.Time, maxEntries int) (int, error)
}

// QueryFilter 日志检索条件。
type QueryFilter struct {
	Category  string
	Level     string
	From      *time.Time
	To        *time.Time
	Keyword   string
	RequestID string
	UserID    *int64
	Page      int
	PageSize  int
}

var levelRank = map[string]int{LevelDebug: 10, LevelInfo: 20, LevelWarn: 30, LevelError: 40}

// Logger 分类日志记录器。
type Logger struct {
	loc      *time.Location
	minLevel int
	queue    chan Entry
	store    Store
	fallback *fallbackWriter

	startOnce sync.Once
	closeOnce sync.Once
	cancel    context.CancelFunc
	done      chan struct{}

	dropped atomic.Int64
	closed  atomic.Bool
}

const queueSize = 4096
const batchSize = 100
const flushInterval = time.Second

// New 创建日志记录器。store 允许为 nil（此时仅写降级文件）。
func New(level string, loc *time.Location, store Store, fallbackDir string) *Logger {
	if loc == nil {
		loc = time.Local
	}
	rank := levelRank[LevelInfo]
	if r, ok := levelRank[strings.ToLower(level)]; ok {
		rank = r
	}
	return &Logger{
		loc:      loc,
		minLevel: rank,
		queue:    make(chan Entry, queueSize),
		store:    store,
		fallback: newFallbackWriter(fallbackDir, loc),
		done:     make(chan struct{}),
	}
}

// Start 启动后台批量落库协程。
func (l *Logger) Start() {
	l.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		l.cancel = cancel
		go l.loop(ctx)
	})
}

// Close 停止接收日志并刷盘。
func (l *Logger) Close() {
	l.closeOnce.Do(func() {
		l.closed.Store(true)
		if l.cancel != nil {
			l.cancel()
			<-l.done
		}
		// 关闭后把残余队列同步刷掉，避免丢失尾日志。
		l.write(drain(l.queue))
		l.fallback.Close()
	})
}

func (l *Logger) loop(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]Entry, 0, batchSize)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		batch := make([]Entry, len(buf))
		copy(batch, buf)
		buf = buf[:0]
		l.write(batch)
	}

	for {
		select {
		case e, ok := <-l.queue:
			if !ok {
				flush()
				return
			}
			buf = append(buf, e)
			if len(buf) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (l *Logger) write(batch []Entry) {
	if l.store == nil {
		l.toFallback(batch)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.store.InsertBatch(ctx, batch); err != nil {
		l.dropped.Add(int64(len(batch)))
		fmt.Fprintf(os.Stderr, "[logging] 日志落库失败，降级文件: %v\n", err)
		l.toFallback(batch)
	}
}

func (l *Logger) toFallback(batch []Entry) {
	for _, e := range batch {
		line := fmt.Sprintf("%s %-5s [%s] %s %s %s",
			e.TS.In(l.loc).Format(time.RFC3339), strings.ToUpper(e.Level), e.Category, e.Module, e.Message, e.ContextJSON)
		if e.RequestID != "" {
			line += " request_id=" + e.RequestID
		}
		l.fallback.WriteLine(line)
	}
}

func drain(ch chan Entry) []Entry {
	var out []Entry
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		default:
			return out
		}
	}
}

// Log 写入一条日志。fields 中的 request_id / user_id 会被提取为独立列。
func (l *Logger) Log(level, category, module, message string, fields map[string]any) {
	if l == nil {
		return
	}
	rank := levelRank[level]
	if rank == 0 {
		rank = levelRank[LevelInfo]
		level = LevelInfo
	}
	if rank < l.minLevel {
		return
	}
	var reqID string
	var userID *int64
	if fields != nil {
		if v, ok := fields["request_id"]; ok {
			reqID, _ = v.(string)
			delete(fields, "request_id")
		}
		if v, ok := fields["user_id"]; ok {
			switch n := v.(type) {
			case int64:
				id := n
				userID = &id
			case int:
				id := int64(n)
				userID = &id
			}
			delete(fields, "user_id")
		}
	}
	e := Entry{
		TS:          time.Now(),
		Level:       level,
		Category:    category,
		Module:      module,
		Message:     message,
		ContextJSON: Sanitize(fields),
		RequestID:   reqID,
		UserID:      userID,
	}
	if l.closed.Load() {
		l.toFallback([]Entry{e})
		return
	}
	select {
	case l.queue <- e:
	default:
		// 队列已满：直接降级，不阻塞调用方。
		l.dropped.Add(1)
		l.toFallback([]Entry{e})
	}
}

// Debug 调试日志。
func (l *Logger) Debug(category, module, message string, fields map[string]any) {
	l.Log(LevelDebug, category, module, message, fields)
}

// Info 普通日志。
func (l *Logger) Info(category, module, message string, fields map[string]any) {
	l.Log(LevelInfo, category, module, message, fields)
}

// Warn 告警日志。
func (l *Logger) Warn(category, module, message string, fields map[string]any) {
	l.Log(LevelWarn, category, module, message, fields)
}

// Error 错误日志。
func (l *Logger) Error(category, module, message string, fields map[string]any) {
	l.Log(LevelError, category, module, message, fields)
}

// Store 返回底层存储（可能为 nil）。
func (l *Logger) Store() Store { return l.store }

// Dropped 返回降级写入条数，用于自检。
func (l *Logger) Dropped() int64 { return l.dropped.Load() }

// SetLevel 运行时调整日志级别（P2 能力，便于诊断）。
func (l *Logger) SetLevel(level string) {
	if r, ok := levelRank[strings.ToLower(level)]; ok {
		l.minLevel = r
	}
}

// Level 返回当前级别名称。
func (l *Logger) Level() string {
	for name, r := range levelRank {
		if r == l.minLevel {
			return name
		}
	}
	return LevelInfo
}

// sensitiveKeys 已知敏感字段（N-25）。
var sensitiveKeys = map[string]bool{
	"password": true, "old_password": true, "new_password": true, "password_hash": true,
	"token": true, "access_token": true, "refresh_token": true, "authorization": true,
	"cookie": true, "set-cookie": true, "secret": true, "jwt": true, "jwt_secret": true,
}

// Sanitize 将上下文转为 JSON 字符串并按白名单脱敏。
func Sanitize(fields map[string]any) string {
	if len(fields) == 0 {
		return ""
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		lk := strings.ToLower(k)
		if sensitiveKeys[lk] || strings.Contains(lk, "password") || strings.Contains(lk, "token") ||
			strings.Contains(lk, "secret") || strings.Contains(lk, "cookie") {
			out[k] = "[redacted]"
			continue
		}
		switch val := v.(type) {
		case nil:
			out[k] = nil
		case string:
			if len(val) > 1024 {
				val = val[:1024] + "...(truncated)"
			}
			out[k] = val
		case error:
			out[k] = val.Error()
		default:
			out[k] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// RedactPath 将绝对路径脱敏为「末两级目录/文件名」，用于日志上下文。
func RedactPath(p string) string {
	if p == "" {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	parts := strings.Split(clean, "/")
	if len(parts) <= 2 {
		return "/" + strings.Join(parts, "/")
	}
	return "/…/" + strings.Join(parts[len(parts)-2:], "/")
}

// fallbackWriter 按天滚动的降级日志文件。
type fallbackWriter struct {
	dir  string
	loc  *time.Location
	mu   sync.Mutex
	file *os.File
	day  string
}

func newFallbackWriter(dir string, loc *time.Location) *fallbackWriter {
	return &fallbackWriter{dir: dir, loc: loc}
}

func (w *fallbackWriter) WriteLine(line string) {
	fmt.Fprintln(os.Stderr, line)
	if w == nil || w.dir == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	day := time.Now().In(w.loc).Format("2006-01-02")
	if w.file == nil || w.day != day {
		if w.file != nil {
			w.file.Close()
		}
		if err := os.MkdirAll(w.dir, 0o755); err != nil {
			log.Printf("[logging] 创建降级日志目录失败: %v", err)
			return
		}
		f, err := os.OpenFile(filepath.Join(w.dir, "backend-"+day+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		w.file = f
		w.day = day
	}
	if w.file != nil {
		w.file.WriteString(line + "\n")
	}
}

func (w *fallbackWriter) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
}
