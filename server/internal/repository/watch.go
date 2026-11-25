package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// ---------- 观看历史（按用户隔离） ----------

// GetWatchHistory 查询某用户对某条的观看记录。
func (s *Store) GetWatchHistory(ctx context.Context, userID int64, videoKey string) (*models.WatchHistory, error) {
	var w models.WatchHistory
	var completed int
	var last, created string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, video_key, rel_path, media_type, root_index, folder_path,
		        duration, position, watched_seconds, play_count, completed, last_played_at, created_at
		 FROM watch_history WHERE user_id = ? AND video_key = ?`, userID, videoKey).
		Scan(&w.ID, &w.UserID, &w.VideoKey, &w.RelPath, &w.MediaType, &w.RootIndex, &w.FolderPath,
			&w.Duration, &w.Position, &w.WatchedSeconds, &w.PlayCount, &completed, &last, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	w.Completed = completed == 1
	w.LastPlayedAt = parseTime(last)
	w.CreatedAt = parseTime(created)
	return &w, nil
}

// ListWatchHistory 查询用户观看历史，按最近播放倒序。
func (s *Store) ListWatchHistory(ctx context.Context, userID int64, limit int) ([]models.WatchHistory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, video_key, rel_path, media_type, root_index, folder_path,
		        duration, position, watched_seconds, play_count, completed, last_played_at, created_at
		 FROM watch_history WHERE user_id = ? ORDER BY last_played_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.WatchHistory, 0, limit)
	for rows.Next() {
		var w models.WatchHistory
		var completed int
		var last, created string
		if err := rows.Scan(&w.ID, &w.UserID, &w.VideoKey, &w.RelPath, &w.MediaType, &w.RootIndex, &w.FolderPath,
			&w.Duration, &w.Position, &w.WatchedSeconds, &w.PlayCount, &completed, &last, &created); err != nil {
			return nil, err
		}
		w.Completed = completed == 1
		w.LastPlayedAt = parseTime(last)
		w.CreatedAt = parseTime(created)
		out = append(out, w)
	}
	return out, rows.Err()
}

// ListWatchHistoryKeys 返回用户观看过的所有 key（供进度批量查询）。
func (s *Store) ListWatchHistoryKeys(ctx context.Context, userID int64) (map[string]models.WatchHistory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT video_key, duration, position, watched_seconds, play_count, completed, last_played_at
		 FROM watch_history WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.WatchHistory{}
	for rows.Next() {
		var w models.WatchHistory
		var completed int
		var last string
		if err := rows.Scan(&w.VideoKey, &w.Duration, &w.Position, &w.WatchedSeconds, &w.PlayCount, &completed, &last); err != nil {
			return nil, err
		}
		w.Completed = completed == 1
		w.LastPlayedAt = parseTime(last)
		out[w.VideoKey] = w
	}
	return out, rows.Err()
}

// UpsertWatchProgress 上报观看进度。
//
// 完播只按「累计真实观看时长 >= 视频时长」判定，拖动进度条到末尾不会增加 watchedDelta，
// 因此不会误判为完播。completed 入参只保留兼容，不再作为判定依据。
// restart=true 时把进度、观看时长与完播状态清零（"从头开始"按钮）。
func (s *Store) UpsertWatchProgress(ctx context.Context, userID int64, in WatchProgressInput) error {
	now := s.now()
	delta := in.WatchedDelta
	if delta < 0 {
		delta = 0
	}
	if in.Restart {
		delta = 0
	}

	var prevWatched, prevDuration float64
	var prevCompleted int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(watched_seconds,0), COALESCE(duration,0), completed
		 FROM watch_history WHERE user_id = ? AND video_key = ?`, userID, in.VideoKey).
		Scan(&prevWatched, &prevDuration, &prevCompleted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	duration := in.Duration
	if duration <= 0 {
		duration = prevDuration
	}
	totalWatched := prevWatched + delta
	completeNow := duration > 0 && totalWatched >= duration*0.98
	completed := 0
	completeInc := 0
	if completeNow && !in.Restart {
		completed = 1
		if prevCompleted == 0 {
			completeInc = 1
		}
	}

	// 播放次数只在「首次开始播放」或「从头重播」时 +1，普通进度上报不累加。
	playInc := 0
	if in.First || in.Restart {
		playInc = 1
	}
	position := in.Position
	if in.Restart {
		position, completed, completeInc = 0, 0, 0
	}
	initPlay := playInc
	if initPlay < 1 {
		initPlay = 1 // 首次插入这条记录时，播放次数至少为 1
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO watch_history
		 (user_id, video_key, rel_path, media_type, root_index, folder_path, duration,
		  position, watched_seconds, play_count, completed, last_played_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, video_key) DO UPDATE SET
		   rel_path=excluded.rel_path, media_type=excluded.media_type,
		   root_index=excluded.root_index, folder_path=excluded.folder_path,
		   duration=excluded.duration,
		   position=CASE WHEN ?=1 THEN 0 ELSE excluded.position END,
		   watched_seconds=CASE WHEN ?=1 THEN 0 ELSE watch_history.watched_seconds + excluded.watched_seconds END,
		   play_count=watch_history.play_count + ?,
		   completed=CASE WHEN ?=1 THEN 0 WHEN excluded.completed=1 THEN 1 ELSE watch_history.completed END,
		   last_played_at=excluded.last_played_at`,
		userID, in.VideoKey, in.RelPath, in.MediaType, in.RootIndex, in.FolderPath, in.Duration,
		position, delta, initPlay, completed, now, now,
		b2i(in.Restart), b2i(in.Restart), playInc, b2i(in.Restart))
	if err != nil {
		return err
	}
	if err := s.insertWatchEvent(ctx, userID, in, delta, playInc, completeInc, now); err != nil {
		return err
	}
	if err := s.upsertMediaStats(ctx, in, delta, playInc, completeInc, now); err != nil {
		return err
	}
	return s.upsertFolderStats(ctx, in, delta, playInc, completeInc, now)
}

// insertWatchEvent 记录进度增量流水，供今日全站统计使用。
func (s *Store) insertWatchEvent(ctx context.Context, userID int64, in WatchProgressInput,
	delta float64, playInc, completeInc int, now string) error {
	if delta <= 0 && playInc <= 0 && completeInc <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO watch_events
		 (user_id, video_key, root_index, folder_path, watched_delta, play_inc, completed_inc, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, in.VideoKey, in.RootIndex, in.FolderPath, delta, playInc, completeInc, now)
	return err
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

// upsertMediaStats 更新媒体聚合统计（供管理端与推荐）。
func (s *Store) upsertMediaStats(ctx context.Context, in WatchProgressInput, delta float64, playInc, completeInc int, now string) error {
	// 完播数只在累计观看时长首次达标时 +1，避免重复上报虚高。
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO media_stats
		 (video_key, play_count, completed_count, watched_seconds, favorite_count, last_played_at, updated_at)
		 VALUES (?, ?, ?, ?, (SELECT COUNT(1) FROM favorites WHERE video_key = ?), ?, ?)
		 ON CONFLICT(video_key) DO UPDATE SET
		   play_count = media_stats.play_count + excluded.play_count,
		   completed_count = media_stats.completed_count + excluded.completed_count,
		   watched_seconds = media_stats.watched_seconds + excluded.watched_seconds,
		   favorite_count = (SELECT COUNT(1) FROM favorites WHERE video_key = ?),
		   last_played_at = excluded.last_played_at,
		   updated_at = excluded.updated_at`,
		in.VideoKey, playInc, completeInc, delta, in.VideoKey, now, now, in.VideoKey)
	return err
}

