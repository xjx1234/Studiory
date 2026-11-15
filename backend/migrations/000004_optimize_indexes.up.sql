-- 索引清理与列表查询优化（PostgreSQL）。
--
-- 1) users.phone / users.email 已经有 UNIQUE 约束（隐式唯一索引 users_phone_key /
--    users_email_key），000001 里的两个部分索引只是重复写放大，按等值查询也无法
--    提供比唯一索引更好的计划，直接删除。
-- 2) todos 的列表查询固定 WHERE user_id = ? ORDER BY created_at DESC，
--    单列 idx_todos_user_id 之后仍要排序；换成 (user_id, created_at DESC) 复合索引
--    可同时满足过滤与排序。先建复合索引再删旧索引，避免出现无索引窗口。
--
-- 幂等：DROP INDEX IF EXISTS / CREATE INDEX IF NOT EXISTS，可重复执行。

DROP INDEX IF EXISTS idx_users_phone;
DROP INDEX IF EXISTS idx_users_email;

CREATE INDEX IF NOT EXISTS idx_todos_user_created_at ON todos(user_id, created_at DESC);
DROP INDEX IF EXISTS idx_todos_user_id;
