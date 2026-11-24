-- 业务库初始化脚本（迁移版本 v1）
-- 说明：日志表不在此库中，运行日志位于独立的 logs.db（见 N-24）。

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user',
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS favorites (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_key  TEXT NOT NULL,
    video_path TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    UNIQUE (user_id, video_key)
);
CREATE INDEX IF NOT EXISTS idx_favorites_user ON favorites(user_id);

CREATE TABLE IF NOT EXISTS folder_policies (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    folder_path   TEXT NOT NULL UNIQUE,
    enabled       INTEGER NOT NULL DEFAULT 1,
    schedule_type TEXT NOT NULL DEFAULT 'always',
    time_start    TEXT NOT NULL DEFAULT '',
    time_end      TEXT NOT NULL DEFAULT '',
    updated_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS file_overrides (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    video_key     TEXT NOT NULL UNIQUE,
    enabled       INTEGER NOT NULL DEFAULT 1,
    schedule_type TEXT NOT NULL DEFAULT 'always',
    time_start    TEXT NOT NULL DEFAULT '',
    time_end      TEXT NOT NULL DEFAULT '',
    note          TEXT NOT NULL DEFAULT '',
    updated_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS media_meta (
    video_key  TEXT PRIMARY KEY,
    media_type TEXT NOT NULL,
    duration   REAL NOT NULL DEFAULT 0,
    thumb_path TEXT NOT NULL DEFAULT '',
    scanned_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_operations_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_user_id INTEGER NOT NULL,
    target_type   TEXT NOT NULL,
    target_id     TEXT NOT NULL,
    action        TEXT NOT NULL,
    before_value  TEXT NOT NULL DEFAULT '',
    after_value   TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_created ON admin_operations_log(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_admin ON admin_operations_log(admin_user_id);

CREATE TABLE IF NOT EXISTS system_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);
