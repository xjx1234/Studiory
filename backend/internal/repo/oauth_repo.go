package repo

import (
	"context"

	"github.com/google/uuid"
)

type OAuthRepo interface {
	GetOAuth(ctx context.Context, provider, openID string) (*UserOAuth, error)
	CreateOAuth(ctx context.Context, userID uuid.UUID, provider, openID string) (*UserOAuth, error)
	GetUserByOAuth(ctx context.Context, provider, openID string) (*User, error)
}
