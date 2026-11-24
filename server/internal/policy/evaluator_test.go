package policy

import (
	"context"
	"testing"
	"time"

	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/models"
)

type fakeSource struct {
	folders []models.FolderPolicy
	files   map[string]models.FileOverride
}

func (f *fakeSource) ListFolderPolicies(context.Context) ([]models.FolderPolicy, error) {
	return f.folders, nil
}

func (f *fakeSource) GetFileOverride(_ context.Context, key string) (*models.FileOverride, error) {
	if o, ok := f.files[key]; ok {
		return &o, nil
	}
	return nil, nil
}

func (f *fakeSource) ListFileOverrides(context.Context) (map[string]models.FileOverride, error) {
	return f.files, nil
}

func TestInWindow(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 3, 1, h, m, 0, 0, loc) }

	cases := []struct {
		name       string
		now        time.Time
		start, end string
		want       bool
	}{
		{"窗口内", at(19, 30), "18:00", "23:00", true},
		{"窗口外-早", at(17, 59), "18:00", "23:00", false},
		{"窗口外-晚", at(23, 0), "18:00", "23:00", false},
		{"跨零点-深夜", at(1, 0), "22:00", "02:00", true},
		{"跨零点-白天", at(12, 0), "22:00", "02:00", false},
		{"起止相同视为全天", at(9, 0), "08:00", "08:00", true},
		{"格式非法不额外限制", at(9, 0), "", "", true},
	}
	for _, c := range cases {
		if got := InWindow(c.now, c.start, c.end, loc); got != c.want {
			t.Errorf("%s: InWindow(%s, %s-%s) = %v, want %v",
				c.name, c.now.Format("15:04"), c.start, c.end, got, c.want)
		}
	}
}

func TestEvaluatePrecedence(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, loc)

	rootDisabled := models.FolderPolicy{
		FolderPath: "", Enabled: false, ScheduleType: models.ScheduleDisabled,
	}
	subAlways := models.FolderPolicy{
		FolderPath: "sub", Enabled: true, ScheduleType: models.ScheduleAlways,
	}
	deepWindow := models.FolderPolicy{
		FolderPath: "sub/deep", Enabled: true, ScheduleType: models.ScheduleTimeWindow,
		TimeStart: "18:00", TimeEnd: "23:00",
	}
	src := &fakeSource{
		folders: []models.FolderPolicy{rootDisabled, subAlways, deepWindow},
		files: map[string]models.FileOverride{
			"file-disabled-key": {
				VideoKey: "file-disabled-key", Enabled: false, ScheduleType: models.ScheduleDisabled,
			},
			"file-ok-key": {
				VideoKey: "file-ok-key", Enabled: true, ScheduleType: models.ScheduleAlways,
			},
		},
	}
	e := NewEvaluator(src, loc, nil)

	item := func(key, rel string) *media.Item {
		return &media.Item{Key: key, RelPath: rel, Type: models.MediaVideo}
	}

	// 根目录策略作为全局默认，覆盖未单独配置的目录
	if d := e.Evaluate(context.Background(), item("k1", "other/a.mp4"), false, now); d.Playable {
		t.Errorf("根目录禁用应影响未配置的子目录，实际 %+v", d)
	}
	// 子目录策略覆盖根目录
	if d := e.Evaluate(context.Background(), item("k2", "sub/a.mp4"), false, now); !d.Playable {
		t.Errorf("子目录 always 策略应覆盖根目录禁用，实际 %+v", d)
	}
	// 最长前缀优先：sub/deep 的时间窗口优先于 sub
	if d := e.Evaluate(context.Background(), item("k3", "sub/deep/a.mp4"), false, now); d.Playable || d.Reason != models.ReasonOutOfWindow {
		t.Errorf("应命中 sub/deep 时间窗口并被拦截，实际 %+v", d)
	}
	// 文件级覆盖 > 文件夹策略
	if d := e.Evaluate(context.Background(), item("file-ok-key", "other/a.mp4"), false, now); !d.Playable {
		t.Errorf("文件级启用应覆盖文件夹禁用，实际 %+v", d)
	}
	if d := e.Evaluate(context.Background(), item("file-disabled-key", "sub/a.mp4"), false, now); d.Playable || d.Reason != models.ReasonDisabledByAdmin {
		t.Errorf("文件级禁用应优先，实际 %+v", d)
	}
	// 管理员不受策略限制
	if d := e.Evaluate(context.Background(), item("k1", "other/a.mp4"), true, now); !d.Playable {
		t.Errorf("管理员应始终可播放，实际 %+v", d)
	}
	// 缺少媒体条目
	if d := e.Evaluate(context.Background(), nil, false, now); d.Playable || d.Reason != models.ReasonFileMissing {
		t.Errorf("空条目应返回 file_missing，实际 %+v", d)
	}
}

func TestEvaluateCacheInvalidation(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, loc)
	src := &fakeSource{}
	e := NewEvaluator(src, loc, nil)
	it := &media.Item{Key: "k", RelPath: "a.mp4"}

	if d := e.Evaluate(context.Background(), it, false, now); !d.Playable {
		t.Fatalf("默认应允许，实际 %+v", d)
	}

	// 策略变更：写入后 Invalidate 应立即生效
	src.folders = []models.FolderPolicy{{FolderPath: "", Enabled: false, ScheduleType: models.ScheduleDisabled}}
	e.Invalidate()
	if d := e.Evaluate(context.Background(), it, false, now); d.Playable {
		t.Fatalf("失效缓存后应读取到新策略，实际 %+v", d)
	}
}
