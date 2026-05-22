package pg

import (
	"time"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func uuidFromPG(v pgtype.UUID) uuid.UUID {
	if !v.Valid {
		return uuid.Nil
	}
	id, _ := uuid.FromBytes(v.Bytes[:])
	return id
}

func ptrStringFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func userFromRow(u *sqlcgen.User) *repo.User {
	return &repo.User{
		ID:           uuidFromPG(u.ID),
		Phone:        ptrStringFromText(u.Phone),
		Email:        ptrStringFromText(u.Email),
		PasswordHash: ptrStringFromText(u.PasswordHash),
		Nickname:     u.Nickname,
		Avatar:       u.Avatar,
		Role:         u.Role,
		CreatedAt:    timeFromTimestamptz(u.CreatedAt),
		UpdatedAt:    timeFromTimestamptz(u.UpdatedAt),
	}
}

func oauthFromRow(o *sqlcgen.UserOauth) *repo.UserOAuth {
	return &repo.UserOAuth{
		ID:        uuidFromPG(o.ID),
		UserID:    uuidFromPG(o.UserID),
		Provider:  o.Provider,
		OpenID:    o.OpenID,
		CreatedAt: timeFromTimestamptz(o.CreatedAt),
	}
}

func todoFromRow(t *sqlcgen.Todo) *repo.Todo {
	return &repo.Todo{
		ID:        uuidFromPG(t.ID),
		UserID:    uuidFromPG(t.UserID),
		Title:     t.Title,
		Done:      t.Done,
		CreatedAt: timeFromTimestamptz(t.CreatedAt),
		UpdatedAt: timeFromTimestamptz(t.UpdatedAt),
	}
}
