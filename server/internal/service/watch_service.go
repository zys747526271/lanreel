package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/repository"
)

// WatchService 观看进度、历史与统计业务（v0.3.0）。
type WatchService struct {
	store *repository.Store
	media *MediaService
}

func NewWatchService(store *repository.Store, mediaSvc *MediaService) *WatchService {
	return &WatchService{store: store, media: mediaSvc}
}

// HistoryEntry 返回给前端的观看历史条目（含实时可播放状态与进度）。
type HistoryEntry struct {
	Key            string  `json:"key"`
	MediaType      string  `json:"media_type"`
	RelPath        string  `json:"rel_path"`
	RootIndex      int     `json:"root_index"`
	RootName       string  `json:"root_name"`
	Folder         string  `json:"folder"`
	Duration       float64 `json:"duration"`
	Position       float64 `json:"position"`
	Progress       float64 `json:"progress"`
	PlayCount      int     `json:"play_count"`
	Completed      bool    `json:"completed"`
	Playable       bool    `json:"playable"`
	Reason         string  `json:"reason,omitempty"`
	StreamURL      string  `json:"stream_url"`
	ThumbURL       string  `json:"thumb_url"`
	LastPlayedAt   string  `json:"last_played_at"`
	FavoriteCount  int     `json:"favorite_count"`
}

// ListHistory 返回用户观看历史（服务端、按用户隔离），带实时权限与进度。
func (s *WatchService) ListHistory(ctx context.Context, u *models.User, limit int) ([]HistoryEntry, error) {
	rows, err := s.store.ListWatchHistory(ctx, u.ID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryEntry, 0, len(rows))
	for _, w := range rows {
		it, found := s.media.Get(w.VideoKey)
		entry := HistoryEntry{
			Key: w.VideoKey, MediaType: w.MediaType, RelPath: w.RelPath,
			RootIndex: w.RootIndex, Folder: w.FolderPath, Duration: w.Duration,
			Position: w.Position, PlayCount: w.PlayCount, Completed: w.Completed,
			Playable: true,
			LastPlayedAt: w.LastPlayedAt.Format(time.RFC3339),
		}
		if w.Duration > 0 {
			entry.Progress = w.Position / w.Duration
			if entry.Progress > 1 {
				entry.Progress = 1
			}
		}
		if found {
			entry.RootName = it.RootName
			entry.RelPath = it.RelPath
			entry.Folder = it.Folder()
			entry.MediaType = it.Type
			entry.Duration = it.Duration
			entry.StreamURL = "/api/v1/media/" + it.Key + "/stream"
			entry.ThumbURL = "/api/v1/media/" + it.Key + "/thumb"
			d := s.media.Decide(ctx, u, it)
			entry.Playable, entry.Reason = d.Playable, d.Reason
		} else {
			entry.Playable = false
			entry.Reason = models.ReasonFileMissing
			entry.StreamURL = "/api/v1/media/" + w.VideoKey + "/stream"
			entry.ThumbURL = "/api/v1/media/" + w.VideoKey + "/thumb"
		}
		if st, err := s.store.GetMediaStats(ctx, w.VideoKey); err == nil {
			entry.FavoriteCount = st.FavoriteCount
		}
		out = append(out, entry)
	}
	return out, nil
}

// ProgressMap 返回用户所有条目的进度快照（key -> 进度信息）。
func (s *WatchService) ProgressMap(ctx context.Context, u *models.User) (map[string]ProgressView, error) {
	rows, err := s.store.ListWatchHistoryKeys(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ProgressView, len(rows))
	for k, w := range rows {
		progress := 0.0
		if w.Duration > 0 {
			progress = w.Position / w.Duration
			if progress > 1 {
				progress = 1
			}
		}
		out[k] = ProgressView{
			Position: w.Position, Duration: w.Duration, Progress: progress,
			Completed: w.Completed, PlayCount: w.PlayCount,
			WatchedSeconds: w.WatchedSeconds,
		}
	}
	return out, nil
}

