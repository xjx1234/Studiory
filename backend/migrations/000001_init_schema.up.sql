-- 拾习社 PostgreSQL 初始 schema
-- 用户、学科、工具、单词集、单词

-- 启用 UUID 扩展（PostgreSQL 13+ 自带 gen_random_uuid()，无需扩展）
-- 若使用 PG < 13，可取消下一行注释: CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 用户表（支持手机/邮箱/第三方登录）
CREATE TABLE users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone      VARCHAR(20) UNIQUE,
    email      VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255),
    nickname   VARCHAR(100) NOT NULL DEFAULT '',
    avatar     VARCHAR(500) NOT NULL DEFAULT '',
    role       VARCHAR(20) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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

-- 学科
CREATE TABLE subjects (
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(32) NOT NULL UNIQUE,
    name VARCHAR(64) NOT NULL
);

-- 工具（归属某学科）
CREATE TABLE tools (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id   UUID NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    code        VARCHAR(64) NOT NULL,
    name        VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    UNIQUE(subject_id, code)
);

CREATE INDEX idx_tools_subject_id ON tools(subject_id);

-- 单词集（如：三年级上册 Unit 1）
CREATE TABLE word_sets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code       VARCHAR(64) NOT NULL UNIQUE,
    name       VARCHAR(128) NOT NULL,
    grade      VARCHAR(32) NOT NULL DEFAULT '',
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_word_sets_grade ON word_sets(grade);
CREATE INDEX idx_word_sets_enabled ON word_sets(enabled) WHERE enabled = true;

-- 单词（归属某单词集）
CREATE TABLE words (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    word_set_id UUID NOT NULL REFERENCES word_sets(id) ON DELETE CASCADE,
    text        VARCHAR(128) NOT NULL,
    meaning     VARCHAR(512) NOT NULL DEFAULT '',
    phonetic    VARCHAR(128) NOT NULL DEFAULT '',
    audio_url   VARCHAR(500) NOT NULL DEFAULT '',
    tags        JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_words_word_set_id ON words(word_set_id);
CREATE INDEX idx_words_text ON words(word_set_id, text);
