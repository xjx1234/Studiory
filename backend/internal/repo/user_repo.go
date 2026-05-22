package repo

import (
	"context"

	"github.com/google/uuid"
)

type UserRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByPhone(ctx context.Context, phone string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)

	Create(ctx context.Context, phone, email, passwordHash *string, nickname, avatar, role string) (*User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, nickname, avatar string) (*User, error)
}
