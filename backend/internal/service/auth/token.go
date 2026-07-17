package authservice

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/internal/session"
	"backend/pkg/errcode"
)

const refreshBlacklistTTL = 7 * 24 * time.Hour

func (s *AuthServiceImpl) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, *errcode.Error) {
	// 检查黑名单（fail-closed：Redis 出错时拒绝刷新，防止已登出的 token 被利用）
	blackKey := s.blacklistKey(refreshToken)
	exists, err := s.rdb.Exists(ctx, blackKey).Result()
	if err != nil {
		s.LogInternal("Refresh check blacklist", err)
		return nil, errcode.ErrInternal
	}
	if exists > 0 {
		return nil, errcode.ErrInvalidToken
	}

	claims, parseErr := s.tokens.ParseRefreshToken(refreshToken)
	if parseErr != nil {
		return nil, errcode.ErrInvalidToken
	}

	// 校验账号当前状态：被禁用的用户即使持有有效 refresh token 也不能换取新 token。
	if uid, e := baseservice.ParseUUID(claims.UserID); e == nil {
		user, lookupErr := s.users.GetByID(ctx, uid)
		switch {
		case lookupErr == nil:
			if user.Status == repo.StatusDisabled {
				return nil, errcode.ErrAccountDisabled
			}
		case errors.Is(lookupErr, repo.ErrNotFound):
			return nil, errcode.ErrInvalidToken
		default:
			s.LogInternal("Refresh lookup user", lookupErr, baseservice.UserIDField(claims.UserID))
			return nil, errcode.ErrInternal
		}
	}

	sessionID := claims.SessionID
	if sessionID != "" && s.sessions != nil && !s.sessions.Validate(ctx, claims.UserID, sessionID) {
		return nil, errcode.ErrInvalidToken
	}

	if err := s.rdb.Set(ctx, blackKey, "1", refreshBlacklistTTL).Err(); err != nil {
		s.LogInternal("Refresh blacklist old token", err, baseservice.UserIDField(claims.UserID))
		return nil, errcode.ErrInternal
	}

	pair, issueErr := s.tokens.IssueTokenPair(claims.UserID, claims.Role, sessionID)
	if issueErr != nil {
		s.LogInternal("Refresh issue token pair", issueErr, baseservice.UserIDField(claims.UserID))
		return nil, errcode.ErrInternal
	}

	return pair, nil
}

func (s *AuthServiceImpl) Logout(ctx context.Context, refreshToken string) *errcode.Error {
	if refreshToken == "" {
		return nil
	}

	var logoutErr *errcode.Error

	key := s.blacklistKey(refreshToken)
	claims, parseErr := s.tokens.ParseRefreshToken(refreshToken)

	if err := s.rdb.Set(ctx, key, "1", refreshBlacklistTTL).Err(); err != nil {
		if parseErr == nil {
			s.LogInternal("Logout blacklist refresh token", err, baseservice.UserIDField(claims.UserID))
		} else {
			s.LogInternal("Logout blacklist refresh token", err)
		}
		logoutErr = errcode.ErrInternal
	}

	if parseErr == nil && claims.SessionID != "" && s.sessions != nil {
		if err := s.sessions.Revoke(ctx, claims.UserID, claims.SessionID); err != nil {
			s.LogInternal("Logout revoke session", err, baseservice.UserIDField(claims.UserID))
			logoutErr = errcode.ErrInternal
		}
	}

	return logoutErr
}

// issueResult 颁发 Token 并组装登录结果。
func (s *AuthServiceImpl) issueResult(ctx context.Context, user *repo.User) (*auth.LoginResult, *errcode.Error) {
	if user.Status == repo.StatusDisabled {
		return nil, errcode.ErrAccountDisabled
	}

	sessionID := session.NewSessionID()
	if s.sessions != nil {
		if err := s.sessions.Register(ctx, user.ID.String(), sessionID); err != nil {
			s.LogInternal("issueResult register session", err, baseservice.UserIDField(user.ID.String()))
			return nil, errcode.ErrInternal
		}
	}

	pair, err := s.tokens.IssueTokenPair(user.ID.String(), user.Role, sessionID)
	if err != nil {
		s.LogInternal("issueResult issue token pair", err, baseservice.UserIDField(user.ID.String()))
		return nil, errcode.ErrInternal
	}

	return &auth.LoginResult{
		Tokens: pair,
		User: &auth.UserInfo{
			ID:       user.ID.String(),
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Role:     user.Role,
		},
	}, nil
}

func (s *AuthServiceImpl) blacklistKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%s:blacklist:refresh:%x", s.codePrefix, h)
}
