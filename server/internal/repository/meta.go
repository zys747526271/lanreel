package repository

import (
	"context"
	"strings"
	"time"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// ListAll 读取全部媒体元数据缓存。
func (s *Store) ListAll(ctx context.Context) (map[string]models.MediaMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT video_key, media_type, duration, thumb_path, scanned_at FROM media_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.MediaMeta{}
	for rows.Next() {
		var m models.MediaMeta
		var scanned string
		if err := rows.Scan(&m.VideoKey, &m.MediaType, &m.Duration, &m.ThumbPath, &scanned); err != nil {
			return nil, err
		}
		m.ScannedAt = parseTime(scanned)
		out[m.VideoKey] = m
	}
	return out, rows.Err()
}

// UpsertBatch 批量写入媒体元数据。
func (s *Store) UpsertBatch(ctx context.Context, metas []models.MediaMeta) error {
	if len(metas) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO media_meta (video_key, media_type, duration, thumb_path, scanned_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(video_key) DO UPDATE SET media_type = excluded.media_type,
		   duration = excluded.duration, thumb_path = excluded.thumb_path, scanned_at = excluded.scanned_at`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, m := range metas {
		if _, err := stmt.ExecContext(ctx, m.VideoKey, m.MediaType, m.Duration, m.ThumbPath, s.format(m.ScannedAt)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeleteByKeys 删除指定媒体元数据。
func (s *Store) DeleteByKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM media_meta WHERE video_key = ?`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, k := range keys {
		if _, err := stmt.ExecContext(ctx, k); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---------- 审计日志 ----------

// InsertAudit 写入审计记录（业务库，独立保留，见 F-42）。
func (s *Store) InsertAudit(ctx context.Context, a models.AuditLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_operations_log
		 (admin_user_id, target_type, target_id, action, before_value, after_value, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.AdminUserID, a.TargetType, a.TargetID, a.Action, a.BeforeValue, a.AfterValue, s.now())
	return err
}

// AuditFilter 审计日志查询条件。
type AuditFilter struct {
	TargetType string
	Action     string
	AdminID    *int64
	Keyword    string
	From       *time.Time
	To         *time.Time
	Page       int
	PageSize   int
}

// ListAudit 分页查询审计日志。
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]models.AuditLog, int, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	var where []string
	var args []any
	if f.TargetType != "" {
		where = append(where, "target_type = ?")
		args = append(args, f.TargetType)
	}
	if f.Action != "" {
		where = append(where, "action LIKE ?")
		args = append(args, "%"+f.Action+"%")
	}
	if f.AdminID != nil {
		where = append(where, "admin_user_id = ?")
		args = append(args, *f.AdminID)
	}
	if f.From != nil {
		where = append(where, "created_at >= ?")
		args = append(args, s.format(*f.From))
	}
	if f.To != nil {
		where = append(where, "created_at <= ?")
		args = append(args, s.format(*f.To))
	}
	if f.Keyword != "" {
		where = append(where, "(target_id LIKE ? OR action LIKE ? OR after_value LIKE ?)")
		kw := "%" + f.Keyword + "%"
		args = append(args, kw, kw, kw)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM admin_operations_log"+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT id, admin_user_id, target_type, target_id, action, before_value, after_value, created_at
	      FROM admin_operations_log` + cond + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]models.AuditLog, 0, f.PageSize)
	for rows.Next() {
		var a models.AuditLog
		var created string
		if err := rows.Scan(&a.ID, &a.AdminUserID, &a.TargetType, &a.TargetID, &a.Action,
			&a.BeforeValue, &a.AfterValue, &created); err != nil {
			return nil, 0, err
		}
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, total, rows.Err()
}
