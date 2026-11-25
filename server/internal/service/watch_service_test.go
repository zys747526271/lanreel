package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/zys747526271/lanreel/server/internal/database"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/repository"
)

// newHeatTestService 构造带真实业务库的热度服务；空扫描快照表示媒体文件已被移除。
func newHeatTestService(t *testing.T) (*WatchService, *sql.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "heat.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.MigrateBusiness(db); err != nil {
		t.Fatalf("迁移测试库失败: %v", err)
	}
	store := repository.New(db, time.Local)
	return NewWatchService(store, NewMediaService(&media.Scanner{}, nil, nil, nil, store)), db
}

func heatTestExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("写入测试数据失败: %v", err)
	}
}

func TestHeatDisplayName(t *testing.T) {
	cases := []struct {
		name, rel, key, want string
	}{
		{"posix", "root/sub/电影.mp4", "k", "电影.mp4"},
		{"windows", `root\sub\电影.mp4`, "k", "电影.mp4"},
		{"trailing slash", "root/sub/", "k", "sub"},
		{"root only", "/", "k", "k"},
		{"blank path", "  ", "fallback-key", "fallback-key"},
		{"all blank", "", "", "未命名媒体"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heatDisplayName(tc.rel, tc.key); got != tc.want {
				t.Fatalf("heatDisplayName(%q, %q) = %q, want %q", tc.rel, tc.key, got, tc.want)
			}
		})
	}
}

func TestMediaHeatPagedMissingNames(t *testing.T) {
	svc, db := newHeatTestService(t)
	ctx := context.Background()
	now := time.Now().Format(time.RFC3339)
	heatTestExec(t, db, `INSERT INTO users(id, username, password_hash, role, created_at) VALUES (1, 'u', 'x', 'user', ?)`, now)
	for _, key := range []string{"k-history", "k-favorite", "k-key-only"} {
		heatTestExec(t, db,
			`INSERT INTO media_stats(video_key, play_count, completed_count, watched_seconds, favorite_count, last_played_at, updated_at)
			 VALUES (?, 0, 0, 0, 0, ?, ?)`, key, now, now)
	}
	heatTestExec(t, db,
		`INSERT INTO watch_history(user_id, video_key, rel_path, last_played_at, created_at)
		 VALUES (1, 'k-history', 'B/历史片段.mp4', ?, ?)`, now, now)
	heatTestExec(t, db,
		`INSERT INTO favorites(user_id, video_key, video_path, created_at)
		 VALUES (1, 'k-favorite', 'C/收藏片段.mp4', ?)`, now)

	// 名称升序与播放升序都会把这些 0 播放的孤立统计排到前面，名称不能为空。
	for _, sortKey := range []string{"name", "play", "heat"} {
		page, err := svc.MediaHeatPaged(ctx, sortKey, false, 1, 10, false)
		if err != nil {
			t.Fatalf("%s 升序查询失败: %v", sortKey, err)
		}
		if page.Total != 3 || len(page.Items) != 3 {
			t.Fatalf("%s 升序返回 total=%d items=%d, want 3/3", sortKey, page.Total, len(page.Items))
		}
		for _, item := range page.Items {
			if item.DisplayName == "" {
				t.Fatalf("%s 升序中 %s 的显示名为空", sortKey, item.Key)
			}
			if !item.Missing || item.RelPath != "" {
				t.Fatalf("%s 应标记为已移除且不携带当前路径: %+v", item.Key, item)
			}
		}
	}

	page, err := svc.MediaHeatPaged(ctx, "name", false, 1, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]string{}
	for _, item := range page.Items {
		byKey[item.Key] = item.DisplayName
	}
	if byKey["k-history"] != "已移除 · 历史片段.mp4" {
		t.Fatalf("观看历史路径未恢复: %q", byKey["k-history"])
	}
	if byKey["k-favorite"] != "已移除 · 收藏片段.mp4" {
		t.Fatalf("收藏路径未恢复: %q", byKey["k-favorite"])
	}
	if byKey["k-key-only"] != "已移除 · k-key-only" {
		t.Fatalf("无路径时应退回 key 并标记已移除: %q", byKey["k-key-only"])
	}
}

