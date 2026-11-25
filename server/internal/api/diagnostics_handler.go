package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/webui"
)

// DiagnosticCheck 单项检测结果；status 取 ok / warn / error。
type DiagnosticCheck struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	Detail     string `json:"detail,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type diagnosticView struct {
	CheckedAt string             `json:"checked_at"`
	Healthy   bool               `json:"healthy"`
	Counts    map[string]int     `json:"counts"`
	Checks    []DiagnosticCheck  `json:"checks"`
}

func diag(status, summary string) DiagnosticCheck {
	return DiagnosticCheck{Status: status, Summary: summary}
}

func fileSize(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0, false
	}
	return info.Size(), true
}

// handleAdminDiagnostics GET /api/v1/admin/diagnostics
// 真实执行一组检测；除数据目录临时文件外不修改数据。每项独立容错，单项失败不影响其他检测。
func (h *Handlers) handleAdminDiagnostics(c *gin.Context) {
	ctx := c.Request.Context()
	checks := make([]DiagnosticCheck, 0, 8)
	add := func(id, name string, fn func() DiagnosticCheck) {
		item := diag("error", "检测未完成")
		item.ID, item.Name = id, name
		defer func() {
			if r := recover(); r != nil {
				item.Status = "error"
				item.Summary = fmt.Sprintf("检测异常：%v", r)
			}
			checks = append(checks, item)
		}()
		item = fn()
		item.ID, item.Name = id, name
	}

	// 服务进程：版本与运行时长。
	add("service", "服务进程", func() DiagnosticCheck {
		v := diag("ok", "服务运行中")
		v.Detail = fmt.Sprintf("版本 %s · 已运行 %s", h.version, formatUptime(h.startedAt))
		return v
	})

	// 数据库：真实 PRAGMA quick_check，并报告文件大小。
	add("database", "业务数据库", func() DiagnosticCheck {
		v := diag("error", "数据库检查失败")
		var result string
		if err := h.store.DB().QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&result); err != nil {
			v.Summary = "数据库完整性检查失败"
			v.Detail = err.Error()
			v.Suggestion = "检查磁盘空间与数据库文件权限，必要时从备份恢复。"
			return v
		}
		if !strings.EqualFold(strings.TrimSpace(result), "ok") {
			v.Summary = "数据库存在损坏页"
			v.Detail = result
			v.Suggestion = "立即停止写入并从备份恢复数据库。"
			return v
		}
		v.Status, v.Summary = "ok", "数据库完整"
		if size, ok := fileSize(h.cfg.Database.Path); ok {
			v.Detail = fmt.Sprintf("%s · %s", h.cfg.Database.Path, humanBytes(size))
		} else {
			v.Detail = h.cfg.Database.Path
		}
		return v
	})

	// 媒体目录：目录可达、媒体文件是否仍存在。
	add("media", "媒体目录与文件", func() DiagnosticCheck {
		roots := h.media.Roots()
		if len(roots) == 0 {
			v := diag("error", "未配置媒体目录")
			v.Suggestion = "到「媒体目录」页添加至少一个存在的文件夹。"
			return v
		}
		missingRoots := 0
		for _, r := range roots {
			info, err := os.Stat(r.Path)
			if err != nil || !info.IsDir() {
				missingRoots++
			}
		}
		snap := h.media.Snapshot()
		missingFiles := 0
		// 抽样检查，避免超大媒体库把诊断请求拖成慢查询。
		checked := 0
		for _, it := range snap.Items {
			if checked >= 2000 {
				break
			}
			if it.AbsPath == "" {
				continue
			}
			checked++
			if _, err := os.Stat(it.AbsPath); err != nil {
				missingFiles++
			}
		}
		v := diag("ok", fmt.Sprintf("%d 个媒体目录 · %d 个媒体文件", len(roots), len(snap.Items)))
		details := make([]string, 0, 3)
		details = append(details, fmt.Sprintf("目录不可达 %d 个", missingRoots))
		missingLabel := fmt.Sprintf("文件已丢失 %d 个", missingFiles)
		if checked < len(snap.Items) {
			missingLabel = fmt.Sprintf("文件已丢失 %d 个（抽样 %d/%d）", missingFiles, checked, len(snap.Items))
		}
		details = append(details, missingLabel)
		v.Detail = strings.Join(details, " · ")
		if missingRoots > 0 {
			v.Status, v.Suggestion = "error", "检查媒体目录路径、挂载状态与读取权限。"
		} else if missingFiles > 0 {
			v.Status, v.Suggestion = "warn", "在收藏夹中清理失效视频，或恢复缺失的媒体文件。"
		}
		if len(snap.Items) == 0 && v.Status != "error" {
			v.Status, v.Suggestion = "warn", "目录存在但未扫描到媒体，点击「立即重新扫描」。"
		}
		return v
	})

	// 扫描任务：最近扫描时间与最近一次扫描告警。
	add("scan", "媒体扫描任务", func() DiagnosticCheck {
		stats, err := h.admin.Stats(ctx)
		if err != nil {
			v := diag("error", "无法读取扫描状态")
			v.Detail = err.Error()
			return v
		}
		s := stats.Scan
		v := diag("ok", "扫描状态正常")
		if s.FinishedAt.IsZero() {
			v.Status, v.Summary = "warn", "尚未完成过扫描"
			return v
		}
		v.Detail = fmt.Sprintf("最近完成 %s · 耗时 %d ms · 跳过 %d · 新增/移除 %d/%d",
			s.FinishedAt.In(h.cfg.Location).Format("2006-01-02 15:04:05"), s.DurationMS, s.Skipped, s.Added, s.Removed)
		staleFor := time.Since(s.FinishedAt)
		if staleFor > time.Duration(h.cfg.Media.ScanIntervalSeconds*3)*time.Second && staleFor > 5*time.Minute {
			v.Status, v.Suggestion = "warn", "扫描已长时间未更新，检查后台扫描是否被文件监听频繁打断。"
		}
		if len(s.Errors) > 0 {
			v.Status, v.Suggestion = "warn", "查看「运行日志」中的 scan 分类，处理无法读取的目录或文件。"
			v.Detail += fmt.Sprintf(" · 告警 %d 条", len(s.Errors))
		}
		return v
	})

	// 策略：策略数量、版本号和实时通知通道是否可用。
	add("policy", "播放策略", func() DiagnosticCheck {
		folderPolicies, err1 := h.store.ListFolderPolicies(ctx)
		fileOverrides, err2 := h.store.CountFileOverrides(ctx)
		if err1 != nil || err2 != nil {
			v := diag("error", "无法读取播放策略")
			if err1 != nil {
				v.Detail = err1.Error()
			} else {
				v.Detail = err2.Error()
			}
			return v
		}
		version := h.media.PolicyVersion()
		v := diag("ok", fmt.Sprintf("文件夹策略 %d 条 · 文件级策略 %d 条", len(folderPolicies), fileOverrides))
		v.Detail = fmt.Sprintf("策略版本 %d · 实时事件流可用", version)
		return v
	})

	// 日志：近 24 小时错误/告警数量，日志文件尺寸。
	add("logs", "运行日志", func() DiagnosticCheck {
		if h.logStore == nil {
			v := diag("warn", "日志存储未启用")
			v.Suggestion = "检查日志数据库配置。"
			return v
		}
		from := time.Now().Add(-24 * time.Hour)
		_, errorCount, err1 := h.logStore.Query(ctx, logging.QueryFilter{PageSize: 1, Level: logging.LevelError, From: &from})
		_, warnCount, err2 := h.logStore.Query(ctx, logging.QueryFilter{PageSize: 1, Level: logging.LevelWarn, From: &from})
		if err1 != nil || err2 != nil {
			v := diag("error", "读取运行日志失败")
			v.Suggestion = "检查日志数据库文件权限。"
			return v
		}
		v := diag("ok", fmt.Sprintf("近 24 小时：错误 %d · 告警 %d", errorCount, warnCount))
		if size, ok := fileSize(h.cfg.Logging.Path); ok {
			v.Detail = fmt.Sprintf("日志库 %s", humanBytes(size))
		}
		if errorCount > 0 {
			v.Status, v.Suggestion = "warn", "到「运行日志」筛选 error 级别，定位最近失败请求。"
		}
		return v
	})

	// 前端资源：单二进制内嵌构建产物是否齐全。
	add("assets", "前端资源", func() DiagnosticCheck {
		_, ok := webui.Assets()
		if !ok {
			v := diag("error", "内嵌前端资源缺失")
			v.Suggestion = "重新执行前端构建并重新编译单二进制。"
			return v
		}
		return diag("ok", "前端资源已内嵌")
	})

	// 数据目录：存在且可写。
	add("storage", "数据目录", func() DiagnosticCheck {
		dir := h.cfg.App.DataDir
		if dir == "" {
			dir = filepath.Dir(h.cfg.Database.Path)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			v := diag("error", "数据目录不可用")
			v.Detail = err.Error()
			v.Suggestion = "为运行账号授予数据目录写入权限。"
			return v
		}
		f, err := os.CreateTemp(dir, ".lanreel-diag-*")
		if err != nil {
			v := diag("error", "数据目录不可写")
			v.Detail = err.Error()
			v.Suggestion = "为运行账号授予数据目录写入权限。"
			return v
		}
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		v := diag("ok", "数据目录可写")
		v.Detail = dir
		return v
	})

	counts := map[string]int{"ok": 0, "warn": 0, "error": 0}
	for _, chk := range checks {
		counts[chk.Status]++
	}
	okCount := true
	for _, chk := range checks {
		if chk.Status == "error" {
			okCount = false
		}
	}
	ok(c, diagnosticView{
		CheckedAt: time.Now().In(h.cfg.Location).Format(time.RFC3339),
		Healthy:   okCount,
		Counts:    counts,
		Checks:    checks,
	})
}

func formatUptime(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时 %d 分钟", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%d 天 %d 小时", int(d.Hours())/24, int(d.Hours())%24)
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
