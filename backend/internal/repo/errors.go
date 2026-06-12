package repo

import "errors"

var (
	// ErrNotFound 记录不存在（repo 层统一使用，屏蔽底层 pgx.ErrNoRows）。
	ErrNotFound = errors.New("record not found")

	// ErrAlreadyExists 唯一约束冲突（repo 层统一使用，屏蔽底层 PostgreSQL 23505）。
	ErrAlreadyExists = errors.New("record already exists")
)