// upsertFolderStats 实时同步文件夹聚合统计（供管理端与推荐）。
func (s *Store) upsertFolderStats(ctx context.Context, in WatchProgressInput, delta float64, playInc, completeInc int, now string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO folder_stats
		 (root_index, folder_path, play_count, completed_count, watched_seconds, favorite_count, media_count, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, 1, ?)
		 ON CONFLICT(root_index, folder_path) DO UPDATE SET
		   play_count = folder_stats.play_count + excluded.play_count,
		   completed_count = folder_stats.completed_count + excluded.completed_count,
		   watched_seconds = folder_stats.watched_seconds + excluded.watched_seconds,
		   updated_at = excluded.updated_at`,
		in.RootIndex, in.FolderPath, playInc, completeInc, delta, now)
	return err
}

// RefreshFavoriteCounts 同步媒体与文件夹的收藏计数（收藏增删后调用）。
func (s *Store) RefreshFavoriteCounts(ctx context.Context, videoKey string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE media_stats SET favorite_count = (SELECT COUNT(1) FROM favorites WHERE video_key = ?), updated_at = ?
		 WHERE video_key = ?`, videoKey, s.now(), videoKey)
	return err
}

// ClearWatchHistory 清空用户观看历史。
func (s *Store) ClearWatchHistory(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM watch_history WHERE user_id = ?`, userID)
	return err
}

// RemoveWatchHistory 删除用户单条观看记录。
func (s *Store) RemoveWatchHistory(ctx context.Context, userID int64, videoKey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM watch_history WHERE user_id = ? AND video_key = ?`, userID, videoKey)
	return err
}

