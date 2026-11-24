package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("记录不存在")

const userColumns = `id, username, password_hash, role, enabled, created_at`

// CreateUser 创建用户。
func (s *Store) CreateUser(ctx context.Context, username, hash, role string, enabled bool) (*models.User, error) {
	now := s.now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, enabled, created_at) VALUES (?, ?, ?, ?, ?)`,
		username, hash, role, boolToInt(enabled), now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("用户名已存在")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.User{
		ID: id, Username: username, PasswordHash: hash, Role: role,
		Enabled: enabled, CreatedAt: parseTime(now),
	}, nil
}

// GetUserByID 按 ID 查询用户。
func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// GetUserByUsername 按用户名查询用户。
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username))
}

func (s *Store) scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	var enabled int
	var created string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &enabled, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Enabled = enabled == 1
	u.CreatedAt = parseTime(created)
	return &u, nil
}

// UserListFilter 用户列表查询条件。
type UserListFilter struct {
	Keyword  string
	Page     int
	PageSize int
}

// ListUsers 分页查询用户。
func (s *Store) ListUsers(ctx context.Context, f UserListFilter) ([]models.User, int, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	cond := ""
	var args []any
	if f.Keyword != "" {
		cond = " WHERE username LIKE ?"
		args = append(args, "%"+f.Keyword+"%")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users"+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := "SELECT " + userColumns + " FROM users" + cond + " ORDER BY id ASC LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]models.User, 0, f.PageSize)
	for rows.Next() {
		var u models.User
		var enabled int
		var created string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &enabled, &created); err != nil {
			return nil, 0, err
		}
		u.Enabled = enabled == 1
		u.CreatedAt = parseTime(created)
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UpdateUser 更新用户可变字段（nil 表示不修改）。
func (s *Store) UpdateUser(ctx context.Context, id int64, passwordHash, role *string, enabled *bool) error {
	sets := []string{}
	args := []any{}
	if passwordHash != nil {
		sets = append(sets, "password_hash = ?")
		args = append(args, *passwordHash)
	}
	if role != nil {
		sets = append(sets, "role = ?")
		args = append(args, *role)
	}
	if enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolToInt(*enabled))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, "UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// DeleteUser 删除用户并级联删除其收藏（F-26）。
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// CountUsers 统计用户数。
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users").Scan(&n)
	return n, err
}

// CountAdmins 统计管理员数，用于防止删除最后一个管理员。
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE role = ? AND enabled = 1", models.RoleAdmin).Scan(&n)
	return n, err
}

// FirstAdmin 返回最早创建的启用管理员。
// 用于 --reset-admin-password 兜底：网页初始化时管理员用户名可能不同于 auth.username。
func (s *Store) FirstAdmin(ctx context.Context) (*models.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE role = ? AND enabled = 1 ORDER BY id ASC LIMIT 1`,
		models.RoleAdmin))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
