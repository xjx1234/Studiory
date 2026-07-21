package pg

import (
	"context"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type todoRepo struct {
	q *sqlcgen.Queries
}

func (r *todoRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	return r.q.CountTodosByUserID(ctx, pgtype.UUID{Bytes: to16(userID), Valid: true})
}

func (r *todoRepo) ListByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]repo.Todo, error) {
	rows, err := r.q.ListTodosByUserIDPaginated(ctx, &sqlcgen.ListTodosByUserIDPaginatedParams{
		UserID: pgtype.UUID{Bytes: to16(userID), Valid: true},
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]repo.Todo, 0, len(rows))
	for _, row := range rows {
		out = append(out, *todoFromRow(row))
	}
	return out, nil
}

func (r *todoRepo) GetByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*repo.Todo, error) {
	row, err := r.q.GetTodoByIDAndUserID(ctx, &sqlcgen.GetTodoByIDAndUserIDParams{
		ID:     pgtype.UUID{Bytes: to16(id), Valid: true},
		UserID: pgtype.UUID{Bytes: to16(userID), Valid: true},
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return todoFromRow(row), nil
}

func (r *todoRepo) Create(ctx context.Context, userID uuid.UUID, title string) (*repo.Todo, error) {
	row, err := r.q.CreateTodo(ctx, &sqlcgen.CreateTodoParams{
		UserID: pgtype.UUID{Bytes: to16(userID), Valid: true},
		Title:  title,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return todoFromRow(row), nil
}

func (r *todoRepo) Update(ctx context.Context, id, userID uuid.UUID, title string, done bool) (*repo.Todo, error) {
	row, err := r.q.UpdateTodo(ctx, &sqlcgen.UpdateTodoParams{
		ID:     pgtype.UUID{Bytes: to16(id), Valid: true},
		UserID: pgtype.UUID{Bytes: to16(userID), Valid: true},
		Title:  title,
		Done:   done,
	})
	if err != nil {
		return nil, wrapErr(err)
	}
	return todoFromRow(row), nil
}

func (r *todoRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.q.DeleteTodo(ctx, &sqlcgen.DeleteTodoParams{
		ID:     pgtype.UUID{Bytes: to16(id), Valid: true},
		UserID: pgtype.UUID{Bytes: to16(userID), Valid: true},
	})
	return wrapErr(err)
}
