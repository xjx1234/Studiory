package repo

import (
	"time"

	"github.com/google/uuid"
)

// 说明：
// - repo 层 types 是“数据访问层的稳定结构”，供 usecase 使用。
// - 不直接暴露 sqlc 生成的 pgtype 类型，避免上层被数据库细节绑死。

type User struct {
	ID           uuid.UUID
	Phone        *string
	Email        *string
	PasswordHash *string
	Nickname     string
	Avatar       string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// 用户角色常量（与 users.role 字段保持一致）。
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type UserOAuth struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Provider  string
	OpenID    string
	CreatedAt time.Time
}

// Todo 示例模块实体（新业务可在本文件或独立 types 中定义领域结构）。
type Todo struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	Done      bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
