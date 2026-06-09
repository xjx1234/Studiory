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

type UserRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByPhone(ctx context.Context, phone string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)

	Create(ctx context.Context, params *CreateUserParams) (*User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, nickname, avatar string) (*User, error)
}
