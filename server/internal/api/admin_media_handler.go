package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/media"
)

// dirEntryPayload 目录浏览器的一项。
type dirEntryPayload struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// handleAdminMediaSettings GET /api/v1/admin/settings/media
func (h *Handlers) handleAdminMediaSettings(c *gin.Context) {
	ok(c, h.admin.MediaSettings(c.Request.Context()))
}

type mediaRootInput struct {
	Path      string `json:"path"`
	Recursive *bool  `json:"recursive"`
}

type mediaRootsRequest struct {
	Roots      []string         `json:"roots"`
	RootInputs []mediaRootInput `json:"root_inputs"`
	// Recursive 全局默认（当 root_inputs 未提供时使用）。
	Recursive *bool `json:"recursive"`
}

// normalize 把兼容旧版的 roots []string 与新版的 root_inputs 统一成路径 + 递归标志。
func (r mediaRootsRequest) normalize() ([]string, []bool) {
	paths := r.Roots
	rec := make([]bool, 0, len(paths))
	if len(r.RootInputs) > 0 {
		paths = make([]string, 0, len(r.RootInputs))
		for _, in := range r.RootInputs {
			paths = append(paths, in.Path)
			v := true
			if in.Recursive != nil {
				v = *in.Recursive
			}
			rec = append(rec, v)
		}
	} else {
		v := true
		if r.Recursive != nil {
			v = *r.Recursive
		}
		for range paths {
			rec = append(rec, v)
		}
	}
	return paths, rec
}

// handleAdminSetMediaRoots PUT /api/v1/admin/settings/media
//
// 保存后立即热更新扫描根目录并在后台重扫；更换目录会重新计算 video_key，
// 旧收藏可能失效（前端有提示）。
func (h *Handlers) handleAdminSetMediaRoots(c *gin.Context) {
	var req mediaRootsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}
	u := currentUser(c)
	if u == nil {
		fail(c, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}
	paths, recursive := req.normalize()
	if len(paths) == 0 {
		fail(c, http.StatusBadRequest, "invalid_body", "至少保留一个媒体目录")
		return
	}
	v, err := h.admin.UpdateMediaRootsEx(c.Request.Context(), u, paths, recursive)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, v)
}

// handleAdminResetMediaRoots DELETE /api/v1/admin/settings/media
func (h *Handlers) handleAdminResetMediaRoots(c *gin.Context) {
	u := currentUser(c)
	if u == nil {
		fail(c, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}
	v, err := h.admin.ResetMediaRoots(c.Request.Context(), u)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, v)
}

// handleAdminFSDirs GET /api/v1/admin/fs/dirs?path=
//
// 目录浏览器：path 为空时列出根（Windows 为盘符），否则列出该目录下的子目录。
func (h *Handlers) handleAdminFSDirs(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("path"))
	current, parent := "", ""
	entries := make([]dirEntryPayload, 0, 32)

	if raw == "" {
		for _, d := range rootDirs() {
			entries = append(entries, dirEntryPayload{Name: d, Path: d})
		}
	} else {
		abs, err := media.NormalizeRoot(raw)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_path", "目录不存在或不可访问："+raw)
			return
		}
		current = abs
		if p := filepath.Dir(abs); p != abs {
			parent = p
		}
		des, err := os.ReadDir(abs)
		if err != nil {
			fail(c, http.StatusBadRequest, "unreadable_path", "无法读取目录："+err.Error())
			return
		}
		for _, de := range des {
			if !de.IsDir() {
				continue
			}
			name := de.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			entries = append(entries, dirEntryPayload{Name: name, Path: filepath.Join(abs, name)})
		}
		sort.Slice(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		if len(entries) > 500 {
			entries = entries[:500]
		}
	}
	ok(c, gin.H{
		"current":   current,
		"parent":    parent,
		"entries":   entries,
		"separator": string(os.PathSeparator),
	})
}

// rootDirs 返回文件系统根：Windows 为可用盘符，其他系统为 /。
func rootDirs() []string {
	if runtime.GOOS != "windows" {
		return []string{"/"}
	}
	out := make([]string, 0, 8)
	for ch := 'A'; ch <= 'Z'; ch++ {
		p := string(ch) + ":" + string(os.PathSeparator)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}
