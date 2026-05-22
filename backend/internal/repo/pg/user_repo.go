package pg

import (
	"context"
	"errors"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type userRepo struct {
	q *sqlcgen.Queries
}

func (r *userRepo) GetByID(ctx context.Context, id uuid.UUID) (*repo.User, error) {
	row, err := r.q.GetUserByID(ctx, pgtype.UUID{Bytes: to16(id), Valid: true})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

func (r *userRepo) GetByPhone(ctx context.Context, phone string) (*repo.User, error) {
	row, err := r.q.GetUserByPhone(ctx, pgtype.Text{String: phone, Valid: true})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*repo.User, error) {
	row, err := r.q.GetUserByEmail(ctx, pgtype.Text{String: email, Valid: true})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

func (r *userRepo) Create(ctx context.Context, phone, email, passwordHash *string, nickname, avatar, role string) (*repo.User, error) {
	params := &sqlcgen.CreateUserParams{
		Phone:        textFromPtr(phone),
		Email:        textFromPtr(email),
		PasswordHash: textFromPtr(passwordHash),
		Nickname:     nickname,
		Avatar:       avatar,
		Role:         role,
	}
	row, err := r.q.CreateUser(ctx, params)
	if err != nil {
		return nil, err
	}
	return userFromRow(row), nil
}

func (r *userRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) (*repo.User, error) {
	row, err := r.q.UpdateUserPassword(ctx, &sqlcgen.UpdateUserPasswordParams{
		ID:           pgtype.UUID{Bytes: to16(id), Valid: true},
		PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

func (r *userRepo) UpdateProfile(ctx context.Context, id uuid.UUID, nickname, avatar string) (*repo.User, error) {
	row, err := r.q.UpdateUserProfile(ctx, &sqlcgen.UpdateUserProfileParams{
		ID:       pgtype.UUID{Bytes: to16(id), Valid: true},
		Nickname: nickname,
		Avatar:   avatar,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

// wrapErr 将 pgx.ErrNoRows 统一转换为 repo.ErrNotFound，屏蔽底层 pgx 细节。
func wrapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ErrNotFound
	}
	return err
}

func textFromPtr(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *p, Valid: true}
}

func to16(id uuid.UUID) [16]byte {
	var b [16]byte
	copy(b[:], id[:])
	return b
}
