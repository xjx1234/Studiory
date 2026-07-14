// Package adminservice 提供后台用户管理能力（列表 / 详情 / 改角色 / 启用禁用）。
// 作为脚手架示例，演示如何基于 RBAC 在 admin 路由下编写管理类业务。
package adminservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/internal/session"
	"backend/pkg/errcode"
	"backend/pkg/pagination"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// UserItem 后台用户视图（对外输出结构）。
type UserItem struct {
	ID        string `json:"id"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListInput 用户列表查询入参。
type ListInput struct {
	Keyword string
	Status  string
}

// Service 定义后台用户管理业务入口。
type Service interface {
	ListUsers(ctx context.Context, in ListInput, page pagination.Query) (pagination.List[UserItem], *errcode.Error)
	GetUser(ctx context.Context, userID string) (*UserItem, *errcode.Error)
	UpdateRole(ctx context.Context, actingUserID, targetUserID, role string) (*UserItem, *errcode.Error)
	SetStatus(ctx context.Context, actingUserID, targetUserID, status string) (*UserItem, *errcode.Error)
}

type adminServiceImpl struct {
	baseservice.LogSupport

	users     repo.UserRepo
	sessions  *session.Store
	rdb       redis.UniversalClient
	keyPrefix string
	accessTTL time.Duration
}

type Option func(*adminServiceImpl)

// New 创建 AdminService。
func New(users repo.UserRepo, opts ...Option) Service {
	s := &adminServiceImpl{users: users}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithLogger(logger *zap.Logger) Option {
	return func(s *adminServiceImpl) {
		s.SetLogger(logger)
	}
}

// WithSessionStore 注入会话存储，禁用用户时吊销全部 session。
func WithSessionStore(store *session.Store) Option {
	return func(s *adminServiceImpl) {
		s.sessions = store
	}
}

// WithRevokeSupport 注入 Redis，禁用用户后立即吊销其旧 access token。
func WithRevokeSupport(rdb redis.UniversalClient, keyPrefix string, accessTTL time.Duration) Option {
	return func(s *adminServiceImpl) {
		s.rdb = rdb
		s.keyPrefix = keyPrefix
		s.accessTTL = accessTTL
	}
}

func (s *adminServiceImpl) ListUsers(ctx context.Context, in ListInput, page pagination.Query) (pagination.List[UserItem], *errcode.Error) {
	var empty pagination.List[UserItem]

	if in.Status != "" && !isValidStatus(in.Status) {
		return empty, errcode.ErrBadRequest
	}

	total, err := s.users.Count(ctx, in.Keyword, in.Status)
	if err != nil {
		s.LogInternal("ListUsers count", err)
		return empty, errcode.ErrInternal
	}

	rows, err := s.users.List(ctx, &repo.ListUsersParams{
		Keyword: in.Keyword,
		Status:  in.Status,
		Limit:   int32(page.PageSize),
		Offset:  int32(page.Offset()),
	})
	if err != nil {
		s.LogInternal("ListUsers list", err)
		return empty, errcode.ErrInternal
	}

	items := make([]UserItem, 0, len(rows))
	for _, u := range rows {
		items = append(items, toUserItem(u))
	}
	return pagination.NewList(items, page, total), nil
}

func (s *adminServiceImpl) GetUser(ctx context.Context, userID string) (*UserItem, *errcode.Error) {
	id, e := baseservice.ParseUUID(userID)
	if e != nil {
		return nil, e
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("GetUser lookup", err, baseservice.UserIDField(userID))
		return nil, errcode.ErrInternal
	}

	item := toUserItem(user)
	return &item, nil
}

func (s *adminServiceImpl) UpdateRole(ctx context.Context, actingUserID, targetUserID, role string) (*UserItem, *errcode.Error) {
	if !isValidRole(role) {
		return nil, errcode.ErrBadRequest
	}
	id, e := baseservice.ParseUUID(targetUserID)
	if e != nil {
		return nil, e
	}
	// 不允许管理员修改自己的角色，避免自我降权后失去管理入口。
	if actingUserID == targetUserID {
		return nil, errcode.ErrCannotModifySelf
	}

	updated, err := s.users.UpdateRole(ctx, id, role)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("UpdateRole update", err,
			baseservice.UserIDField(targetUserID),
			baseservice.ActorUserIDField(actingUserID),
		)
		return nil, errcode.ErrInternal
	}

	item := toUserItem(updated)
	return &item, nil
}

func (s *adminServiceImpl) SetStatus(ctx context.Context, actingUserID, targetUserID, status string) (*UserItem, *errcode.Error) {
	if !isValidStatus(status) {
		return nil, errcode.ErrBadRequest
	}
	id, e := baseservice.ParseUUID(targetUserID)
	if e != nil {
		return nil, e
	}
	// 不允许管理员禁用自己，避免把自己锁在系统外。
	if actingUserID == targetUserID && status == repo.StatusDisabled {
		return nil, errcode.ErrCannotModifySelf
	}

	updated, err := s.users.UpdateStatus(ctx, id, status)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("SetStatus update", err,
			baseservice.UserIDField(targetUserID),
			baseservice.ActorUserIDField(actingUserID),
		)
		return nil, errcode.ErrInternal
	}

	// 禁用后立即吊销该用户所有旧 access token，使其当前会话即时失效。
	if status == repo.StatusDisabled {
		s.revokeAccessTokens(ctx, targetUserID)
	}

	item := toUserItem(updated)
	return &item, nil
}

func (s *adminServiceImpl) revokeAccessTokens(ctx context.Context, userID string) {
	if s.sessions != nil {
		if err := s.sessions.RevokeAll(ctx, userID); err != nil {
			s.LogInternal("SetStatus revoke all sessions", err, baseservice.UserIDField(userID))
		}
	}
	if s.rdb == nil {
		return
	}
	revokeKey := fmt.Sprintf("%s:revoke:uid:%s", s.keyPrefix, userID)
	if err := s.rdb.Set(ctx, revokeKey, time.Now().Unix(), s.accessTTL).Err(); err != nil {
		s.LogInternal("SetStatus revoke access tokens", err, baseservice.UserIDField(userID))
	}
}

func isValidRole(role string) bool {
	return role == repo.RoleAdmin || role == repo.RoleUser
}

func isValidStatus(status string) bool {
	return status == repo.StatusActive || status == repo.StatusDisabled
}

func toUserItem(u *repo.User) UserItem {
	item := UserItem{
		ID:        u.ID.String(),
		Nickname:  u.Nickname,
		Avatar:    u.Avatar,
		Role:      u.Role,
		Status:    u.Status,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
		UpdatedAt: u.UpdatedAt.Format(time.RFC3339),
	}
	if u.Phone != nil {
		item.Phone = *u.Phone
	}
	if u.Email != nil {
		item.Email = *u.Email
	}
	return item
}
