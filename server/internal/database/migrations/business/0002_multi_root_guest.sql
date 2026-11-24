-- 业务库迁移 v2：多根目录策略隔离（B-07）+ 游客可见性
--
-- 1) folder_policies 增加 root_index：策略按「根索引 + 源内相对路径」隔离，
--    不再出现不同根下同名目录共享策略。
--    root_index = -1 表示全局默认策略，对所有根生效（旧数据的根目录策略迁移为此值）。
-- 2) folder_policies / file_overrides 增加 guest_enabled：游客（未登录）可见性，
--    与普通用户可见性（enabled）共用同一时间窗口。

CREATE TABLE folder_policies_v2 (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    root_index    INTEGER NOT NULL DEFAULT 0,
    folder_path   TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    guest_enabled INTEGER NOT NULL DEFAULT 1,
    schedule_type TEXT NOT NULL DEFAULT 'always',
    time_start    TEXT NOT NULL DEFAULT '',
    time_end      TEXT NOT NULL DEFAULT '',
    updated_at    DATETIME NOT NULL,
    UNIQUE (root_index, folder_path)
);

INSERT INTO folder_policies_v2
    (id, root_index, folder_path, enabled, guest_enabled, schedule_type, time_start, time_end, updated_at)
SELECT id,
       CASE WHEN folder_path = '' THEN -1 ELSE 0 END,
       folder_path, enabled, 1, schedule_type, time_start, time_end, updated_at
FROM folder_policies;

DROP TABLE folder_policies;
ALTER TABLE folder_policies_v2 RENAME TO folder_policies;

ALTER TABLE file_overrides ADD COLUMN guest_enabled INTEGER NOT NULL DEFAULT 1;
