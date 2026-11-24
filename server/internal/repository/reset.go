package repository

import "context"

// ResetBusinessData 清空全部业务数据，使其回到首次部署状态。
// 保留 schema_migrations 与媒体文件本身，只重置账号、策略、收藏、历史、统计与设置。
func (s *Store) ResetBusinessData(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	tables := []string{
		"users",
		"favorites",
		"folder_policies",
		"file_overrides",
		"watch_history",
		"watch_events",
		"media_stats",
		"folder_stats",
		"media_meta",
		"media_root_options",
		"admin_operations_log",
		"system_settings",
	}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			tx.Rollback()
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sqlite_sequence`); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
