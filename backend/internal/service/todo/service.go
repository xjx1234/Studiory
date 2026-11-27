package todoservice

import (
	"context"
	"errors"

	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/internal/service/domainerr"
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
//
// 错误一律返回 domainerr（Code + cause），不感知 HTTP 状态码与 i18n；
// 映射集中在 internal/http/errors.go。
type Service interface {
	List(ctx context.Context, userID string, page pagination.Query) (pagination.List[Item], error)
	Get(ctx context.Context, userID, todoID string) (*Item, error)
	Create(ctx context.Context, userID string, in *CreateInput) (*Item, error)
	Update(ctx context.Context, userID, todoID string, in *UpdateInput) (*Item, error)
	Delete(ctx context.Context, userID, todoID string) error
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

func (s *serviceImpl) List(ctx context.Context, userID string, page pagination.Query) (pagination.List[Item], error) {
	uid, err := baseservice.ParseUUIDError(userID)
	if err != nil {
		return pagination.List[Item]{}, err
	}

	total, err := s.todos.CountByUserID(ctx, uid)
	if err != nil {
		s.LogInternal(ctx, "List count todos", err, baseservice.UserIDField(userID))
		return pagination.List[Item]{}, domainerr.Wrap(domainerr.CodeInternal, "count todos", err)
	}

	limit := page.LimitInt32()
	offset := page.OffsetInt32()
	rows, err := s.todos.ListByUserIDPaginated(ctx, uid, limit, offset)
	if err != nil {
		s.LogInternal(ctx, "List todos", err, baseservice.UserIDField(userID))
		return pagination.List[Item]{}, domainerr.Wrap(domainerr.CodeInternal, "list todos", err)
	}

	items := make([]Item, 0, len(rows))
	for i := range rows {
		items = append(items, toItem(&rows[i]))
	}

	return pagination.NewList(items, page, total), nil
}

func (s *serviceImpl) Get(ctx context.Context, userID, todoID string) (*Item, error) {
	uid, tid, err := baseservice.ParseUUIDPairError(userID, todoID)
	if err != nil {
		return nil, err
	}

	row, err := s.todos.GetByIDAndUserID(ctx, tid, uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domainerr.Wrap(domainerr.CodeNotFound, "todo not found", err)
		}
		s.LogInternal(ctx, "Get todo", err, baseservice.UserIDField(userID), zap.String("todo_id", todoID))
		return nil, domainerr.Wrap(domainerr.CodeInternal, "get todo", err)
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Create(ctx context.Context, userID string, in *CreateInput) (*Item, error) {
	uid, err := baseservice.ParseUUIDError(userID)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Title == "" {
		return nil, domainerr.New(domainerr.CodeValidation, "title is required")
	}

	row, err := s.todos.Create(ctx, uid, in.Title)
	if err != nil {
		s.LogInternal(ctx, "Create todo", err, baseservice.UserIDField(userID))
		return nil, domainerr.Wrap(domainerr.CodeInternal, "create todo", err)
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Update(ctx context.Context, userID, todoID string, in *UpdateInput) (*Item, error) {
	uid, tid, err := baseservice.ParseUUIDPairError(userID, todoID)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Title == "" {
		return nil, domainerr.New(domainerr.CodeValidation, "title is required")
	}

	row, err := s.todos.Update(ctx, tid, uid, in.Title, in.Done)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domainerr.Wrap(domainerr.CodeNotFound, "todo not found", err)
		}
		s.LogInternal(ctx, "Update todo", err, baseservice.UserIDField(userID), zap.String("todo_id", todoID))
		return nil, domainerr.Wrap(domainerr.CodeInternal, "update todo", err)
	}
	item := toItem(row)
	return &item, nil
}

func (s *serviceImpl) Delete(ctx context.Context, userID, todoID string) error {
	uid, tid, err := baseservice.ParseUUIDPairError(userID, todoID)
	if err != nil {
		return err
	}

	err = s.todos.Delete(ctx, tid, uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domainerr.Wrap(domainerr.CodeNotFound, "todo not found", err)
		}
		s.LogInternal(ctx, "Delete todo", err, baseservice.UserIDField(userID), zap.String("todo_id", todoID))
		return domainerr.Wrap(domainerr.CodeInternal, "delete todo", err)
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