// ProgressView 单条进度视图。
type ProgressView struct {
	Position       float64 `json:"position"`
	Duration       float64 `json:"duration"`
	Progress       float64 `json:"progress"`
	Completed      bool    `json:"completed"`
	PlayCount      int     `json:"play_count"`
	WatchedSeconds float64 `json:"watched_seconds"`
}

// ReportProgress 上报观看进度，使用媒体条目补全元数据后落库。
func (s *WatchService) ReportProgress(ctx context.Context, u *models.User, in repository.WatchProgressInput) error {
	// 只接受真实开始播放后的上报：预加载、缩略图抽帧不会触发 play，不写历史。
	if !in.Started && !in.Restart && in.WatchedDelta <= 0 {
		return nil
	}
	// 用服务端条目信息覆盖客户端提交，防止伪造 rel_path / folder。
	if it, ok := s.media.Get(in.VideoKey); ok {
		in.RelPath = it.RelPath
		in.MediaType = it.Type
		in.RootIndex = it.RootIndex
		in.FolderPath = it.Folder()
		if in.Duration <= 0 {
			in.Duration = it.Duration
		}
	}
	if in.MediaType == "" {
		in.MediaType = models.MediaVideo
	}
	// 观看时长做上限保护：单次上报不超过 60 秒，避免异常客户端污染统计。
	if in.WatchedDelta < 0 {
		in.WatchedDelta = 0
	}
	if in.WatchedDelta > 60 {
		in.WatchedDelta = 60
	}
	return s.store.UpsertWatchProgress(ctx, u.ID, in)
}

// RestartProgress 从头开始播放：清零进度并增加播放次数。
func (s *WatchService) RestartProgress(ctx context.Context, u *models.User, videoKey string) error {
	in := repository.WatchProgressInput{VideoKey: videoKey, MediaType: models.MediaVideo, Restart: true}
	return s.ReportProgress(ctx, u, in)
}

// UserSummary 用户观看统计。
func (s *WatchService) UserSummary(ctx context.Context, u *models.User) (repository.UserWatchSummary, error) {
	return s.store.GetUserWatchSummary(ctx, u.ID)
}

// ClearHistory 清空用户观看历史。
func (s *WatchService) ClearHistory(ctx context.Context, u *models.User) error {
	return s.store.ClearWatchHistory(ctx, u.ID)
}

// RemoveHistory 删除用户单条观看记录。
func (s *WatchService) RemoveHistory(ctx context.Context, u *models.User, key string) error {
	return s.store.RemoveWatchHistory(ctx, u.ID, key)
}

// ---------- 管理端统计与推荐 ----------

// MediaHeatView 媒体热度视图（管理端展示 / 推荐数据源）。
type MediaHeatView struct {
	Key            string    `json:"key"`
	RelPath        string    `json:"rel_path"`
	Folder         string    `json:"folder"`
	RootIndex      int       `json:"root_index"`
	MediaType      string    `json:"media_type"`
	PlayCount      int       `json:"play_count"`
	CompletedCount int       `json:"completed_count"`
	WatchedSeconds float64   `json:"watched_seconds"`
	FavoriteCount  int       `json:"favorite_count"`
	LastPlayedAt   string    `json:"last_played_at"`
	HeatScore      float64   `json:"heat_score"`
	Stats          *models.MediaStats `json:"stats,omitempty"`
}

// heatScore 热度评分：完播 > 观看时长 > 收藏 > 播放。
// 各指标先取 log1p 抑制头部条目碾压，再按 12:6:3:1 加权；
// 一次真实完播（约 8.3 分）比 100 次播放（约 4.6 分）更有价值。
func heatScore(p, c, f int, watched float64) float64 {
	return math.Log1p(float64(c))*12 +
		math.Log1p(math.Max(0, watched)/60)*6 +
		math.Log1p(float64(f))*3 +
		math.Log1p(float64(p))
}

