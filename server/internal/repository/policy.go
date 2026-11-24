package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// ---------- 文件夹策略 ----------

// ListFolderPolicies 返回全部文件夹策略。
func (s *Store) ListFolderPolicies(ctx context.Context) ([]models.FolderPolicy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, root_index, folder_path, enabled, guest_enabled, schedule_type, time_start, time_end,
		        guest_schedule_type, guest_time_start, guest_time_end, updated_at
		 FROM folder_policies ORDER BY root_index, folder_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.FolderPolicy, 0, 16)
	for rows.Next() {
		var p models.FolderPolicy
		var enabled, guestEnabled int
		var updated string
		if err := rows.Scan(&p.ID, &p.RootIndex, &p.FolderPath, &enabled, &guestEnabled,
			&p.ScheduleType, &p.TimeStart, &p.TimeEnd,
			&p.GuestScheduleType, &p.GuestTimeStart, &p.GuestTimeEnd, &updated); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		p.GuestEnabled = guestEnabled == 1
		p.UpdatedAt = parseTime(updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertFolderPolicy 新建或更新文件夹策略。
func (s *Store) UpsertFolderPolicy(ctx context.Context, p models.FolderPolicy) (*models.FolderPolicy, error) {
	now := s.now()
	guestSchedule := p.GuestScheduleType
	if guestSchedule == "" {
		guestSchedule = p.ScheduleType
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO folder_policies (root_index, folder_path, enabled, guest_enabled, schedule_type, time_start, time_end,
		        guest_schedule_type, guest_time_start, guest_time_end, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(root_index, folder_path) DO UPDATE SET
		   enabled = excluded.enabled, guest_enabled = excluded.guest_enabled,
		   schedule_type = excluded.schedule_type,
		   time_start = excluded.time_start, time_end = excluded.time_end,
		   guest_schedule_type = excluded.guest_schedule_type,
		   guest_time_start = excluded.guest_time_start, guest_time_end = excluded.guest_time_end,
		   updated_at = excluded.updated_at`,
		p.RootIndex, p.FolderPath, boolToInt(p.Enabled), boolToInt(p.GuestEnabled),
		p.ScheduleType, p.TimeStart, p.TimeEnd,
		guestSchedule, p.GuestTimeStart, p.GuestTimeEnd, now)
	if err != nil {
		return nil, err
	}
	var out models.FolderPolicy
	var enabled, guestEnabled int
	var updated string
	err = s.db.QueryRowContext(ctx,
		`SELECT id, root_index, folder_path, enabled, guest_enabled, schedule_type, time_start, time_end,
		        guest_schedule_type, guest_time_start, guest_time_end, updated_at
		 FROM folder_policies WHERE root_index = ? AND folder_path = ?`, p.RootIndex, p.FolderPath).
		Scan(&out.ID, &out.RootIndex, &out.FolderPath, &enabled, &guestEnabled,
			&out.ScheduleType, &out.TimeStart, &out.TimeEnd,
			&out.GuestScheduleType, &out.GuestTimeStart, &out.GuestTimeEnd, &updated)
	if err != nil {
		return nil, err
	}
	out.Enabled = enabled == 1
	out.GuestEnabled = guestEnabled == 1
	out.UpdatedAt = parseTime(updated)
	return &out, nil
}

// DeleteFolderPolicy 删除文件夹策略（恢复上级/默认规则）。
func (s *Store) DeleteFolderPolicy(ctx context.Context, rootIndex int, folderPath string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM folder_policies WHERE root_index = ? AND folder_path = ?`, rootIndex, folderPath)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---------- 文件级覆盖 ----------

// GetFileOverride 查询单文件覆盖。
func (s *Store) GetFileOverride(ctx context.Context, videoKey string) (*models.FileOverride, error) {
	var o models.FileOverride
	var enabled int
	var updated string
	var guestEnabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, video_key, enabled, guest_enabled, schedule_type, time_start, time_end,
		        guest_schedule_type, guest_time_start, guest_time_end, note, updated_at
		 FROM file_overrides WHERE video_key = ?`, videoKey).
		Scan(&o.ID, &o.VideoKey, &enabled, &guestEnabled, &o.ScheduleType, &o.TimeStart, &o.TimeEnd,
			&o.GuestScheduleType, &o.GuestTimeStart, &o.GuestTimeEnd, &o.Note, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o.Enabled = enabled == 1
	o.GuestEnabled = guestEnabled == 1
	o.UpdatedAt = parseTime(updated)
	return &o, nil
}

// ListFileOverrides 返回全部文件级覆盖（key → 覆盖）。
func (s *Store) ListFileOverrides(ctx context.Context) (map[string]models.FileOverride, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, video_key, enabled, guest_enabled, schedule_type, time_start, time_end,
		        guest_schedule_type, guest_time_start, guest_time_end, note, updated_at FROM file_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.FileOverride{}
	for rows.Next() {
		var o models.FileOverride
		var enabled, guestEnabled int
		var updated string
		if err := rows.Scan(&o.ID, &o.VideoKey, &enabled, &guestEnabled,
			&o.ScheduleType, &o.TimeStart, &o.TimeEnd,
			&o.GuestScheduleType, &o.GuestTimeStart, &o.GuestTimeEnd, &o.Note, &updated); err != nil {
			return nil, err
		}
		o.Enabled = enabled == 1
		o.GuestEnabled = guestEnabled == 1
		o.UpdatedAt = parseTime(updated)
		out[o.VideoKey] = o
	}
	return out, rows.Err()
}

// UpsertFileOverride 新建或更新文件级覆盖。
func (s *Store) UpsertFileOverride(ctx context.Context, o models.FileOverride) (*models.FileOverride, error) {
	now := s.now()
	guestSchedule := o.GuestScheduleType
	if guestSchedule == "" {
		guestSchedule = o.ScheduleType
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO file_overrides (video_key, enabled, guest_enabled, schedule_type, time_start, time_end,
		        guest_schedule_type, guest_time_start, guest_time_end, note, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(video_key) DO UPDATE SET
		   enabled = excluded.enabled, guest_enabled = excluded.guest_enabled,
		   schedule_type = excluded.schedule_type,
		   time_start = excluded.time_start, time_end = excluded.time_end,
		   guest_schedule_type = excluded.guest_schedule_type,
		   guest_time_start = excluded.guest_time_start, guest_time_end = excluded.guest_time_end,
		   note = excluded.note, updated_at = excluded.updated_at`,
		o.VideoKey, boolToInt(o.Enabled), boolToInt(o.GuestEnabled),
		o.ScheduleType, o.TimeStart, o.TimeEnd,
		guestSchedule, o.GuestTimeStart, o.GuestTimeEnd, o.Note, now)
	if err != nil {
		return nil, err
	}
	return s.GetFileOverride(ctx, o.VideoKey)
}

// DeleteFileOverride 删除文件级覆盖（回落文件夹策略）。
func (s *Store) DeleteFileOverride(ctx context.Context, videoKey string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM file_overrides WHERE video_key = ?`, videoKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountFileOverrides 统计文件级覆盖数量。
func (s *Store) CountFileOverrides(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM file_overrides`).Scan(&n)
	return n, err
}

// ---------- 设置 ----------

// GetSetting 读取设置项。
func (s *Store) GetSetting(ctx context.Context, key, def string) string {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting 写入设置项。
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, s.now())
	return err
}

// AllSettings 返回全部设置。
func (s *Store) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// NormalizeFolder 归一化文件夹相对路径。
func NormalizeFolder(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.Trim(p, "/")
	if p == "." {
		return ""
	}
	return p
}
