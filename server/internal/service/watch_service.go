package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
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
	Key           string  `json:"key"`
	MediaType     string  `json:"media_type"`
	RelPath       string  `json:"rel_path"`
	RootIndex     int     `json:"root_index"`
	RootName      string  `json:"root_name"`
	Folder        string  `json:"folder"`
	Duration      float64 `json:"duration"`
	Position      float64 `json:"position"`
	Progress      float64 `json:"progress"`
	PlayCount     int     `json:"play_count"`
	Completed     bool    `json:"completed"`
	Playable      bool    `json:"playable"`
	Reason        string  `json:"reason,omitempty"`
	StreamURL     string  `json:"stream_url"`
	ThumbURL      string  `json:"thumb_url"`
	LastPlayedAt  string  `json:"last_played_at"`
	FavoriteCount int     `json:"favorite_count"`
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
			Playable:     true,
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
	Key     string `json:"key"`
	RelPath string `json:"rel_path"`
	// DisplayName 是表格里始终可见的媒体名：当前路径 > 历史/收藏快照 > key。
	DisplayName string `json:"display_name"`
	// Missing 表示媒体已从扫描库移除，只保留历史统计，不应跳转到文件管理页。
	Missing        bool               `json:"missing,omitempty"`
	Folder         string             `json:"folder"`
	RootIndex      int                `json:"root_index"`
	MediaType      string             `json:"media_type"`
	PlayCount      int                `json:"play_count"`
	CompletedCount int                `json:"completed_count"`
	WatchedSeconds float64            `json:"watched_seconds"`
	FavoriteCount  int                `json:"favorite_count"`
	LastPlayedAt   string             `json:"last_played_at"`
	HeatScore      float64            `json:"heat_score"`
	Stats          *models.MediaStats `json:"stats,omitempty"`
}

