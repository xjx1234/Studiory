package mysql

import (
	"context"
	"encoding/json"
	"errors"

	"backend/internal/repo"

	"github.com/google/uuid"
)

type auditRepo struct {
	q executor
}

func (r *auditRepo) Create(ctx context.Context, entry *repo.AdminAuditLog) error {
	if entry == nil {
		return errors.New("audit log is nil")
	}
	before, err := encodeAuditValues(entry.Before)
	if err != nil {
		return err
	}
	after, err := encodeAuditValues(entry.After)
	if err != nil {
		return err
	}

	var operator, target any
	if entry.OperatorID != nil {
		operator = entry.OperatorID.String()
	}
	if entry.TargetID != nil {
		target = entry.TargetID.String()
	}

	_, err = r.q.ExecContext(ctx,
		`INSERT INTO admin_audit_logs (id, operator_id, target_id, action, before_value, after_value) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), operator, target, entry.Action, string(before), string(after))
	return err
}

// encodeAuditValues 把变更字段序列化成 JSON 文本；nil map 写成 JSON null。
func encodeAuditValues(values map[string]string) ([]byte, error) {
	if values == nil {
		return []byte("null"), nil
	}
	return json.Marshal(values)
}
