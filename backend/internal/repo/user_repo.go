package repo

import (
	"context"

	"github.com/google/uuid"
)

// CreateUserParams 创建用户入参。
type CreateUserParams struct {
	Phone        *string
	Email        *string
	PasswordHash *string
	Nickname     string
	Avatar       string
	Role         string
}

// ListUsersParams 后台用户列表查询入参。
// Keyword 为空时不做模糊匹配；Status 为空时不做状态过滤。
type ListUsersParams struct {
	Keyword string
	Status  string
	Limit   int32
	Offset  int32
}

type UserRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByPhone(ctx context.Context, phone string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)

	Create(ctx context.Context, params *CreateUserParams) (*User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, nickname, avatar string) (*User, error)

	// 后台管理
	List(ctx context.Context, params *ListUsersParams) ([]*User, error)
	Count(ctx context.Context, keyword, status string) (int64, error)
	UpdateRole(ctx context.Context, id uuid.UUID, role string) (*User, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*User, error)
}
