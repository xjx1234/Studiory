package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminAuditLog 记录一次后台管理操作：谁（OperatorID）对谁（TargetID）做了什么（Action），
// 以及改动前后的值（Before / After，只放发生变化的字段）。
//
// TargetID 为空表示不针对具体用户的操作；OperatorID 为空表示操作者账号已被物理删除
// （写入时一定有值，是外键 ON DELETE SET NULL 造成的）。
type AdminAuditLog struct {
	ID         uuid.UUID
	OperatorID *uuid.UUID
	TargetID   *uuid.UUID
	Action     string
	Before     map[string]string
	After      map[string]string
	CreatedAt  time.Time
}

// AdminAuditRepo 追加后台管理审计日志。
// 实现必须保证 append-only：不提供更新/删除接口。
type AdminAuditRepo interface {
	Create(ctx context.Context, log *AdminAuditLog) error
}
