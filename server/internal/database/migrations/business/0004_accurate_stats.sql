-- 业务库迁移 v4：真实观看判定、今日统计与按优先级排序的热度数据（v0.3.1 修订）
--
-- 1) watch_events：每次进度上报的增量流水，今日全站统计按事件时间聚合；
-- 2) 完播只按累计观看时长判定，清掉旧数据中靠拖动进度条产生的虚假完播；
-- 3) 清掉预加载/缓存误写的零进度历史；
-- 4) 重建媒体与文件夹聚合统计，保证热度公式使用修正后的数据。

CREATE TABLE IF NOT EXISTS watch_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        INTEGER NOT NULL,
    video_key      TEXT NOT NULL,
    root_index     INTEGER NOT NULL DEFAULT 0,
    folder_path    TEXT NOT NULL DEFAULT '',
    watched_delta  REAL NOT NULL DEFAULT 0,
    play_inc       INTEGER NOT NULL DEFAULT 0,
    completed_inc  INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_watch_events_created ON watch_events(created_at);
CREATE INDEX IF NOT EXISTS idx_watch_events_user_key ON watch_events(user_id, video_key);

-- 拖动进度条到末尾不会产生 watched_seconds，这类记录不再算完播。
UPDATE watch_history
SET completed = CASE WHEN duration > 0 AND watched_seconds >= duration * 0.98 THEN 1 ELSE 0 END;

-- 预加载/缩略图抽帧误写的历史：没有真实播放时长也没有进度，直接清理。
DELETE FROM watch_history
WHERE watched_seconds <= 0 AND position <= 0 AND completed = 0;

UPDATE media_stats
SET play_count = COALESCE((SELECT SUM(play_count) FROM watch_history w WHERE w.video_key = media_stats.video_key), 0),
    completed_count = COALESCE((SELECT COUNT(1) FROM watch_history w WHERE w.video_key = media_stats.video_key AND w.completed = 1), 0),
    watched_seconds = COALESCE((SELECT SUM(watched_seconds) FROM watch_history w WHERE w.video_key = media_stats.video_key), 0);

UPDATE folder_stats
SET play_count = COALESCE((SELECT SUM(play_count) FROM watch_history w
    WHERE w.root_index = folder_stats.root_index AND w.folder_path = folder_stats.folder_path), 0),
    completed_count = COALESCE((SELECT COUNT(1) FROM watch_history w
    WHERE w.root_index = folder_stats.root_index AND w.folder_path = folder_stats.folder_path AND w.completed = 1), 0),
    watched_seconds = COALESCE((SELECT SUM(watched_seconds) FROM watch_history w
    WHERE w.root_index = folder_stats.root_index AND w.folder_path = folder_stats.folder_path), 0);