// WatchProgressInput 进度上报入参。
type WatchProgressInput struct {
	VideoKey   string  `json:"video_key"`
	RelPath    string  `json:"rel_path"`
	MediaType  string  `json:"media_type"`
	RootIndex  int     `json:"root_index"`
	FolderPath string  `json:"folder_path"`
	Duration   float64 `json:"duration"`
	Position   float64 `json:"position"`
	// WatchedDelta 距上次上报新增的观看秒数（前端节流到约 5 秒上报一次）。
	WatchedDelta float64 `json:"watched_delta"`
	Completed    bool    `json:"completed"`
	Restart      bool    `json:"restart"`
	// First 表示这是本次会话第一次真实播放该条（用于准确计播放次数；进度上报不带）。
	First bool `json:"first"`
	// Started 表示播放器确实触发过 play 事件；预加载/缩略图抽帧上报该值为 false。
	Started bool `json:"started"`
}

// ---------- 用户观看统计 ----------

// UserWatchSummary 用户观看汇总。
type UserWatchSummary struct {
	WatchedVideos   int     `json:"watched_videos"`
	CompletedVideos int     `json:"completed_videos"`
	TotalWatchTime  float64 `json:"total_watch_time"`
}

// GetUserWatchSummary 汇总某用户的观看数据。
func (s *Store) GetUserWatchSummary(ctx context.Context, userID int64) (UserWatchSummary, error) {
	var v UserWatchSummary
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1),
		        COALESCE(SUM(CASE WHEN completed=1 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(watched_seconds), 0)
		 FROM watch_history WHERE user_id = ?`, userID).
		Scan(&v.WatchedVideos, &v.CompletedVideos, &v.TotalWatchTime)
	return v, err
}

// ---------- 全站 / 管理端统计 ----------

// UserWatchRank 用户观看排行条目（管理端统计）。
type UserWatchRank struct {
	UserID          int64   `json:"user_id"`
	Username        string  `json:"username"`
	WatchedVideos   int     `json:"watched_videos"`
	CompletedVideos int     `json:"completed_videos"`
	TotalWatchTime  float64 `json:"total_watch_time"`
	LastPlayedAt    string  `json:"last_played_at"`
}

// ListUserWatchRank 返回用户观看排行（管理端控制台）。
func (s *Store) ListUserWatchRank(ctx context.Context, limit int) ([]UserWatchRank, error) {
	q := `SELECT w.user_id, COALESCE(u.username, ''), COUNT(DISTINCT w.video_key),
	             COALESCE(SUM(CASE WHEN w.completed=1 THEN 1 ELSE 0 END), 0),
	             COALESCE(SUM(w.watched_seconds), 0), MAX(w.last_played_at)
	      FROM watch_history w LEFT JOIN users u ON u.id = w.user_id
	      GROUP BY w.user_id ORDER BY SUM(w.watched_seconds) DESC`
	if limit > 0 {
		q += " LIMIT ?"
	}
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.QueryContext(ctx, q, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserWatchRank, 0, 16)
	for rows.Next() {
		var r UserWatchRank
		var last string
		if err := rows.Scan(&r.UserID, &r.Username, &r.WatchedVideos, &r.CompletedVideos,
			&r.TotalWatchTime, &last); err != nil {
			return nil, err
		}
		r.LastPlayedAt = parseTime(last).Format("2006-01-02 15:04:05")
		out = append(out, r)
	}
	return out, rows.Err()
}

// AggregateWatchSummary 全站历史观看汇总。
func (s *Store) AggregateWatchSummary(ctx context.Context) (UserWatchSummary, error) {
	var v UserWatchSummary
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id || ':' || video_key),
		        COALESCE(SUM(CASE WHEN completed=1 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(watched_seconds), 0)
		 FROM watch_history`).Scan(&v.WatchedVideos, &v.CompletedVideos, &v.TotalWatchTime)
	return v, err
}