// heatDisplayName 从路径中取文件名，兼容 Windows 与 POSIX 分隔符；路径缺失时退回 key。
func heatDisplayName(relPath, key string) string {
	p := strings.TrimSpace(relPath)
	if p == "" {
		p = strings.TrimSpace(key)
	}
	// 先去掉尾部分隔符，避免 "root/sub/" 被当成一整段名称。
	p = strings.TrimRight(p, `/\`)
	if p == "" {
		p = strings.TrimSpace(key)
	}
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	if p == "" {
		return "未命名媒体"
	}
	return p
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
	// 扫描移除的文件在 media_stats 里仍留有统计行，用历史/收藏里的路径快照补回名字。
	snapshots, _ := s.store.ListMediaPathSnapshots(ctx)
	out := make([]MediaHeatView, 0, len(stats))
	for _, st := range stats {
		v := MediaHeatView{
			Key: st.VideoKey, PlayCount: st.PlayCount, CompletedCount: st.CompletedCount,
			WatchedSeconds: st.WatchedSeconds, FavoriteCount: st.FavoriteCount,
			LastPlayedAt: st.LastPlayedAt.Format(time.RFC3339),
			HeatScore:    heatScore(st.PlayCount, st.CompletedCount, st.FavoriteCount, st.WatchedSeconds),
			Stats:        &st,
		}
		rel := snapshots[st.VideoKey]
		if it, ok := s.media.Get(st.VideoKey); ok {
			v.RelPath = it.RelPath
			rel = it.RelPath
			v.Folder = it.Folder()
			v.RootIndex = it.RootIndex
			v.MediaType = it.Type
		} else {
			v.Missing = true
		}
		v.DisplayName = heatDisplayName(rel, st.VideoKey)
		if v.Missing {
			v.DisplayName = "已移除 · " + v.DisplayName
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
			HeatScore:  heatScore(st.PlayCount, st.CompletedCount, st.FavoriteCount, st.WatchedSeconds),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HeatScore > out[j].HeatScore })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// HeatSortKey 管理端热度表支持的全局排序字段。
type HeatSortKey string

const (
	HeatSortHeat     HeatSortKey = "heat"
	HeatSortPlay     HeatSortKey = "play"
	HeatSortComplete HeatSortKey = "complete"
	HeatSortWatched  HeatSortKey = "watched"
	HeatSortFavorite HeatSortKey = "favorite"
	HeatSortName     HeatSortKey = "name"
)

// HeatPage 热度分页结果：先对全量数据排序，再切片分页。
type HeatPage[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// normalizeHeatSort 把查询参数收敛到白名单，避免把外部字符串拼进排序逻辑。
func normalizeHeatSort(raw string, allowed ...HeatSortKey) (HeatSortKey, bool) {
	key := HeatSortKey(strings.ToLower(strings.TrimSpace(raw)))
	for _, a := range allowed {
		if key == a {
			return key, false
		}
	}
	return allowed[0], true
}

func pageSlice[T any](items []T, page, size int) []T {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 200 {
		size = 200
	}
	// 极端参数下避免整数溢出导致切片越界。
	if page > len(items)/size+2 {
		return []T{}
	}
	start := (page - 1) * size
	if start >= len(items) {
		return []T{}
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// MediaHeatPaged 先按指定字段对全量媒体排序，再分页返回。
func (s *WatchService) MediaHeatPaged(ctx context.Context, sortBy string, desc bool, page, size int, hideMissing bool) (HeatPage[MediaHeatView], error) {
	key, _ := normalizeHeatSort(sortBy, HeatSortHeat, HeatSortPlay, HeatSortComplete, HeatSortWatched, HeatSortFavorite, HeatSortName)
	all, err := s.MediaHeat(ctx, 0)
	if err != nil {
		return HeatPage[MediaHeatView]{}, err
	}
	// 在排序和分页之前过滤，保证 total 与页码对应的是实际可见条目。
	if hideMissing {
		visible := make([]MediaHeatView, 0, len(all))
		for _, v := range all {
			if !v.Missing {
				visible = append(visible, v)
			}
		}
		all = visible
	}
	sort.SliceStable(all, func(i, j int) bool {
		var less bool
		switch key {
		case HeatSortPlay:
			less = all[i].PlayCount < all[j].PlayCount
		case HeatSortComplete:
			less = all[i].CompletedCount < all[j].CompletedCount
		case HeatSortWatched:
			less = all[i].WatchedSeconds < all[j].WatchedSeconds
		case HeatSortFavorite:
			less = all[i].FavoriteCount < all[j].FavoriteCount
		case HeatSortName:
			li, lj := heatSortName(all[i]), heatSortName(all[j])
			less = li < lj
		default:
			less = all[i].HeatScore < all[j].HeatScore
		}
		if desc {
			return !less && !heatEqual(all[i], all[j], key)
		}
		return less
	})
	return HeatPage[MediaHeatView]{Items: pageSlice(all, page, size), Total: len(all)}, nil
}

// heatSortName 按真实文件名排序，忽略「已移除」状态前缀。
func heatSortName(v MediaHeatView) string {
	name := strings.TrimPrefix(v.DisplayName, "已移除 · ")
	if name == "" {
		name = heatDisplayName(v.RelPath, v.Key)
	}
	return strings.ToLower(name)
}

// heatEqual 仅在倒序时用于避免把相等项判定为「小于」导致的不稳定交换。
func heatEqual(a, b MediaHeatView, key HeatSortKey) bool {
	switch key {
	case HeatSortPlay:
		return a.PlayCount == b.PlayCount
	case HeatSortComplete:
		return a.CompletedCount == b.CompletedCount
	case HeatSortWatched:
		return a.WatchedSeconds == b.WatchedSeconds
	case HeatSortFavorite:
		return a.FavoriteCount == b.FavoriteCount
	case HeatSortName:
		return heatSortName(a) == heatSortName(b)
	default:
		return a.HeatScore == b.HeatScore
	}
}

// FolderHeatPaged 先按指定字段对全量文件夹排序，再分页返回。
func (s *WatchService) FolderHeatPaged(ctx context.Context, sortBy string, desc bool, page, size int) (HeatPage[FolderHeatView], error) {
	key, _ := normalizeHeatSort(sortBy, HeatSortHeat, HeatSortPlay, HeatSortComplete, HeatSortWatched, HeatSortFavorite, HeatSortName)
	all, err := s.FolderHeat(ctx, 0)
	if err != nil {
		return HeatPage[FolderHeatView]{}, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		var less, equal bool
		switch key {
		case HeatSortPlay:
			less, equal = all[i].PlayCount < all[j].PlayCount, all[i].PlayCount == all[j].PlayCount
		case HeatSortComplete:
			less, equal = all[i].CompletedCount < all[j].CompletedCount, all[i].CompletedCount == all[j].CompletedCount
		case HeatSortWatched:
			less, equal = all[i].WatchedSeconds < all[j].WatchedSeconds, all[i].WatchedSeconds == all[j].WatchedSeconds
		case HeatSortFavorite:
			less, equal = all[i].FavoriteCount < all[j].FavoriteCount, all[i].FavoriteCount == all[j].FavoriteCount
		case HeatSortName:
			li, lj := strings.ToLower(all[i].RootName+"/"+all[i].Folder), strings.ToLower(all[j].RootName+"/"+all[j].Folder)
			less, equal = li < lj, li == lj
		default:
			less, equal = all[i].HeatScore < all[j].HeatScore, all[i].HeatScore == all[j].HeatScore
		}
		if desc {
			return !less && !equal
		}
		return less
	})
	return HeatPage[FolderHeatView]{Items: pageSlice(all, page, size), Total: len(all)}, nil
}

// UserHeatPaged 先按指定字段对全量用户统计排序，再分页返回。
func (s *WatchService) UserHeatPaged(ctx context.Context, sortBy string, desc bool, page, size int) (HeatPage[repository.UserWatchRank], error) {
	key, _ := normalizeHeatSort(sortBy, HeatSortName, HeatSortPlay, HeatSortComplete, HeatSortWatched)
	all, err := s.store.ListUserWatchRank(ctx, 0)
	if err != nil {
		return HeatPage[repository.UserWatchRank]{}, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		var less, equal bool
		switch key {
		case HeatSortPlay:
			less, equal = all[i].WatchedVideos < all[j].WatchedVideos, all[i].WatchedVideos == all[j].WatchedVideos
		case HeatSortComplete:
			less, equal = all[i].CompletedVideos < all[j].CompletedVideos, all[i].CompletedVideos == all[j].CompletedVideos
		case HeatSortWatched:
			less, equal = all[i].TotalWatchTime < all[j].TotalWatchTime, all[i].TotalWatchTime == all[j].TotalWatchTime
		default:
			li, lj := strings.ToLower(all[i].Username), strings.ToLower(all[j].Username)
			less, equal = li < lj, li == lj
		}
		if desc {
			return !less && !equal
		}
		return less
	})
	return HeatPage[repository.UserWatchRank]{Items: pageSlice(all, page, size), Total: len(all)}, nil
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
