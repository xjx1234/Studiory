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

// blacklistTTL 计算「这条 refresh token 还需要在黑名单里被记住多久」。
//
// 黑名单唯一的作用是防重放：token 一旦自然过期，JWT 解析就会拒绝它，
// 继续占着 Redis key 没有任何防护价值。所以 TTL 必须跟着 token 自身的剩余寿命走，
// 而不是修复前那个硬编码的 7 天——它既不跟 jwt.refresh_ttl 联动（配置调小就白占内存，
// 将来签发超长票时又会在到期前悄悄失去防重放能力），也没法用 ttl<=0 表达「不必写」。
//
// 返回 ok=false 表示这条 token 已经过期（或没有 exp），无需也不能写入黑名单：
// go-redis 会把 ttl<=0 解释成「永不过期」，那才是真正会泄漏 key 的做法。
func (s *AuthServiceImpl) blacklistTTL(claims *auth.Claims) (time.Duration, bool) {
	if claims == nil {
		return 0, false
	}

	// 兜底上限：配置里的 refresh TTL。正常签发的 token 剩余寿命必然 <= 它，
	// 只有异常签发（手工造的超长票）才会命中这条上限。
	limit := s.tokens.RefreshTokenTTL()

	if claims.ExpiresAt == nil {
		// 本项目签发路径一定带 exp；没有 exp 的票按配置上限保守处理，避免永不过期的垃圾 key。
		return limit, limit > 0
	}

	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining <= 0 {
		return 0, false
	}
	if limit > 0 && remaining > limit {
		return limit, true
	}
	return remaining, true
}

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
	ttl, needBlacklist := s.blacklistTTL(claims)
	if !needBlacklist {
		// 到不了这里：ParseRefreshToken 已拒掉过期票。真命中说明票已失效，
		// 直接拒绝即可，不能带着 ttl<=0 去写黑名单。
		return nil, errcode.ErrInvalidToken
	}
	ok, err := s.cache.SetNX(ctx, blackKey, "1", ttl)
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

// Logout 吊销调用方提供的 refresh token。
//
// 解析不出来的 token（伪造、过期、格式错误）是幂等空操作：它本来就换不到新票，
// 修复前却照样会按 sha256(token) 往 Redis 写一个黑名单 key——攻击者随便发一串垃圾
// 就能在 /auth/logout 上无限堆 key（TTL 又硬编码成 7 天），等于一个免费的 Redis 内存放大器。
func (s *AuthServiceImpl) Logout(ctx context.Context, refreshToken string) *errcode.Error {
	if refreshToken == "" {
		return nil
	}

	var logoutErr *errcode.Error

	claims, parseErr := s.tokens.ParseRefreshToken(refreshToken)
	if parseErr != nil {
		return nil
	}

	if ttl, ok := s.blacklistTTL(claims); ok {
		key := s.blacklistKey(refreshToken)
		if err := s.cache.Set(ctx, key, "1", ttl); err != nil {
			s.LogInternal("Logout blacklist refresh token", err, baseservice.UserIDField(claims.UserID))
			logoutErr = errcode.ErrInternal
		}
	}

	if claims.SessionID != "" && s.sessions != nil {
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
