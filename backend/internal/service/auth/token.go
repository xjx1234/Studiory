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
	// 先解析 token，格式不合法直接拒绝（不占用黑名单 key）
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

	// 原子化黑名单标记（fail-closed）：SET NX 将「检查+写入」合并为一步，
	// 消除 TOCTOU 竞态——同一 refresh token 并发刷新时仅一个请求能成功，
	// 其余视为重放攻击直接拒绝。
	blackKey := s.blacklistKey(refreshToken)
	ok, err := s.cache.SetNX(ctx, blackKey, "1", refreshBlacklistTTL)
	if err != nil {
		s.LogInternal("Refresh blacklist old token", err, baseservice.UserIDField(claims.UserID))
		return nil, errcode.ErrInternal
	}
	if !ok {
		return nil, errcode.ErrInvalidToken
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

	if err := s.cache.Set(ctx, key, "1", refreshBlacklistTTL); err != nil {
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
