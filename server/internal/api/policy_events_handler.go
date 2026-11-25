package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// handlePolicyVersion GET /api/v1/events/policy/version
// 供 SSE 断线重连与不支持 EventSource 的环境兜底轮询。
func (h *Handlers) handlePolicyVersion(c *gin.Context) {
	ok(c, gin.H{"version": h.media.PolicyVersion()})
}

// handlePolicyEvents GET /api/v1/events/policy
// 单向 SSE：策略变更时只广播版本号，客户端再调用 /media/status 重新判定自己可见的视频。
// 不推送用户、路径或策略细节，避免把管理端信息泄漏给普通用户。
func (h *Handlers) handlePolicyEvents(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		fail(c, http.StatusInternalServerError, "stream_unsupported", "当前环境不支持事件流")
		return
	}
	changes, cleanup := h.media.SubscribePolicy()
	defer cleanup()

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", "text/event-stream; charset=utf-8")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// EventSource 只识别事件流里的 retry 字段；短重连间隔让视频流释放连接后快速补上。
	_, _ = fmt.Fprint(c.Writer, "retry: 2000\n\n")
	flusher.Flush()

	// 首帧带当前版本：连接建立前发生的变更不会丢失。
	writePolicyEvent(c.Writer, flusher, policyEventData{Version: h.media.PolicyVersion(), ChangedAt: time.Now()})

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			// 具名 ping 让前端能区分「连接正常」与「半开连接」。
			_, _ = fmt.Fprint(c.Writer, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		case change, open := <-changes:
			if !open {
				return
			}
			writePolicyEvent(c.Writer, flusher, policyEventData{Version: change.Version, ChangedAt: change.ChangedAt})
		}
	}
}

type policyEventData struct {
	Version   uint64    `json:"version"`
	ChangedAt time.Time `json:"changed_at"`
}

func writePolicyEvent(w http.ResponseWriter, flusher http.Flusher, data policyEventData) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: policy\nid: %s\ndata: %s\n\n", strconv.FormatUint(data.Version, 10), payload)
	flusher.Flush()
}
