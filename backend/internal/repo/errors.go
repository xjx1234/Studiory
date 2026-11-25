package repo

import "errors"

var (
	// ErrNotFound 记录不存在（repo 层统一使用，屏蔽底层 pgx.ErrNoRows）。
	ErrNotFound = errors.New("record not found")

	// ErrAlreadyExists 唯一约束冲突（repo 层统一使用，屏蔽底层 PostgreSQL 23505）。
	ErrAlreadyExists = errors.New("record already exists")

	// ErrLastAdmin 该操作会让系统失去最后一个可用管理员（active admin），已被拒绝。
	// 由 UpdateRole / UpdateStatus 的守卫条件返回，调用方应映射为业务错误而非 500。
	ErrLastAdmin = errors.New("cannot remove the last active admin")
)
