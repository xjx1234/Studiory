-- 用户状态：用于后台管理启用/禁用账号
ALTER TABLE users
    ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled'));

-- 后台用户列表按创建时间倒序分页
CREATE INDEX idx_users_created_at ON users(created_at DESC);
