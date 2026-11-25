-- 审计日志属于合规留痕，回滚结构会一并丢弃历史记录；
-- 仅在建表失败或本地重建环境时使用。
DROP TABLE IF EXISTS admin_audit_logs;
