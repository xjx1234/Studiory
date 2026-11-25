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

-- 与 PG 侧 000006_admin_audit_logs 对齐：后台管理操作审计，只追加。
-- 注意：000001 是合并后的初始迁移，已建库的环境不会重跑本文件；
-- 这些环境需要手工执行本段 CREATE TABLE（字段与 PG 保持一致）。
CREATE TABLE admin_audit_logs (
    id           CHAR(36) NOT NULL PRIMARY KEY,
    operator_id  CHAR(36) NULL,
    target_id    CHAR(36) NULL,
    action       VARCHAR(64) NOT NULL,
    before_value JSON NULL,
    after_value  JSON NULL,
    created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_admin_audit_logs_created_at (created_at DESC),
    INDEX idx_admin_audit_logs_operator (operator_id, created_at DESC),
    INDEX idx_admin_audit_logs_target (target_id, created_at DESC),
    CONSTRAINT fk_admin_audit_operator FOREIGN KEY (operator_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_admin_audit_target FOREIGN KEY (target_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