// DailyWatchSummary 今日全站观看汇总（按进度事件时间，而非累计历史）。
type DailyWatchSummary struct {
	WatchedVideos  int     `json:"watched_videos"`
	TotalWatchTime float64 `json:"total_watch_time"`
}

// DailyWatchSummary 统计今日有观看活动的去重视频数与观看时长。
func (s *Store) DailyWatchSummary(ctx context.Context) (DailyWatchSummary, error) {
	var v DailyWatchSummary
	now := time.Now().In(s.loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	end := start.AddDate(0, 0, 1)
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id || ':' || video_key), COALESCE(SUM(watched_delta), 0)
		 FROM watch_events WHERE created_at >= ? AND created_at < ?`,
		start.Format(time.RFC3339), end.Format(time.RFC3339)).
		Scan(&v.WatchedVideos, &v.TotalWatchTime)
	return v, err
}

// ListMediaPathSnapshots 返回观看历史/收藏里保留的媒体路径快照。
// 媒体被扫描移除后 media_stats 仍保留统计行，可用这些快照补回可读文件名。
// 观看历史按 id 递增覆盖，同一个 key 取最近一次记录；收藏只补空缺。
func (s *Store) ListMediaPathSnapshots(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT video_key, rel_path FROM watch_history
		 WHERE TRIM(COALESCE(rel_path, '')) <> '' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, path string
		if err := rows.Scan(&key, &path); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out[key] = path
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx,
		`SELECT video_key, video_path FROM favorites
		 WHERE TRIM(COALESCE(video_path, '')) <> '' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, path string
		if err := rows.Scan(&key, &path); err != nil {
			return nil, err
		}
		if _, ok := out[key]; !ok {
			out[key] = path
		}
	}
	return out, rows.Err()
}

