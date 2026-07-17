package todoservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/repo"
	"backend/pkg/errcode"
	"backend/pkg/pagination"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeTodoRepo struct {
	items []repo.Todo

	// 错误注入：非 nil 时对应方法直接返回该错误，用于覆盖 service 层的内部错误分支。
	countErr  error
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func (r *fakeTodoRepo) CountByUserID(_ context.Context, userID uuid.UUID) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	var count int64
	for _, item := range r.items {
		if item.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (r *fakeTodoRepo) ListByUserIDPaginated(_ context.Context, userID uuid.UUID, limit, offset int32) ([]repo.Todo, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
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
	if r.getErr != nil {
		return nil, r.getErr
	}
	for _, item := range r.items {
		if item.ID == id && item.UserID == userID {
			return &item, nil
		}
	}
	return nil, repo.ErrNotFound
}

func (r *fakeTodoRepo) Create(_ context.Context, userID uuid.UUID, title string) (*repo.Todo, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
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
	if r.updateErr != nil {
		return nil, r.updateErr
	}
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
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for i := range r.items {
		if r.items[i].ID == id && r.items[i].UserID == userID {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return nil
		}
	}
	return repo.ErrNotFound
}

func TestWithLoggerOption(t *testing.T) {
	svc := New(&fakeTodoRepo{}, WithLogger(zap.NewNop()))
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
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

func TestList_InvalidUserID(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	_, e := svc.List(context.Background(), "not-a-uuid", pagination.Query{Page: 1, PageSize: 20})
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestList_CountInternalError(t *testing.T) {
	svc := New(&fakeTodoRepo{countErr: errors.New("db down")})

	_, e := svc.List(context.Background(), uuid.New().String(), pagination.Query{Page: 1, PageSize: 20})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestList_ListInternalError(t *testing.T) {
	svc := New(&fakeTodoRepo{listErr: errors.New("db down")})

	_, e := svc.List(context.Background(), uuid.New().String(), pagination.Query{Page: 1, PageSize: 20})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestGet_Success(t *testing.T) {
	userID := uuid.New()
	repoImpl := &fakeTodoRepo{}
	svc := New(repoImpl)

	created, e := svc.Create(context.Background(), userID.String(), &CreateInput{Title: "todo-1"})
	if e != nil {
		t.Fatalf("create failed: %+v", e)
	}

	got, e := svc.Get(context.Background(), userID.String(), created.ID)
	if e != nil {
		t.Fatalf("get failed: %+v", e)
	}
	if got.Title != "todo-1" {
		t.Fatalf("unexpected title: %s", got.Title)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	_, e := svc.Get(context.Background(), uuid.New().String(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestGet_InvalidIDs(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	cases := [][2]string{
		{"not-a-uuid", uuid.New().String()},
		{uuid.New().String(), "not-a-uuid"},
	}
	for _, c := range cases {
		_, e := svc.Get(context.Background(), c[0], c[1])
		if e == nil || e.Code != errcode.ErrBadRequest.Code {
			t.Errorf("userID=%q todoID=%q: expected ErrBadRequest, got %+v", c[0], c[1], e)
		}
	}
}

func TestGet_InternalError(t *testing.T) {
	svc := New(&fakeTodoRepo{getErr: errors.New("db down")})

	_, e := svc.Get(context.Background(), uuid.New().String(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestCreate_InvalidUserID(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	_, e := svc.Create(context.Background(), "not-a-uuid", &CreateInput{Title: "x"})
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	svc := New(&fakeTodoRepo{})
	userID := uuid.New().String()

	cases := []*CreateInput{nil, {}, {Title: ""}}
	for _, in := range cases {
		_, e := svc.Create(context.Background(), userID, in)
		if e == nil || e.Code != errcode.ErrValidation.Code {
			t.Errorf("input=%+v: expected ErrValidation, got %+v", in, e)
		}
	}
}

func TestCreate_InternalError(t *testing.T) {
	svc := New(&fakeTodoRepo{createErr: errors.New("db down")})

	_, e := svc.Create(context.Background(), uuid.New().String(), &CreateInput{Title: "x"})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestUpdate_Success(t *testing.T) {
	userID := uuid.New()
	repoImpl := &fakeTodoRepo{}
	svc := New(repoImpl)

	created, e := svc.Create(context.Background(), userID.String(), &CreateInput{Title: "old"})
	if e != nil {
		t.Fatalf("create failed: %+v", e)
	}

	updated, e := svc.Update(context.Background(), userID.String(), created.ID, &UpdateInput{Title: "new", Done: true})
	if e != nil {
		t.Fatalf("update failed: %+v", e)
	}
	if updated.Title != "new" || !updated.Done {
		t.Fatalf("unexpected item: %+v", updated)
	}
}

func TestUpdate_InvalidIDs(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	_, e := svc.Update(context.Background(), "not-a-uuid", uuid.New().String(), &UpdateInput{Title: "x"})
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestUpdate_ValidationError(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	_, e := svc.Update(context.Background(), uuid.New().String(), uuid.New().String(), &UpdateInput{Title: ""})
	if e == nil || e.Code != errcode.ErrValidation.Code {
		t.Fatalf("expected ErrValidation, got %+v", e)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	_, e := svc.Update(context.Background(), uuid.New().String(), uuid.New().String(), &UpdateInput{Title: "x"})
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestUpdate_InternalError(t *testing.T) {
	svc := New(&fakeTodoRepo{updateErr: errors.New("db down")})

	_, e := svc.Update(context.Background(), uuid.New().String(), uuid.New().String(), &UpdateInput{Title: "x"})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestDelete_Success(t *testing.T) {
	userID := uuid.New()
	repoImpl := &fakeTodoRepo{}
	svc := New(repoImpl)

	created, e := svc.Create(context.Background(), userID.String(), &CreateInput{Title: "to-delete"})
	if e != nil {
		t.Fatalf("create failed: %+v", e)
	}

	if e := svc.Delete(context.Background(), userID.String(), created.ID); e != nil {
		t.Fatalf("delete failed: %+v", e)
	}

	if _, e := svc.Get(context.Background(), userID.String(), created.ID); e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected item to be gone after delete, got %+v", e)
	}
}

func TestDelete_InvalidIDs(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	e := svc.Delete(context.Background(), "not-a-uuid", uuid.New().String())
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := New(&fakeTodoRepo{})

	e := svc.Delete(context.Background(), uuid.New().String(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestDelete_InternalError(t *testing.T) {
	svc := New(&fakeTodoRepo{deleteErr: errors.New("db down")})

	e := svc.Delete(context.Background(), uuid.New().String(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}
