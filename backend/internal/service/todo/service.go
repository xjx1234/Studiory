package todoservice

import (
	"context"
	"errors"

	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/pkg/errcode"
	"backend/pkg/pagination"

	"go.uber.org/zap"
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
	baseservice.LogSupport

	todos repo.TodoRepo
}

type Option func(*serviceImpl)

func New(todos repo.TodoRepo, opts ...Option) Service {
	s := &serviceImpl{todos: todos}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithLogger(logger *zap.Logger) Option {
	return func(s *serviceImpl) {
		s.SetLogger(logger)
	}
}

func (s *serviceImpl) List(ctx context.Context, userID string, page pagination.Query) (pagination.List[Item], *errcode.Error) {
	uid, e := baseservice.ParseUUID(userID)
	if e != nil {
		return pagination.List[Item]{}, e
	}

	total, err := s.todos.CountByUserID(ctx, uid)
	if err != nil {
		s.LogInternal("List count todos", err, baseservice.UserIDField(userID))
		return pagination.List[Item]{}, errcode.ErrInternal
	}

	limit := page.LimitInt32()
	offset := page.OffsetInt32()
	rows, err := s.todos.ListByUserIDPaginated(ctx, uid, limit, offset)
	if err != nil {
		s.LogInternal("List todos", err, baseservice.UserIDField(userID))
		return pagination.List[Item]{}, errcode.ErrInternal
	}

	items := make([]Item, 0, len(rows))
	for i := range rows {
		items = append(items, toItem(&rows[i]))
	}

	return pagination.NewList(items, page, total), nil
}

func (s *serviceImpl) Get(ctx context.Context, userID, todoID string) (*Item, *errcode.Error) {
	uid, tid, e := baseservice.ParseUUIDPair(userID, todoID)
	if e != nil {
		return nil, e
	}

	row, err := s.todos.GetByIDAndUserID(ctx, tid, uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("Get todo", err, baseservice.UserIDField(userID), zap.String("todo_id", todoID))
		return nil, errcode.ErrInternal
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Create(ctx context.Context, userID string, in *CreateInput) (*Item, *errcode.Error) {
	uid, e := baseservice.ParseUUID(userID)
	if e != nil {
		return nil, e
	}
	if in == nil || in.Title == "" {
		return nil, errcode.ErrValidation
	}

	row, err := s.todos.Create(ctx, uid, in.Title)
	if err != nil {
		s.LogInternal("Create todo", err, baseservice.UserIDField(userID))
		return nil, errcode.ErrInternal
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Update(ctx context.Context, userID, todoID string, in *UpdateInput) (*Item, *errcode.Error) {
	uid, tid, e := baseservice.ParseUUIDPair(userID, todoID)
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
		s.LogInternal("Update todo", err, baseservice.UserIDField(userID), zap.String("todo_id", todoID))
		return nil, errcode.ErrInternal
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Delete(ctx context.Context, userID, todoID string) *errcode.Error {
	uid, tid, e := baseservice.ParseUUIDPair(userID, todoID)
	if e != nil {
		return e
	}

	err := s.todos.Delete(ctx, tid, uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return errcode.ErrNotFound
		}
		s.LogInternal("Delete todo", err, baseservice.UserIDField(userID), zap.String("todo_id", todoID))
		return errcode.ErrInternal
	}
	return nil
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
