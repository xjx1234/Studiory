package userservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/repo"
	"backend/pkg/errcode"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
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

// ChangePasswordInput 改密请求。
type ChangePasswordInput struct {
	OldPassword string
	NewPassword string
}

// Service 定义用户模块业务入口（profile 相关）。
type Service interface {
	GetProfile(ctx context.Context, userID string) (*ProfileResult, *errcode.Error)
	UpdateProfile(ctx context.Context, userID string, in *UpdateProfileInput) (*ProfileResult, *errcode.Error)
	ChangePassword(ctx context.Context, userID string, in *ChangePasswordInput) *errcode.Error
}

type userServiceImpl struct {
	users      repo.UserRepo
	rdb        redis.UniversalClient
	keyPrefix  string
	accessTTL  time.Duration
	logger     *zap.Logger
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

// WithRevokeSupport 注入 Redis，改密后立即吊销该用户的旧 access token。
func WithRevokeSupport(rdb redis.UniversalClient, keyPrefix string, accessTTL time.Duration) Option {
	return func(s *userServiceImpl) {
		s.rdb = rdb
		s.keyPrefix = keyPrefix
		s.accessTTL = accessTTL
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

func (s *userServiceImpl) ChangePassword(ctx context.Context, userID string, in *ChangePasswordInput) *errcode.Error {
	if in == nil || in.OldPassword == "" || in.NewPassword == "" {
		return errcode.ErrBadRequest
	}

	id, err := uuid.Parse(userID)
	if err != nil {
		return errcode.ErrBadRequest
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return errcode.ErrNotFound
		}
		s.logInternal("ChangePassword lookup user", err)
		return errcode.ErrInternal
	}

	if user.PasswordHash == nil || *user.PasswordHash == "" {
		return errcode.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(in.OldPassword)); err != nil {
		return errcode.ErrWrongPassword
	}

	if in.NewPassword == in.OldPassword {
		return errcode.ErrSamePassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.logInternal("ChangePassword hash new password", err)
		return errcode.ErrInternal
	}

	if _, err := s.users.UpdatePassword(ctx, id, string(hash)); err != nil {
		s.logInternal("ChangePassword update password", err)
		return errcode.ErrInternal
	}

	// 改密后吊销该用户所有旧 access token
	if s.rdb != nil {
		revokeKey := fmt.Sprintf("%s:revoke:uid:%s", s.keyPrefix, userID)
		if err := s.rdb.Set(ctx, revokeKey, time.Now().Unix(), s.accessTTL).Err(); err != nil {
			s.logInternal("ChangePassword revoke access tokens", err)
		}
	}

	return nil
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
