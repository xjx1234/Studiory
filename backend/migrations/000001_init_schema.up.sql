-- 通用 API 脚手架 - 初始 schema（用户与第三方登录）

-- 用户表（支持手机/邮箱/第三方登录）
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone         VARCHAR(20) UNIQUE,
    email         VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255),
    nickname      VARCHAR(100) NOT NULL DEFAULT '',
    avatar        VARCHAR(500) NOT NULL DEFAULT '',
    role          VARCHAR(20) NOT NULL DEFAULT 'user'
        CHECK (role IN ('admin', 'user')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_phone ON users(phone) WHERE phone IS NOT NULL;
CREATE INDEX idx_users_email ON users(email) WHERE email IS NOT NULL;

-- 第三方绑定（微信/Apple/Google 等）
CREATE TABLE user_oauth (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider   VARCHAR(32) NOT NULL,
    open_id    VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, open_id)
);

CREATE INDEX idx_user_oauth_user_id ON user_oauth(user_id);
