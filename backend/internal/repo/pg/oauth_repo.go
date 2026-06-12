package pg

import (
	"context"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type oauthRepo struct {
	q *sqlcgen.Queries
}

func (r *oauthRepo) GetOAuth(ctx context.Context, provider, openID string) (*repo.UserOAuth, error) {
	row, err := r.q.GetUserOAuthByProviderOpenID(ctx, &sqlcgen.GetUserOAuthByProviderOpenIDParams{
		Provider: provider,
		OpenID:   openID,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return oauthFromRow(row), nil
}

func (r *oauthRepo) CreateOAuth(ctx context.Context, userID uuid.UUID, provider, openID string) (*repo.UserOAuth, error) {
	row, err := r.q.CreateUserOAuth(ctx, &sqlcgen.CreateUserOAuthParams{
		UserID:   pgtype.UUID{Bytes: to16(userID), Valid: true},
		Provider: provider,
		OpenID:   openID,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return oauthFromRow(row), nil
}

func (r *oauthRepo) GetUserByOAuth(ctx context.Context, provider, openID string) (*repo.User, error) {
	row, err := r.q.GetUserByOAuth(ctx, &sqlcgen.GetUserByOAuthParams{
		Provider: provider,
		OpenID:   openID,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}
