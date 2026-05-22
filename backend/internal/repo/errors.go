package repo

import "errors"

// ErrNotFound 记录不存在（repo 层统一使用，屏蔽底层 pgx.ErrNoRows）。
var ErrNotFound = errors.New("record not found")
