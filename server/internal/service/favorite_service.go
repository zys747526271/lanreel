package service

import (
	"context"
	"errors"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/models"
	"github.com/zys747526271/lanreel/server/internal/repository"
)

// ErrMediaNotFound 媒体不存在。
var ErrMediaNotFound = errors.New("媒体不存在")

// FavoriteService 收藏业务（F-18 ~ F-23）。
type FavoriteService struct {
	store    *repository.Store
	media    *MediaService
	log      *logging.Logger
}

// NewFavoriteService 构造收藏服务。
func NewFavoriteService(store *repository.Store, mediaSvc *MediaService, logger *logging.Logger) *FavoriteService {
	return &FavoriteService{store: store, media: mediaSvc, log: logger}
}

// List 返回用户收藏（含不可播放标记，F-16）。
func (s *FavoriteService) List(ctx context.Context, u *models.User) ([]MediaView, error) {
	favs, err := s.store.ListFavorites(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	favorited := map[string]bool{}
	for _, f := range favs {
		favorited[f.VideoKey] = true
	}
	out := make([]MediaView, 0, len(favs))
	for _, f := range favs {
		if it, ok := s.media.Get(f.VideoKey); ok {
			out = append(out, s.media.BuildView(ctx, u, it, favorited))
			continue
		}
		out = append(out, MediaView{
			Key: f.VideoKey, RelPath: f.VideoPath, MediaType: models.MediaVideo,
			Playable: false, Reason: models.ReasonFileMissing, Favorite: true,
		})
	}
	return out, nil
}

// Keys 返回收藏 key 列表（供随机流「仅看收藏」使用）。
func (s *FavoriteService) Keys(ctx context.Context, userID int64) ([]string, error) {
	favs, err := s.store.ListFavorites(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(favs))
	for _, f := range favs {
		keys = append(keys, f.VideoKey)
	}
	return keys, nil
}

// Add 添加收藏；必须先落库成功再更新本地状态（F-21）。
func (s *FavoriteService) Add(ctx context.Context, u *models.User, videoKey string) (*MediaView, error) {
	it, ok := s.media.Get(videoKey)
	if !ok {
		return nil, ErrMediaNotFound
	}
	if _, err := s.store.AddFavorite(ctx, u.ID, it.Key, it.RelPath); err != nil {
		return nil, err
	}
	// 同步该视频的收藏计数（供收藏按钮与热度统计使用，v0.3.0）。
	_ = s.store.RefreshFavoriteCounts(ctx, it.Key)
	s.log.Info(logging.CatBusiness, "api.favorites", "添加收藏", withCtx(ctx, map[string]any{
		"video_key": it.Key, "rel_path": it.RelPath,
	}))
	v := s.media.BuildView(ctx, u, it, map[string]bool{it.Key: true})
	return &v, nil
}

// Remove 取消收藏。
func (s *FavoriteService) Remove(ctx context.Context, u *models.User, videoKey string) error {
	removed, err := s.store.DeleteFavorite(ctx, u.ID, videoKey)
	if err != nil {
		return err
	}
	// 同步该视频的收藏计数（v0.3.0）。
	_ = s.store.RefreshFavoriteCounts(ctx, videoKey)
	s.log.Info(logging.CatBusiness, "api.favorites", "取消收藏", withCtx(ctx, map[string]any{
		"video_key": videoKey, "removed": removed,
	}))
	return nil
}

// RemoveBatch 批量取消收藏（含不可播放条目），返回实际删除数量。
func (s *FavoriteService) RemoveBatch(ctx context.Context, u *models.User, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	n, err := s.store.DeleteFavoritesByKeys(ctx, u.ID, keys)
	if err != nil {
		return 0, err
	}
	for _, key := range keys {
		_ = s.store.RefreshFavoriteCounts(ctx, key)
	}
	s.log.Info(logging.CatBusiness, "api.favorites", "批量取消收藏", withCtx(ctx, map[string]any{
		"user_id": u.ID, "count": n,
	}))
	return n, nil
}

// CleanInvalid 清理用户收藏中失效（文件已删除）的条目，返回清理数量（F-28）。
func (s *FavoriteService) CleanInvalid(ctx context.Context, userID int64) (int, error) {
	favs, err := s.store.ListFavorites(ctx, userID)
	if err != nil {
		return 0, err
	}
	var invalid []string
	for _, f := range favs {
		if _, ok := s.media.Get(f.VideoKey); !ok {
			invalid = append(invalid, f.VideoKey)
		}
	}
	return s.store.DeleteFavoritesByKeys(ctx, userID, invalid)
}

// CleanMissing 清理当前用户收藏中「文件已丢失」的条目；被管理员屏蔽的文件仍保留。
func (s *FavoriteService) CleanMissing(ctx context.Context, u *models.User) (int, error) {
	n, err := s.CleanInvalid(ctx, u.ID)
	if err != nil {
		return 0, err
	}
	s.log.Info(logging.CatBusiness, "api.favorites", "一键清理丢失文件收藏", withCtx(ctx, map[string]any{
		"user_id": u.ID, "count": n,
	}))
	return n, nil
}
