package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/reqctx"
	"github.com/zys747526271/lanreel/server/internal/service"
)

// handleRandomMedia GET /api/v1/media/random
func (h *Handlers) handleRandomMedia(c *gin.Context) {
	u := currentUser(c)
	limit := queryInt(c, "limit", 1)
	var exclude []string
	if v := c.Query("exclude"); v != "" {
		for _, part := range strings.Split(v, ",") {
			if s := strings.TrimSpace(part); s != "" {
				exclude = append(exclude, s)
			}
		}
	}
	q := service.RandomQuery{Limit: limit, Exclude: exclude}
	if c.Query("mode") == "favorites" {
		if u == nil {
			fail(c, http.StatusUnauthorized, models.ReasonLoginRequired, "登录后可查看收藏")
			return
		}
		q.Favorites = true
		keys, err := h.fav.Keys(c.Request.Context(), u.ID)
		if err != nil {
			failService(c, err)
			return
		}
		q.FavoriteKeys = keys
	}
	items, err := h.media.Random(c.Request.Context(), u, q)
	if err != nil {
		failService(c, err)
		return
	}
	h.log.Debug(logging.CatMedia, "api.media", "返回随机媒体", map[string]any{
		"request_id": reqctx.RequestID(c.Request.Context()),
		"count":      len(items),
		"mode":       c.Query("mode"),
	})
	ok(c, items)
}

// handleStream GET /api/v1/media/:key/stream
// 使用 http.ServeContent 原生支持 Range 与条件请求（F-04 / N-07）。
func (h *Handlers) handleStream(c *gin.Context) {
	u := currentUser(c)
	key := c.Param("key")
	item, found := h.media.Get(key)
	if !found {
		h.log.Warn(logging.CatMedia, "api.media", "媒体不存在", map[string]any{
			"request_id": reqctx.RequestID(c.Request.Context()),
			"video_key":  key,
		})
		fail(c, http.StatusNotFound, "media_not_found", "媒体不存在或已被移除")
		return
	}
	decision := h.media.Decide(c.Request.Context(), u, item)
	if !decision.Playable {
		h.log.Info(logging.CatMedia, "api.media", "拒绝播放不可用媒体", map[string]any{
			"request_id": reqctx.RequestID(c.Request.Context()),
			"video_key":  key,
			"reason":     decision.Reason,
			"rel_path":   item.RelPath,
		})
		fail(c, http.StatusForbidden, decision.Reason, reasonMessage(decision.Reason))
		return
	}

	abs, err := media.ResolveVerified(item)
	if err != nil {
		h.log.Error(logging.CatError, "api.media", "媒体文件读取失败", map[string]any{
			"request_id": reqctx.RequestID(c.Request.Context()),
			"video_key":  key,
			"rel_path":   item.RelPath,
			"error":      err,
		})
		fail(c, http.StatusNotFound, models.ReasonFileMissing, "媒体文件已丢失")
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		fail(c, http.StatusInternalServerError, "open_failed", "打开媒体文件失败")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		fail(c, http.StatusInternalServerError, "stat_failed", "读取媒体信息失败")
		return
	}

	// N-06 / N-07：可缓存一天，不含 Set-Cookie，由 ServeContent 生成 ETag / Last-Modified。
	hdr := c.Writer.Header()
	hdr.Set("Cache-Control", "public, max-age=86400")
	hdr.Set("Accept-Ranges", "bytes")
	hdr.Set("X-Content-Type-Options", "nosniff")
	if item.Type == models.MediaImage {
		hdr.Set("Content-Type", contentTypeByExt(abs))
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(abs), st.ModTime(), f)
}

// handleThumb GET /api/v1/media/:key/thumb
// 优先返回服务端生成的缩略图（{data_dir}/thumbs/{key}.jpg，见 B-01）；
// 图片没有缩略图时回原图，视频没有缩略图时返回 404，由前端用渐变占位（F-16 / 8.风险）。
func (h *Handlers) handleThumb(c *gin.Context) {
	key := c.Param("key")
	item, found := h.media.Get(key)
	if !found {
		fail(c, http.StatusNotFound, "media_not_found", "媒体不存在")
		return
	}
	if h.thumbs != nil {
		if p := h.thumbs.Path(key); h.thumbs.Exists(key) {
			if f, err := os.Open(p); err == nil {
				defer f.Close()
				if st, err := f.Stat(); err == nil && !st.IsDir() {
					c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
					c.Writer.Header().Set("Content-Type", "image/jpeg")
					http.ServeContent(c.Writer, c.Request, filepath.Base(p), st.ModTime(), f)
					return
				}
			}
		}
	}
	if item.Type == models.MediaImage {
		abs, err := media.ResolveVerified(item)
		if err != nil {
			fail(c, http.StatusNotFound, models.ReasonFileMissing, "媒体文件已丢失")
			return
		}
		f, err := os.Open(abs)
		if err != nil {
			fail(c, http.StatusInternalServerError, "open_failed", "打开图片失败")
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
		c.Writer.Header().Set("Content-Type", contentTypeByExt(abs))
		var modTime time.Time
		if st != nil {
			modTime = st.ModTime()
		}
		http.ServeContent(c.Writer, c.Request, filepath.Base(abs), modTime, f)
		return
	}
	// 视频缩略图：预留给后续缩略图生成（存放于数据目录 thumbs/{key}.jpg）。
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		p := filepath.Join(h.cfg.App.DataDir, "thumbs", key+ext)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			defer f.Close()
			c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
			c.Writer.Header().Set("Content-Type", contentTypeByExt(p))
			http.ServeContent(c.Writer, c.Request, filepath.Base(p), st.ModTime(), f)
			return
		}
	}
	fail(c, http.StatusNotFound, "thumb_unavailable", "暂无缩略图")
}

func reasonMessage(reason string) string {
	switch reason {
	case models.ReasonDisabledByAdmin:
		return "该内容已被管理员禁用"
	case models.ReasonOutOfWindow:
		return "当前不在可播放时间段"
	case models.ReasonFileMissing:
		return "媒体文件已丢失"
	default:
		return "当前不可播放"
	}
}

func contentTypeByExt(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".avif":
		return "image/avif"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".ogv":
		return "video/ogg"
	default:
		return "application/octet-stream"
	}
}
