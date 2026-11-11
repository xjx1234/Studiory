CREATE TABLE users (
    id            CHAR(36) NOT NULL PRIMARY KEY,
    phone         VARCHAR(20) NULL UNIQUE,
    email         VARCHAR(255) NULL UNIQUE,
    password_hash VARCHAR(255) NULL,
    nickname      VARCHAR(100) NOT NULL DEFAULT '',
    avatar        VARCHAR(500) NOT NULL DEFAULT '',
    role          VARCHAR(20) NOT NULL DEFAULT 'user',
    created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT chk_users_role CHECK (role IN ('admin', 'user')),
    INDEX idx_users_created_at (created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE user_oauth (
    id         CHAR(36) NOT NULL PRIMARY KEY,
    user_id    CHAR(36) NOT NULL,
    provider   VARCHAR(32) NOT NULL,
    open_id    VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_user_oauth_provider_open_id (provider, open_id),
    INDEX idx_user_oauth_user_id (user_id),
    CONSTRAINT fk_user_oauth_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE todos (
    id         CHAR(36) NOT NULL PRIMARY KEY,
    user_id    CHAR(36) NOT NULL,
    title      VARCHAR(200) NOT NULL,
    done       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_todos_user_created (user_id, created_at DESC),
    CONSTRAINT fk_todos_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE users ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active';
-- 与 PG 侧 000003_user_status 对齐：status 只允许 active/disabled
ALTER TABLE users ADD CONSTRAINT chk_users_status CHECK (status IN ('active', 'disabled'));
