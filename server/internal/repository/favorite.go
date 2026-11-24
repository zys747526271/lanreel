package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// AddFavorite 添加收藏（幂等）。
func (s *Store) AddFavorite(ctx context.Context, userID int64, videoKey, videoPath string) (*models.Favorite, error) {
	now := s.now()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO favorites (user_id, video_key, video_path, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id, video_key) DO UPDATE SET video_path = excluded.video_path`,
		userID, videoKey, videoPath, now); err != nil {
		return nil, err
	}
	return s.GetFavorite(ctx, userID, videoKey)
}

// GetFavorite 查询单条收藏。
func (s *Store) GetFavorite(ctx context.Context, userID int64, videoKey string) (*models.Favorite, error) {
	var f models.Favorite
	var created string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, video_key, video_path, created_at FROM favorites WHERE user_id = ? AND video_key = ?`,
		userID, videoKey).Scan(&f.ID, &f.UserID, &f.VideoKey, &f.VideoPath, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	f.CreatedAt = parseTime(created)
	return &f, nil
}

// ListFavorites 查询用户全部收藏（按收藏时间倒序）。
func (s *Store) ListFavorites(ctx context.Context, userID int64) ([]models.Favorite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, video_key, video_path, created_at FROM favorites WHERE user_id = ? ORDER BY id DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Favorite, 0, 32)
	for rows.Next() {
		var f models.Favorite
		var created string
		if err := rows.Scan(&f.ID, &f.UserID, &f.VideoKey, &f.VideoPath, &created); err != nil {
			return nil, err
		}
		f.CreatedAt = parseTime(created)
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFavorite 取消收藏。
func (s *Store) DeleteFavorite(ctx context.Context, userID int64, videoKey string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND video_key = ?`, userID, videoKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteFavoriteByID 按主键删除收藏（管理员清理失效条目，F-28）。
func (s *Store) DeleteFavoriteByID(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteFavoritesByKeys 批量删除某用户的指定收藏（清理失效条目）。
func (s *Store) DeleteFavoritesByKeys(ctx context.Context, userID int64, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	total := 0
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND video_key = ?`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	for _, k := range keys {
		res, err := stmt.ExecContext(ctx, userID, k)
		if err != nil {
			tx.Rollback()
			return total, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			total += int(n)
		}
	}
	return total, tx.Commit()
}

// CountFavorites 统计用户收藏数。
func (s *Store) CountFavorites(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM favorites WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// CountAllFavorites 统计全站收藏数。
func (s *Store) CountAllFavorites(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM favorites`).Scan(&n)
	return n, err
}
