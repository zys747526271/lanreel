-- 业务库迁移 v3：观看进度与统计（v0.3.0）
--
-- 1) watch_history：观看历史改为服务端存储，按用户隔离（原来是浏览器 IndexedDB 全局共用）。
--    每个用户每条媒体一行，记录进度、观看时长、播放次数、是否看完。
-- 2) media_stats：每个媒体的聚合统计（热度/播放次数/播放时长/收藏次数/最近播放），
--    供管理端控制台与推荐算法使用。
-- 3) folder_stats：每个文件夹的聚合统计（按根索引 + 相对路径），供管理端与推荐使用。
-- 4) folder_policies / file_overrides：拆分用户与游客的播放时段，互不共用。
-- 5) media_roots 表：支持每个根目录是否递归导入子目录（需求 10）。

CREATE TABLE IF NOT EXISTS watch_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_key       TEXT NOT NULL,
    rel_path        TEXT NOT NULL DEFAULT '',
    media_type      TEXT NOT NULL DEFAULT 'video',
    root_index      INTEGER NOT NULL DEFAULT 0,
    folder_path     TEXT NOT NULL DEFAULT '',
    duration        REAL NOT NULL DEFAULT 0,
    position        REAL NOT NULL DEFAULT 0,
    watched_seconds REAL NOT NULL DEFAULT 0,
    play_count      INTEGER NOT NULL DEFAULT 0,
    completed       INTEGER NOT NULL DEFAULT 0,
    last_played_at  DATETIME NOT NULL,
    created_at      DATETIME NOT NULL,
    UNIQUE (user_id, video_key)
);
CREATE INDEX IF NOT EXISTS idx_watch_history_user ON watch_history(user_id, last_played_at DESC);
CREATE INDEX IF NOT EXISTS idx_watch_history_key ON watch_history(video_key);

CREATE TABLE IF NOT EXISTS media_stats (
    video_key       TEXT PRIMARY KEY,
    play_count      INTEGER NOT NULL DEFAULT 0,
    completed_count INTEGER NOT NULL DEFAULT 0,
    watched_seconds REAL NOT NULL DEFAULT 0,
    favorite_count  INTEGER NOT NULL DEFAULT 0,
    last_played_at  DATETIME NOT NULL DEFAULT '',
    updated_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_media_stats_heat ON media_stats(play_count DESC, watched_seconds DESC);

CREATE TABLE IF NOT EXISTS folder_stats (
    root_index      INTEGER NOT NULL DEFAULT 0,
    folder_path     TEXT NOT NULL DEFAULT '',
    play_count      INTEGER NOT NULL DEFAULT 0,
    completed_count INTEGER NOT NULL DEFAULT 0,
    watched_seconds REAL NOT NULL DEFAULT 0,
    favorite_count  INTEGER NOT NULL DEFAULT 0,
    media_count     INTEGER NOT NULL DEFAULT 0,
    updated_at      DATETIME NOT NULL,
    PRIMARY KEY (root_index, folder_path)
);

-- 用户与游客各自的播放时段：新增 *_schedule_* 列（旧的 schedule_* 保留作为普通用户时段）。
ALTER TABLE folder_policies ADD COLUMN guest_schedule_type TEXT NOT NULL DEFAULT 'always';
ALTER TABLE folder_policies ADD COLUMN guest_time_start TEXT NOT NULL DEFAULT '';
ALTER TABLE folder_policies ADD COLUMN guest_time_end TEXT NOT NULL DEFAULT '';

ALTER TABLE file_overrides ADD COLUMN guest_schedule_type TEXT NOT NULL DEFAULT 'always';
ALTER TABLE file_overrides ADD COLUMN guest_time_start TEXT NOT NULL DEFAULT '';
ALTER TABLE file_overrides ADD COLUMN guest_time_end TEXT NOT NULL DEFAULT '';

-- 每个媒体根是否递归导入子目录（默认递归，保持旧行为）。
CREATE TABLE IF NOT EXISTS media_root_options (
    root_index INTEGER PRIMARY KEY,
    recursive  INTEGER NOT NULL DEFAULT 1,
    updated_at DATETIME NOT NULL
);
