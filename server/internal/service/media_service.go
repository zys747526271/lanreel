package service

import (
	"context"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/zys747526271/lanreel/server/internal/config"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/policy"
)

// MediaView 返回给客户端的媒体视图（协议稳定，新增字段不破坏旧客户端，见 9.2）。
type MediaView struct {
	Key       string  `json:"key"`
	MediaType string  `json:"media_type"`
	RelPath   string  `json:"rel_path"`
	Folder    string  `json:"folder"`
	Duration  float64 `json:"duration"`
	Size      int64   `json:"size"`
	Playable  bool    `json:"playable"`
	Reason    string  `json:"reason,omitempty"`
	Favorite  bool    `json:"favorite"`
	StreamURL string  `json:"stream_url"`
	ThumbURL  string  `json:"thumb_url"`
}

// MediaService 媒体浏览业务（随机流 + 视图组装）。
type MediaService struct {
	scanner   *media.Scanner
	evaluator *policy.Evaluator
	cfg       *config.Config
	log       *logging.Logger
}

// NewMediaService 构造媒体服务。
func NewMediaService(scanner *media.Scanner, evaluator *policy.Evaluator, cfg *config.Config, logger *logging.Logger) *MediaService {
	return &MediaService{scanner: scanner, evaluator: evaluator, cfg: cfg, log: logger}
}

// Get 按 key 查询媒体条目。
func (s *MediaService) Get(key string) (*media.Item, bool) {
	return s.scanner.Snapshot().Get(key)
}

// Snapshot 返回扫描快照。
func (s *MediaService) Snapshot() *media.Snapshot { return s.scanner.Snapshot() }

// ScanSummary 返回最近一次扫描统计。
func (s *MediaService) ScanSummary() media.ScanSummary { return s.scanner.Summary() }

// Scanning 是否正在扫描。
func (s *MediaService) Scanning() bool { return s.scanner.Scanning() }

// Scanner 暴露扫描器（后台媒体目录设置需要热更新根目录）。
func (s *MediaService) Scanner() *media.Scanner { return s.scanner }

// BuildView 组装带可见性与收藏状态的视图。
func (s *MediaService) BuildView(ctx context.Context, u *models.User, item *media.Item, favorited map[string]bool) MediaView {
	d := s.evaluator.Evaluate(ctx, item, u.IsAdmin(), time.Now())
	v := MediaView{
		Key:       item.Key,
		MediaType: item.Type,
		RelPath:   item.RelPath,
		Folder:    item.Folder(),
		Duration:  item.Duration,
		Size:      item.Size,
		Playable:  d.Playable,
		Reason:    d.Reason,
		Favorite:  favorited != nil && favorited[item.Key],
		StreamURL: "/api/v1/media/" + item.Key + "/stream",
		ThumbURL:  "/api/v1/media/" + item.Key + "/thumb",
	}
	return v
}

// Decide 判定单个媒体的可见性。
func (s *MediaService) Decide(ctx context.Context, u *models.User, item *media.Item) policy.Decision {
	return s.evaluator.Evaluate(ctx, item, u.IsAdmin(), time.Now())
}

// RandomQuery 随机流查询参数。
type RandomQuery struct {
	Limit     int
	Exclude   []string
	Favorites bool
	// FavoriteKeys 当 Favorites=true 时使用（由调用方从收藏仓储读取）。
	FavoriteKeys []string
}

// Random 返回随机媒体视图；普通用户只返回当前可播放的媒体（F-02 / F-15）。
func (s *MediaService) Random(ctx context.Context, u *models.User, q RandomQuery) ([]MediaView, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}
	exclude := map[string]bool{}
	for _, k := range q.Exclude {
		if k != "" {
			exclude[k] = true
		}
	}

	if q.Favorites {
		return s.randomFromFavorites(ctx, u, q.FavoriteKeys, exclude, limit), nil
	}

	snap := s.scanner.Snapshot()
	candidates := make([]*media.Item, 0, len(snap.Items))
	for _, it := range snap.Items {
		if exclude[it.Key] {
			continue
		}
		if !u.IsAdmin() {
			if d := s.evaluator.Evaluate(ctx, it, false, time.Now()); !d.Playable {
				continue
			}
		}
		candidates = append(candidates, it)
	}
	if len(candidates) == 0 && len(exclude) > 0 {
		// 排除项过多（例如媒体很少）时忽略排除，避免返回空列表。
		for _, it := range snap.Items {
			if u.IsAdmin() || s.evaluator.Evaluate(ctx, it, false, time.Now()).Playable {
				candidates = append(candidates, it)
			}
		}
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]MediaView, 0, len(candidates))
	for _, it := range candidates {
		out = append(out, s.BuildView(ctx, u, it, nil))
	}
	return out, nil
}

// randomFromFavorites 在收藏集合内随机（F-23：不可播放条目仍然展示，由前端模糊处理）。
func (s *MediaService) randomFromFavorites(ctx context.Context, u *models.User, keys []string, exclude map[string]bool, limit int) []MediaView {
	snap := s.scanner.Snapshot()
	picked := make([]*media.Item, 0, len(keys))
	dangling := make([]string, 0)
	for _, k := range keys {
		if exclude[k] {
			continue
		}
		if it, ok := snap.Get(k); ok {
			picked = append(picked, it)
		} else {
			dangling = append(dangling, k)
		}
	}
	rand.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })
	out := make([]MediaView, 0, limit)
	for _, it := range picked {
		if len(out) >= limit {
			break
		}
		v := s.BuildView(ctx, u, it, nil)
		v.Favorite = true
		out = append(out, v)
	}
	// 文件已删除的收藏条目仍然回传，标记 file_missing（F-16）。
	for _, k := range dangling {
		if len(out) >= limit {
			break
		}
		out = append(out, MediaView{
			Key: k, MediaType: models.MediaVideo, Playable: false,
			Reason: models.ReasonFileMissing, Favorite: true,
		})
	}
	return out
}

// FolderStat 文件夹统计视图。
type FolderStat struct {
	FolderPath string `json:"folder_path"`
	Total      int    `json:"total"`
	Videos     int    `json:"videos"`
	Images     int    `json:"images"`
}

// Folders 统计当前快照中的文件夹分布，供后台配置策略。
func (s *MediaService) Folders() []FolderStat {
	snap := s.scanner.Snapshot()
	m := map[string]*FolderStat{}
	for _, it := range snap.Items {
		f := it.Folder()
		st, ok := m[f]
		if !ok {
			st = &FolderStat{FolderPath: f}
			m[f] = st
		}
		st.Total++
		if it.Type == models.MediaVideo {
			st.Videos++
		} else {
			st.Images++
		}
	}
	out := make([]FolderStat, 0, len(m))
	for _, st := range m {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FolderPath < out[j].FolderPath })
	return out
}

// SearchItems 按相对路径关键字搜索媒体（后台文件策略页使用）。
func (s *MediaService) SearchItems(keyword string, limit int) []*media.Item {
	kw := strings.ToLower(strings.TrimSpace(keyword))
	snap := s.scanner.Snapshot()
	out := make([]*media.Item, 0, 32)
	for _, it := range snap.Items {
		if kw != "" && !strings.Contains(strings.ToLower(it.RelPath), kw) {
			continue
		}
		out = append(out, it)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
