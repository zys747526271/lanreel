package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/models"
)

// handleAdminThumbStats GET /api/v1/admin/thumbs/stats
func (h *Handlers) handleAdminThumbStats(c *gin.Context) {
	stats := h.thumbs.Stats()
	snap := h.scanner.Snapshot()
	videos, images, videoWith, imageWith := 0, 0, 0, 0
	for _, it := range snap.Items {
		has := h.thumbs.Exists(it.Key)
		if it.Type == models.MediaVideo {
			videos++
			if has {
				videoWith++
			}
		} else {
			images++
			if has {
				imageWith++
			}
		}
	}
	ok(c, gin.H{
		"stats":        stats,
		"videos":       videos,
		"images":       images,
		"video_thumbs": videoWith,
		"image_thumbs": imageWith,
	})
}

// handleAdminThumbRebuild POST /api/v1/admin/thumbs/rebuild
//
// 重新生成全部缩略图（覆盖已存在的文件），在后台顺序执行；
// 前端通过 /admin/thumbs/stats 轮询进度。
func (h *Handlers) handleAdminThumbRebuild(c *gin.Context) {
	if !h.thumbs.Stats().Enabled {
		fail(c, http.StatusBadRequest, "thumbnails_disabled", "缩略图功能已在配置中关闭")
		return
	}
	items := h.scanner.Snapshot().Items
	go func() {
		for _, it := range items {
			if err := h.thumbs.Generate(it); err != nil {
				h.log.Warn(logging.CatScan, "media.thumb", "重新生成缩略图失败", map[string]any{
					"video_key": it.Key, "error": err.Error(),
				})
			}
		}
		h.log.Info(logging.CatScan, "media.thumb", "缩略图重建完成", map[string]any{"total": len(items)})
	}()
	ok(c, gin.H{"queued": len(items)})
}
