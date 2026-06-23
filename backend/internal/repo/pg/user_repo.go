package pg

import (
	"context"
	"errors"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *userRepo) Create(ctx context.Context, in *repo.CreateUserParams) (*repo.User, error) {
	if in == nil {
		return nil, errors.New("create user params is nil")
	}
	params := &sqlcgen.CreateUserParams{
		Phone:        textFromPtr(in.Phone),
		Email:        textFromPtr(in.Email),
		PasswordHash: textFromPtr(in.PasswordHash),
		Nickname:     in.Nickname,
		Avatar:       in.Avatar,
		Role:         in.Role,
	}
	row, err := r.q.CreateUser(ctx, params)
	if err != nil {
		return nil, wrapErr(err)
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

func (r *userRepo) List(ctx context.Context, params *repo.ListUsersParams) ([]*repo.User, error) {
	if params == nil {
		return nil, errors.New("list users params is nil")
	}
	rows, err := r.q.ListUsers(ctx, &sqlcgen.ListUsersParams{
		Keyword:     params.Keyword,
		Status:      params.Status,
		LimitCount:  params.Limit,
		OffsetCount: params.Offset,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	users := make([]*repo.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromRow(row))
	}
	return users, nil
}

func (r *userRepo) Count(ctx context.Context, keyword, status string) (int64, error) {
	n, err := r.q.CountUsers(ctx, &sqlcgen.CountUsersParams{
		Keyword: keyword,
		Status:  status,
	})
	if err != nil {
		return 0, wrapErr(err)
	}
	return n, nil
}

func (r *userRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) (*repo.User, error) {
	row, err := r.q.UpdateUserRole(ctx, &sqlcgen.UpdateUserRoleParams{
		ID:   pgtype.UUID{Bytes: to16(id), Valid: true},
		Role: role,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

func (r *userRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*repo.User, error) {
	row, err := r.q.UpdateUserStatus(ctx, &sqlcgen.UpdateUserStatusParams{
		ID:     pgtype.UUID{Bytes: to16(id), Valid: true},
		Status: status,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return userFromRow(row), nil
}

// wrapErr 将底层 PostgreSQL/pgx 错误转换为 repo 层稳定错误。
func wrapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return repo.ErrAlreadyExists
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