// MediaHeat 返回媒体热度排行（管理端 & 推荐接口）。
func (s *WatchService) MediaHeat(ctx context.Context, limit int) ([]MediaHeatView, error) {
	stats, err := s.store.ListMediaStats(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MediaHeatView, 0, len(stats))
	for _, st := range stats {
		v := MediaHeatView{
			Key: st.VideoKey, PlayCount: st.PlayCount, CompletedCount: st.CompletedCount,
			WatchedSeconds: st.WatchedSeconds, FavoriteCount: st.FavoriteCount,
			LastPlayedAt: st.LastPlayedAt.Format(time.RFC3339),
			HeatScore: heatScore(st.PlayCount, st.CompletedCount, st.FavoriteCount, st.WatchedSeconds),
			Stats: &st,
		}
		if it, ok := s.media.Get(st.VideoKey); ok {
			v.RelPath = it.RelPath
			v.Folder = it.Folder()
			v.RootIndex = it.RootIndex
			v.MediaType = it.Type
		}
		out = append(out, v)
	}
	return out, nil
}

// FolderHeatView 文件夹热度视图。
type FolderHeatView struct {
	RootIndex      int     `json:"root_index"`
	RootName       string  `json:"root_name"`
	Folder         string  `json:"folder"`
	PlayCount      int     `json:"play_count"`
	CompletedCount int     `json:"completed_count"`
	WatchedSeconds float64 `json:"watched_seconds"`
	FavoriteCount  int     `json:"favorite_count"`
	MediaCount     int     `json:"media_count"`
	HeatScore      float64 `json:"heat_score"`
}

// FolderHeat 返回文件夹热度排行。
func (s *WatchService) FolderHeat(ctx context.Context, limit int) ([]FolderHeatView, error) {
	stats, err := s.store.ListFolderStats(ctx)
	if err != nil {
		return nil, err
	}
	rootName := map[int]string{}
	for _, r := range s.media.Roots() {
		rootName[r.RootIndex] = r.RootName
	}
	out := make([]FolderHeatView, 0, len(stats))
	for _, st := range stats {
		out = append(out, FolderHeatView{
			RootIndex: st.RootIndex, RootName: rootName[st.RootIndex],
			Folder: st.FolderPath, PlayCount: st.PlayCount, CompletedCount: st.CompletedCount,
			WatchedSeconds: st.WatchedSeconds, FavoriteCount: st.FavoriteCount,
			MediaCount: st.MediaCount,
			HeatScore: heatScore(st.PlayCount, st.CompletedCount, st.FavoriteCount, st.WatchedSeconds),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HeatScore > out[j].HeatScore })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// AggregateSummary 全站历史观看汇总。
func (s *WatchService) AggregateSummary(ctx context.Context) (repository.UserWatchSummary, error) {
	return s.store.AggregateWatchSummary(ctx)
}

// DailySummary 今日全站观看汇总。
func (s *WatchService) DailySummary(ctx context.Context) (repository.DailyWatchSummary, error) {
	return s.store.DailyWatchSummary(ctx)
}

// RebuildFolderStats 重建文件夹聚合统计。
func (s *WatchService) RebuildFolderStats(ctx context.Context) error {
	return s.store.RebuildFolderStats(ctx)
}

// ErrNotFound 便捷别名（供 API 层判定 404）。
var ErrNotFound = errors.New("not_found")

// FavoriteCountOf 返回单个视频的收藏数（需求 7：按当前视频统计）。
func (s *WatchService) FavoriteCountOf(ctx context.Context, videoKey string) int {
	if st, err := s.store.GetMediaStats(ctx, videoKey); err == nil {
		return st.FavoriteCount
	}
	n, _ := s.store.CountFavoritesByKey(ctx, videoKey)
	return n
}

// BatchFavoriteCounts 批量返回视频收藏数（供随机流/收藏夹列表展示）。
func (s *WatchService) BatchFavoriteCounts(ctx context.Context, keys []string) map[string]int {
	out := make(map[string]int, len(keys))
	stats, err := s.store.ListMediaStats(ctx, 0)
	if err == nil {
		m := map[string]int{}
		for _, st := range stats {
			m[st.VideoKey] = st.FavoriteCount
		}
		for _, k := range keys {
			out[k] = m[k]
		}
	}
	return out
}

// Placeholder to keep models import referenced in build tags.
var _ = models.MediaVideo
