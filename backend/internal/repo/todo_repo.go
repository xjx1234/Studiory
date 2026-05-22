package repo

import (
	"context"

	"github.com/google/uuid"
)

type TodoRepo interface {
	CountByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
	ListByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]Todo, error)
	GetByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*Todo, error)
	Create(ctx context.Context, userID uuid.UUID, title string) (*Todo, error)
	Update(ctx context.Context, id, userID uuid.UUID, title string, done bool) (*Todo, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
}
