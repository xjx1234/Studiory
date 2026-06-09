package userservice

import (
	"context"
	"errors"

	"backend/internal/repo"
	"backend/pkg/errcode"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ProfileResult 用户资料（对外输出结构）。
type ProfileResult struct {
	ID       string `json:"id"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
}

// UpdateProfileInput 更新资料请求。
type UpdateProfileInput struct {
	Nickname string
	Avatar   string
}

// Service 定义用户模块业务入口（profile 相关）。
type Service interface {
	GetProfile(ctx context.Context, userID string) (*ProfileResult, *errcode.Error)
	UpdateProfile(ctx context.Context, userID string, in *UpdateProfileInput) (*ProfileResult, *errcode.Error)
}

type userServiceImpl struct {
	users  repo.UserRepo
	logger *zap.Logger
}

type Option func(*userServiceImpl)

// New 创建 UserService。
func New(users repo.UserRepo, opts ...Option) Service {
	s := &userServiceImpl{users: users}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithLogger(logger *zap.Logger) Option {
	return func(s *userServiceImpl) {
		s.logger = logger
	}
}

func (s *userServiceImpl) GetProfile(ctx context.Context, userID string) (*ProfileResult, *errcode.Error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, errcode.ErrBadRequest
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.logInternal("GetProfile lookup user", err)
		return nil, errcode.ErrInternal
	}

	return toProfileResult(user), nil
}

func (s *userServiceImpl) UpdateProfile(ctx context.Context, userID string, in *UpdateProfileInput) (*ProfileResult, *errcode.Error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, errcode.ErrBadRequest
	}
	if in == nil {
		return nil, errcode.ErrBadRequest
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.logInternal("UpdateProfile lookup user", err)
		return nil, errcode.ErrInternal
	}

	nickname := user.Nickname
	if in.Nickname != "" {
		nickname = in.Nickname
	}
	avatar := user.Avatar
	if in.Avatar != "" {
		avatar = in.Avatar
	}

	updated, err := s.users.UpdateProfile(ctx, id, nickname, avatar)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.logInternal("UpdateProfile update user", err)
		return nil, errcode.ErrInternal
	}
	return toProfileResult(updated), nil
}

func (s *userServiceImpl) logInternal(op string, err error) {
	if s.logger != nil && err != nil {
		s.logger.Error(op, zap.Error(err))
	}
}

func toProfileResult(u *repo.User) *ProfileResult {
	r := &ProfileResult{
		ID:       u.ID.String(),
		Nickname: u.Nickname,
		Avatar:   u.Avatar,
		Role:     u.Role,
	}
	if u.Phone != nil {
		r.Phone = *u.Phone
	}
	if u.Email != nil {
		r.Email = *u.Email
	}
	return r
}
