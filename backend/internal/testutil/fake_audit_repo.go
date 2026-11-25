package testutil

import (
	"context"

	"backend/internal/repo"
)

// FakeAuditRepo 是 repo.AdminAuditRepo 的内存实现。
// 通过 Logs 断言写入了哪些审计记录；Err 非 nil 时模拟写入失败。
type FakeAuditRepo struct {
	Logs []*repo.AdminAuditLog
	Err  error
}

func (r *FakeAuditRepo) Create(_ context.Context, entry *repo.AdminAuditLog) error {
	if r.Err != nil {
		return r.Err
	}
	r.Logs = append(r.Logs, entry)
	return nil
}
