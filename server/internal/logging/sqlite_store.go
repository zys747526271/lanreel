package logging

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// SQLiteStore 将运行日志写入独立日志库 logs.db（9.3 / 10.2）。
type SQLiteStore struct {
	db *sql.DB
	loc *time.Location
}

// NewSQLiteStore 构造日志存储。
func NewSQLiteStore(db *sql.DB, loc *time.Location) *SQLiteStore {
	if loc == nil {
		loc = time.Local
	}
	return &SQLiteStore{db: db, loc: loc}
}

// InsertBatch 批量写入（单事务，减少 I/O 放大）。
func (s *SQLiteStore) InsertBatch(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO system_logs
		(ts, level, category, module, message, context_json, request_id, user_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	now := time.Now().In(s.loc).Format(time.RFC3339)
	for _, e := range entries {
		var uid any
		if e.UserID != nil {
			uid = *e.UserID
		}
		if _, err := stmt.ExecContext(ctx,
			e.TS.In(s.loc).Format(time.RFC3339),
			e.Level, e.Category, e.Module, e.Message, e.ContextJSON, e.RequestID, uid, now,
		); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Query 分页检索运行日志。
func (s *SQLiteStore) Query(ctx context.Context, f QueryFilter) ([]models.SystemLogEntry, int, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 50
	}

	var where []string
	var args []any
	if f.Category != "" {
		where = append(where, "category = ?")
		args = append(args, f.Category)
	}
	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, f.Level)
	}
	if f.From != nil {
		where = append(where, "ts >= ?")
		args = append(args, f.From.In(s.loc).Format(time.RFC3339))
	}
	if f.To != nil {
		where = append(where, "ts <= ?")
		args = append(args, f.To.In(s.loc).Format(time.RFC3339))
	}
	if f.RequestID != "" {
		where = append(where, "request_id = ?")
		args = append(args, f.RequestID)
	}
	if f.UserID != nil {
		where = append(where, "user_id = ?")
		args = append(args, *f.UserID)
	}
	if f.Keyword != "" {
		where = append(where, "(message LIKE ? OR context_json LIKE ? OR module LIKE ?)")
		kw := "%" + f.Keyword + "%"
		args = append(args, kw, kw, kw)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM system_logs"+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.PageSize
	q := "SELECT id, ts, level, category, module, message, context_json, request_id, user_id, created_at FROM system_logs" +
		cond + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, f.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.SystemLogEntry, 0, f.PageSize)
	for rows.Next() {
		var (
			e    models.SystemLogEntry
			uid  sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &e.TS, &e.Level, &e.Category, &e.Module, &e.Message,
			&e.ContextJSON, &e.RequestID, &uid, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		if uid.Valid {
			id := uid.Int64
			e.UserID = &id
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// Purge 按保留天数与最大条数清理，返回删除条数。
func (s *SQLiteStore) Purge(ctx context.Context, before time.Time, maxEntries int) (int, error) {
	deleted := 0
	if !before.IsZero() {
		res, err := s.db.ExecContext(ctx, "DELETE FROM system_logs WHERE ts < ?",
			before.In(s.loc).Format(time.RFC3339))
		if err != nil {
			return deleted, err
		}
		if n, err := res.RowsAffected(); err == nil {
			deleted += int(n)
		}
	}
	if maxEntries > 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM system_logs").Scan(&count); err != nil {
			return deleted, err
		}
		if count > maxEntries {
			excess := count - maxEntries
			res, err := s.db.ExecContext(ctx, `DELETE FROM system_logs WHERE id IN (
				SELECT id FROM system_logs ORDER BY id ASC LIMIT ?)`, excess)
			if err != nil {
				return deleted, err
			}
			if n, err := res.RowsAffected(); err == nil {
				deleted += int(n)
			}
		}
	}
	return deleted, nil
}

// GetByID 按主键查询单条日志。
func (s *SQLiteStore) GetByID(ctx context.Context, id int64) (*models.SystemLogEntry, error) {
	var (
		e   models.SystemLogEntry
		uid sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, ts, level, category, module, message, context_json, request_id, user_id, created_at
		 FROM system_logs WHERE id = ?`, id).
		Scan(&e.ID, &e.TS, &e.Level, &e.Category, &e.Module, &e.Message, &e.ContextJSON, &e.RequestID, &uid, &e.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("日志不存在")
		}
		return nil, err
	}
	if uid.Valid {
		v := uid.Int64
		e.UserID = &v
	}
	return &e, nil
}

// CountByCategory 统计各分类条数，用于后台概览。
func (s *SQLiteStore) CountByCategory(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT category, COUNT(1) FROM system_logs GROUP BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

// String 便于日志输出。
func (s *SQLiteStore) String() string { return fmt.Sprintf("sqlite-log-store") }
