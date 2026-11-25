package pg

import (
	"context"
	"encoding/json"
	"errors"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/jackc/pgx/v5/pgtype"
)

type auditRepo struct {
	q *sqlcgen.Queries
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

	var operator pgtype.UUID
	if entry.OperatorID != nil {
		operator = pgtype.UUID{Bytes: to16(*entry.OperatorID), Valid: true}
	}
	var target pgtype.UUID
	if entry.TargetID != nil {
		target = pgtype.UUID{Bytes: to16(*entry.TargetID), Valid: true}
	}

	return r.q.CreateAdminAuditLog(ctx, &sqlcgen.CreateAdminAuditLogParams{
		OperatorID:  operator,
		TargetID:    target,
		Action:      entry.Action,
		BeforeValue: before,
		AfterValue:  after,
	})
}

// encodeAuditValues 把变更字段序列化成 JSONB 参数；nil map 写成 JSON null。
func encodeAuditValues(values map[string]string) ([]byte, error) {
	if values == nil {
		return []byte("null"), nil
	}
	return json.Marshal(values)
}