func TestMediaHeatPagedDescendingIsStable(t *testing.T) {
	svc, db := newHeatTestService(t)
	ctx := context.Background()
	now := time.Now().Format(time.RFC3339)
	for _, key := range []string{"a", "b", "c"} {
		heatTestExec(t, db,
			`INSERT INTO media_stats(video_key, play_count, completed_count, watched_seconds, favorite_count, last_played_at, updated_at)
			 VALUES (?, 1, 0, 0, 0, ?, ?)`, key, now, now)
	}
	for _, sortKey := range []string{"name", "play", "heat", "complete", "watched", "favorite"} {
		page, err := svc.MediaHeatPaged(ctx, sortKey, true, 1, 10, false)
		if err != nil {
			t.Fatalf("%s 倒序查询失败: %v", sortKey, err)
		}
		if len(page.Items) != 3 {
			t.Fatalf("%s 倒序返回 %d 条, want 3", sortKey, len(page.Items))
		}
		for i := 1; i < len(page.Items); i++ {
			if heatSortName(page.Items[i-1]) < heatSortName(page.Items[i]) && sortKey == "name" {
				t.Fatalf("name 倒序结果未按显示名排列: %+v", page.Items)
			}
		}
	}
}

func TestListMediaPathSnapshotsPrefersLatestHistory(t *testing.T) {
	svc, db := newHeatTestService(t)
	ctx := context.Background()
	now := time.Now().Format(time.RFC3339)
	heatTestExec(t, db, `INSERT INTO users(id, username, password_hash, role, created_at) VALUES (1, 'u1', 'x', 'user', ?)`, now)
	heatTestExec(t, db, `INSERT INTO users(id, username, password_hash, role, created_at) VALUES (2, 'u2', 'x', 'user', ?)`, now)
	for i, path := range []string{"old/旧路径.mp4", "new/新路径.mp4"} {
		heatTestExec(t, db,
			`INSERT INTO watch_history(user_id, video_key, rel_path, last_played_at, created_at)
			 VALUES (?, 'k', ?, ?, ?)`, i+1, path, now, now)
	}
	heatTestExec(t, db,
		`INSERT INTO favorites(user_id, video_key, video_path, created_at)
		 VALUES (1, 'k', 'fav/收藏路径.mp4', ?)`, now)
	heatTestExec(t, db,
		`INSERT INTO favorites(user_id, video_key, video_path, created_at)
		 VALUES (1, 'fav-only', 'fav/仅收藏.mp4', ?)`, now)

	snapshots, err := svc.store.ListMediaPathSnapshots(ctx)
	if err != nil {
		t.Fatalf("读取路径快照失败: %v", err)
	}
	if snapshots["k"] != "new/新路径.mp4" {
		t.Fatalf("应取最近的观看历史路径，得到 %q", snapshots["k"])
	}
	if snapshots["fav-only"] != "fav/仅收藏.mp4" {
		t.Fatalf("收藏快照未补齐，得到 %q", snapshots["fav-only"])
	}
}

func TestMediaHeatPagedHideMissing(t *testing.T) {
	svc, db := newHeatTestService(t)
	ctx := context.Background()
	now := time.Now().Format(time.RFC3339)
	// 空扫描快照表示这些统计都属于已移除文件。
	for _, key := range []string{"gone-a", "gone-b", "gone-c"} {
		heatTestExec(t, db,
			`INSERT INTO media_stats(video_key, play_count, completed_count, watched_seconds, favorite_count, last_played_at, updated_at)
			 VALUES (?, 0, 0, 0, 0, ?, ?)`, key, now, now)
	}
	shown, err := svc.MediaHeatPaged(ctx, "name", false, 1, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Total != 3 || len(shown.Items) != 3 {
		t.Fatalf("关闭隐藏时应返回 3 条, 得到 total=%d items=%d", shown.Total, len(shown.Items))
	}
	hidden, err := svc.MediaHeatPaged(ctx, "name", false, 1, 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Total != 0 || len(hidden.Items) != 0 {
		t.Fatalf("开启隐藏后 total/items 应为 0, 得到 total=%d items=%d", hidden.Total, len(hidden.Items))
	}
}
