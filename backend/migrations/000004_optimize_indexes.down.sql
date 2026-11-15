-- 回滚 000004：恢复 000001/000002 中的索引，并删除复合索引。
-- 幂等：CREATE INDEX IF NOT EXISTS / DROP INDEX IF EXISTS，可重复执行。

DROP INDEX IF EXISTS idx_todos_user_created_at;

CREATE INDEX IF NOT EXISTS idx_todos_user_id ON todos(user_id);

CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone) WHERE phone IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email IS NOT NULL;
