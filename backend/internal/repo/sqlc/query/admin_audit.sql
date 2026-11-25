-- name: CreateAdminAuditLog :exec
INSERT INTO admin_audit_logs (operator_id, target_id, action, before_value, after_value)
VALUES ($1, $2, $3, $4, $5);
