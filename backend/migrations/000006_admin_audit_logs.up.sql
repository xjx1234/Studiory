-- 后台管理操作审计日志：谁（operator_id）对谁（target_id）做了什么（action），
-- 以及改动前后的值（before_value / after_value）。
--
-- 设计取舍：
-- - 只追加（append-only）：业务代码没有 UPDATE / DELETE 入口，便于事后追溯；
-- - before/after 只存变更字段（如 {"role":"user"}），不复制整行，避免把手机号/邮箱等 PII 扩散到审计表；
-- - 两个用户外键都用 ON DELETE SET NULL：账号被物理删除时审计记录仍保留，只是无法再解析出是谁。
CREATE TABLE admin_audit_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_id  UUID REFERENCES users (id) ON DELETE SET NULL,
    target_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    action       VARCHAR(64) NOT NULL,
    before_value JSONB,
    after_value  JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_audit_logs_created_at ON admin_audit_logs (created_at DESC);
CREATE INDEX idx_admin_audit_logs_operator ON admin_audit_logs (operator_id, created_at DESC);
CREATE INDEX idx_admin_audit_logs_target ON admin_audit_logs (target_id, created_at DESC);
