package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zys747526271/lanreel/server/internal/policy"
	"github.com/zys747526271/lanreel/server/internal/service"
)

func TestHandlePolicyEventsBroadcastsVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eval := policy.NewEvaluator(nil, time.Local, nil)
	h := &Handlers{media: service.NewMediaService(nil, eval, nil, nil, nil)}
	initial := eval.Version()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/events/policy", nil).WithContext(ctx)

	go func() {
		time.Sleep(50 * time.Millisecond)
		eval.Invalidate()
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	h.handlePolicyEvents(c)

	body := w.Body.String()
	if !strings.Contains(body, "event: policy") {
		t.Fatalf("响应缺少 policy 事件: %q", body)
	}
	if !strings.Contains(body, `"version":`+strconv.FormatUint(initial, 10)) {
		t.Fatalf("响应缺少初始版本 %d: %q", initial, body)
	}
	if !strings.Contains(body, fmt.Sprintf(`"version":%d`, initial+1)) {
		t.Fatalf("响应缺少变更后版本 %d: %q", initial+1, body)
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestHandlePolicyVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eval := policy.NewEvaluator(nil, time.Local, nil)
	h := &Handlers{media: service.NewMediaService(nil, eval, nil, nil, nil)}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/events/policy/version", nil)

	h.handlePolicyVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"version":1`) {
		t.Fatalf("版本响应不正确: %q", w.Body.String())
	}
}
