-- 日志库初始化脚本（独立 logs.db，版本号与业务库相互独立，见 N-21 / N-24）
CREATE TABLE IF NOT EXISTS system_logs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    ts           TEXT NOT NULL,           -- RFC3339，配置时区
    level        TEXT NOT NULL,
    category     TEXT NOT NULL,
    module       TEXT NOT NULL,
    message      TEXT NOT NULL,
    context_json TEXT NOT NULL DEFAULT '',
    request_id   TEXT NOT NULL DEFAULT '',
    user_id      INTEGER,
    created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_ts ON system_logs(ts);
CREATE INDEX IF NOT EXISTS idx_logs_category ON system_logs(category);
CREATE INDEX IF NOT EXISTS idx_logs_level ON system_logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_request ON system_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_logs_ts_cat_level ON system_logs(ts, category, level);
