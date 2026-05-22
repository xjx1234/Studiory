package todoservice

import (
	"context"
	"errors"

	"backend/internal/repo"
	"backend/pkg/errcode"
	"backend/pkg/pagination"

	"github.com/google/uuid"
)

// Item 对外返回的待办结构。
type Item struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CreateInput 创建待办。
type CreateInput struct {
	Title string
}

// UpdateInput 更新待办（全量字段，示例保持简单）。
type UpdateInput struct {
	Title string
	Done  bool
}

// Service 示例模块业务入口。
type Service interface {
	List(ctx context.Context, userID string, page pagination.Query) (pagination.List[Item], *errcode.Error)
	Get(ctx context.Context, userID, todoID string) (*Item, *errcode.Error)
	Create(ctx context.Context, userID string, in *CreateInput) (*Item, *errcode.Error)
	Update(ctx context.Context, userID, todoID string, in *UpdateInput) (*Item, *errcode.Error)
	Delete(ctx context.Context, userID, todoID string) *errcode.Error
}

type serviceImpl struct {
	todos repo.TodoRepo
}

func New(todos repo.TodoRepo) Service {
	return &serviceImpl{todos: todos}
}

func (s *serviceImpl) List(ctx context.Context, userID string, page pagination.Query) (pagination.List[Item], *errcode.Error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return pagination.List[Item]{}, errcode.ErrBadRequest
	}

	total, err := s.todos.CountByUserID(ctx, uid)
	if err != nil {
		return pagination.List[Item]{}, errcode.ErrInternal
	}

	limit := int32(page.PageSize)
	offset := int32(page.Offset())
	rows, err := s.todos.ListByUserIDPaginated(ctx, uid, limit, offset)
	if err != nil {
		return pagination.List[Item]{}, errcode.ErrInternal
	}

	items := make([]Item, 0, len(rows))
	for i := range rows {
		items = append(items, toItem(&rows[i]))
	}

	return pagination.NewList(items, page, total), nil
}

func (s *serviceImpl) Get(ctx context.Context, userID, todoID string) (*Item, *errcode.Error) {
	uid, tid, e := parseIDs(userID, todoID)
	if e != nil {
		return nil, e
	}

	row, err := s.todos.GetByIDAndUserID(ctx, tid, uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, errcode.ErrInternal
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Create(ctx context.Context, userID string, in *CreateInput) (*Item, *errcode.Error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, errcode.ErrBadRequest
	}
	if in == nil || in.Title == "" {
		return nil, errcode.ErrValidation
	}

	row, err := s.todos.Create(ctx, uid, in.Title)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Update(ctx context.Context, userID, todoID string, in *UpdateInput) (*Item, *errcode.Error) {
	uid, tid, e := parseIDs(userID, todoID)
	if e != nil {
		return nil, e
	}
	if in == nil || in.Title == "" {
		return nil, errcode.ErrValidation
	}

	row, err := s.todos.Update(ctx, tid, uid, in.Title, in.Done)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, errcode.ErrInternal
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Delete(ctx context.Context, userID, todoID string) *errcode.Error {
	uid, tid, e := parseIDs(userID, todoID)
	if e != nil {
		return e
	}

	err := s.todos.Delete(ctx, tid, uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return errcode.ErrNotFound
		}
		return errcode.ErrInternal
	}
	return nil
}

func parseIDs(userID, todoID string) (uuid.UUID, uuid.UUID, *errcode.Error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return uuid.Nil, uuid.Nil, errcode.ErrBadRequest
	}
	tid, err := uuid.Parse(todoID)
	if err != nil {
		return uuid.Nil, uuid.Nil, errcode.ErrBadRequest
	}
	return uid, tid, nil
}

func toItem(t *repo.Todo) Item {
	return Item{
		ID:        t.ID.String(),
		Title:     t.Title,
		Done:      t.Done,
		CreatedAt: t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
