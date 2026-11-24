// Package repository 隔离数据访问（9.3 存储适配层）。
package repository

import (
	"database/sql"
	"time"
)

// Store 业务库仓储集合。所有方法均显式接收 context，便于替换为其他数据库实现。
type Store struct {
	db  *sql.DB
	loc *time.Location
}

// New 构造仓储。
func New(db *sql.DB, loc *time.Location) *Store {
	if loc == nil {
		loc = time.Local
	}
	return &Store{db: db, loc: loc}
}

// DB 返回底层连接（仅用于健康检查等场景）。
func (s *Store) DB() *sql.DB { return s.db }

// Loc 返回时区。
func (s *Store) Loc() *time.Location { return s.loc }

func (s *Store) now() string { return time.Now().In(s.loc).Format(time.RFC3339) }

func (s *Store) format(t time.Time) string { return t.In(s.loc).Format(time.RFC3339) }

func parseTime(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}
