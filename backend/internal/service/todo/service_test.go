package todoservice

import (
	"context"
	"testing"
	"time"

	"backend/internal/repo"
	"backend/pkg/pagination"

	"github.com/google/uuid"
)

type fakeTodoRepo struct {
	items []repo.Todo
}

func (r *fakeTodoRepo) CountByUserID(_ context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	for _, item := range r.items {
		if item.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (r *fakeTodoRepo) ListByUserIDPaginated(_ context.Context, userID uuid.UUID, limit, offset int32) ([]repo.Todo, error) {
	var filtered []repo.Todo
	for _, item := range r.items {
		if item.UserID == userID {
			filtered = append(filtered, item)
		}
	}

	if offset >= int32(len(filtered)) {
		return []repo.Todo{}, nil
	}

	end := offset + limit
	if end > int32(len(filtered)) {
		end = int32(len(filtered))
	}

	return filtered[offset:end], nil
}

func (r *fakeTodoRepo) GetByIDAndUserID(_ context.Context, id, userID uuid.UUID) (*repo.Todo, error) {
	for _, item := range r.items {
		if item.ID == id && item.UserID == userID {
			return &item, nil
		}
	}
	return nil, repo.ErrNotFound
}

func (r *fakeTodoRepo) Create(_ context.Context, userID uuid.UUID, title string) (*repo.Todo, error) {
	item := repo.Todo{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	r.items = append(r.items, item)
	return &item, nil
}

func (r *fakeTodoRepo) Update(_ context.Context, id, userID uuid.UUID, title string, done bool) (*repo.Todo, error) {
	for i := range r.items {
		if r.items[i].ID == id && r.items[i].UserID == userID {
			r.items[i].Title = title
			r.items[i].Done = done
			r.items[i].UpdatedAt = time.Now()
			return &r.items[i], nil
		}
	}
	return nil, repo.ErrNotFound
}

func (r *fakeTodoRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	for i := range r.items {
		if r.items[i].ID == id && r.items[i].UserID == userID {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return nil
		}
	}
	return repo.ErrNotFound
}

func TestServiceCreateAndListTodos(t *testing.T) {
	userID := uuid.New()
	svc := New(&fakeTodoRepo{})

	created, e := svc.Create(context.Background(), userID.String(), &CreateInput{Title: "first todo"})
	if e != nil {
		t.Fatalf("create failed: %+v", e)
	}
	if created.Title != "first todo" {
		t.Fatalf("unexpected title: %s", created.Title)
	}

	list, e := svc.List(context.Background(), userID.String(), pagination.Query{Page: 1, PageSize: 20})
	if e != nil {
		t.Fatalf("list failed: %+v", e)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}
}
