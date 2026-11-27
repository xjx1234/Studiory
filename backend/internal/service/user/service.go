package userservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/internal/session"
	"backend/pkg/errcode"

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
	// ChangePassword 修改密码。返回的 bool 表示「旧会话与 access token 是否已成功吊销」：
	// 密码本身已落库，因此 Redis 异常时仍返回成功，但调用方需要据此提示用户/运维。
	ChangePassword(ctx context.Context, userID string, in *ChangePasswordInput) (bool, *errcode.Error)
}

type userServiceImpl struct {
	baseservice.LogSupport

	users     repo.UserRepo
	sessions  *session.Store
	rdb       redis.UniversalClient
	keyPrefix string
	accessTTL time.Duration
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
		s.SetLogger(logger)
	}
}

// WithSessionStore 注入会话存储，改密时吊销全部 session。
func WithSessionStore(store *session.Store) Option {
	return func(s *userServiceImpl) {
		s.sessions = store
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
	id, e := baseservice.ParseUUID(userID)
	if e != nil {
		return nil, e
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal(ctx, "GetProfile lookup user", err, baseservice.UserIDField(userID))
		return nil, errcode.ErrInternal
	}

	return toProfileResult(user), nil
}

func (s *userServiceImpl) UpdateProfile(ctx context.Context, userID string, in *UpdateProfileInput) (*ProfileResult, *errcode.Error) {
	id, e := baseservice.ParseUUID(userID)
	if e != nil {
		return nil, e
	}
	if in == nil {
		return nil, errcode.ErrBadRequest
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal(ctx, "UpdateProfile lookup user", err, baseservice.UserIDField(userID))
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
		s.LogInternal(ctx, "UpdateProfile update user", err, baseservice.UserIDField(userID))
		return nil, errcode.ErrInternal
	}
	return toProfileResult(updated), nil
}

func (s *userServiceImpl) ChangePassword(ctx context.Context, userID string, in *ChangePasswordInput) (bool, *errcode.Error) {
	if in == nil || in.OldPassword == "" || in.NewPassword == "" {
		return false, errcode.ErrBadRequest
	}

	id, e := baseservice.ParseUUID(userID)
	if e != nil {
		return false, e
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return false, errcode.ErrNotFound
		}
		s.LogInternal(ctx, "ChangePassword lookup user", err, baseservice.UserIDField(userID))
		return false, errcode.ErrInternal
	}

	if user.PasswordHash == nil || *user.PasswordHash == "" {
		return false, errcode.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(in.OldPassword)); err != nil {
		return false, errcode.ErrWrongPassword
	}

	if in.NewPassword == in.OldPassword {
		return false, errcode.ErrSamePassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), auth.PasswordHashCost)
	if err != nil {
		s.LogInternal(ctx, "ChangePassword hash new password", err, baseservice.UserIDField(userID))
		return false, errcode.ErrInternal
	}

	if _, err := s.users.UpdatePassword(ctx, id, string(hash)); err != nil {
		s.LogInternal(ctx, "ChangePassword update password", err, baseservice.UserIDField(userID))
		return false, errcode.ErrInternal
	}

	// 改密后吊销该用户所有会话与旧 access token。
	// 键名一律用 canonical UUID（id.String()）：userID 是 JWT claim 原样传进来的字符串，
	// 管理员那边用的又是路径参数，两者形式不一致时写出的键没人读，旧 token 会继续存活。
	//
	// 吊销失败不回滚密码（已经落库），但必须把「未完成」告诉调用方：
	// 否则用户以为所有设备都已踢下线，实际旧 token 还能用到自然过期。
	revokedUserID := id.String()
	revoked := true
	if s.sessions != nil {
		if err := s.sessions.RevokeAll(ctx, revokedUserID); err != nil {
			revoked = false
			s.LogInternal(ctx, "ChangePassword revoke all sessions", err, baseservice.UserIDField(revokedUserID))
		}
	}
	if s.rdb != nil {
		revokeKey := fmt.Sprintf("%s:revoke:uid:%s", s.keyPrefix, revokedUserID)
		if err := s.rdb.Set(ctx, revokeKey, time.Now().Unix(), s.accessTTL).Err(); err != nil {
			revoked = false
			s.LogInternal(ctx, "ChangePassword revoke access tokens", err, baseservice.UserIDField(revokedUserID))
		}
	}

	return revoked, nil
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