// ListMediaStats 返回媒体统计（按热度倒序）。
func (s *Store) ListMediaStats(ctx context.Context, limit int) ([]models.MediaStats, error) {
	q := `SELECT video_key, play_count, completed_count, watched_seconds, favorite_count, last_played_at, updated_at
		  FROM media_stats ORDER BY play_count DESC, watched_seconds DESC`
	if limit > 0 {
		q += " LIMIT ?"
	}
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.QueryContext(ctx, q, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MediaStats, 0, 32)
	for rows.Next() {
		var m models.MediaStats
		var last, updated string
		if err := rows.Scan(&m.VideoKey, &m.PlayCount, &m.CompletedCount, &m.WatchedSeconds,
			&m.FavoriteCount, &last, &updated); err != nil {
			return nil, err
		}
		m.LastPlayedAt = parseTime(last)
		m.UpdatedAt = parseTime(updated)
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMediaStats 返回单个媒体的统计。
func (s *Store) GetMediaStats(ctx context.Context, videoKey string) (*models.MediaStats, error) {
	var m models.MediaStats
	var last, updated string
	err := s.db.QueryRowContext(ctx,
		`SELECT video_key, play_count, completed_count, watched_seconds, favorite_count, last_played_at, updated_at
		 FROM media_stats WHERE video_key = ?`, videoKey).
		Scan(&m.VideoKey, &m.PlayCount, &m.CompletedCount, &m.WatchedSeconds, &m.FavoriteCount, &last, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.LastPlayedAt = parseTime(last)
	m.UpdatedAt = parseTime(updated)
	return &m, nil
}

// UpsertFolderStats 聚合更新文件夹统计（从 watch_history + favorites 汇总）。
func (s *Store) RebuildFolderStats(ctx context.Context) error {
	now := s.now()
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM folder_stats`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO folder_stats
		 (root_index, folder_path, play_count, completed_count, watched_seconds, favorite_count, media_count, updated_at)
		 SELECT root_index, folder_path,
		        SUM(play_count), SUM(completed), SUM(watched_seconds), 0, COUNT(DISTINCT video_key), ?
		 FROM watch_history GROUP BY root_index, folder_path`, now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE folder_stats SET favorite_count =
		 (SELECT COUNT(1) FROM favorites f WHERE f.video_key IN
		   (SELECT video_key FROM watch_history w WHERE w.root_index = folder_stats.root_index AND w.folder_path = folder_stats.folder_path))`)
	return err
}

// ListFolderStats 返回文件夹统计。
func (s *Store) ListFolderStats(ctx context.Context) ([]models.FolderStats, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT root_index, folder_path, play_count, completed_count, watched_seconds, favorite_count, media_count, updated_at
		 FROM folder_stats ORDER BY play_count DESC, watched_seconds DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.FolderStats, 0, 32)
	for rows.Next() {
		var f models.FolderStats
		var updated string
		if err := rows.Scan(&f.RootIndex, &f.FolderPath, &f.PlayCount, &f.CompletedCount,
			&f.WatchedSeconds, &f.FavoriteCount, &f.MediaCount, &updated); err != nil {
			return nil, err
		}
		f.UpdatedAt = parseTime(updated)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---------- 媒体根选项（递归导入） ----------

// GetRootRecursive 返回根是否递归导入（默认 true）。
func (s *Store) GetRootRecursive(ctx context.Context, rootIndex int) bool {
	var v int
	if err := s.db.QueryRowContext(ctx,
		`SELECT recursive FROM media_root_options WHERE root_index = ?`, rootIndex).Scan(&v); err != nil {
		return true
	}
	return v == 1
}

// SetRootRecursive 设置根是否递归导入。
func (s *Store) SetRootRecursive(ctx context.Context, rootIndex int, recursive bool) error {
	v := 0
	if recursive {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO media_root_options (root_index, recursive, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(root_index) DO UPDATE SET recursive = excluded.recursive, updated_at = excluded.updated_at`,
		rootIndex, v, s.now())
	return err
}

// GetAllRootOptions 返回所有根选项。
func (s *Store) GetAllRootOptions(ctx context.Context) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT root_index, recursive FROM media_root_options`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var idx, v int
		if err := rows.Scan(&idx, &v); err != nil {
			return nil, err
		}
		out[idx] = v == 1
	}
	return out, rows.Err()
}

// Placeholder helpers to keep imports tidy across build tags.
var (
	_ = errors.Is
	_ = strings.TrimSpace
)
