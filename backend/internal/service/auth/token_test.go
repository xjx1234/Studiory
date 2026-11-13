package authservice

import (
	"context"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
)

const ttlTestSecret = "test-secret-key-for-unit-tests"

// tokenTestSvc 创建带 miniredis 的 AuthService 并预置一个活跃用户。
// 返回 service、预置用户和该用户的 refresh token。
func tokenTestSvc(t *testing.T) (*AuthServiceImpl, *repo.User, string) {
	t.Helper()
	_, rdb := newTestRDB(t)
	users := testutil.NewFakeUserRepo()

	issuer := testTokenIssuer()
	svc, ok := New(users, NewRedisCacheStore(rdb),
		WithTokenIssuer(issuer),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	uid := uuid.New()
	phone := "13700000001"
	user := &repo.User{
		ID:       uid,
		Phone:    &phone,
		Nickname: "token-test-user",
		Role:     repo.RoleUser,
		Status:   repo.StatusActive,
	}
	users.Users[uid] = user

	pair, err := issuer.IssueTokenPair(uid.String(), repo.RoleUser, "sess-1")
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	return svc, user, pair.RefreshToken
}

// TestRefresh_Success 验证合法 refresh token 能换取新 token pair。
func TestRefresh_Success(t *testing.T) {
	svc, user, refreshToken := tokenTestSvc(t)

	pair, e := svc.Refresh(context.Background(), refreshToken)
	if e != nil {
		t.Fatalf("refresh failed: %+v", e)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty token pair")
	}
	// 验证新 access token 可正常解析且携带正确的 userID
	claims, parseErr := svc.tokens.ParseAccessToken(pair.AccessToken)
	if parseErr != nil {
		t.Fatalf("parse new access token: %v", parseErr)
	}
	if claims.UserID != user.ID.String() {
		t.Errorf("new token userID = %q, want %q", claims.UserID, user.ID.String())
	}
}

// TestRefresh_InvalidToken 验证无效 token 返回 ErrInvalidToken。
func TestRefresh_InvalidToken(t *testing.T) {
	svc, _, _ := tokenTestSvc(t)

	_, e := svc.Refresh(context.Background(), "not-a-valid-jwt")
	if e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %+v", e)
	}
}

// TestRefresh_DisabledUser 验证被禁用用户的 refresh 被拒绝。
func TestRefresh_DisabledUser(t *testing.T) {
	svc, user, refreshToken := tokenTestSvc(t)

	// 禁用用户
	user.Status = repo.StatusDisabled

	_, e := svc.Refresh(context.Background(), refreshToken)
	if e != errcode.ErrAccountDisabled {
		t.Fatalf("expected ErrAccountDisabled, got %+v", e)
	}
}

// TestRefresh_ReplayRejected 验证同一 refresh token 二次刷新被 SetNX 拒绝。
func TestRefresh_ReplayRejected(t *testing.T) {
	svc, _, refreshToken := tokenTestSvc(t)

	// 第一次刷新成功
	pair1, e := svc.Refresh(context.Background(), refreshToken)
	if e != nil {
		t.Fatalf("first refresh failed: %+v", e)
	}
	if pair1 == nil {
		t.Fatal("expected token pair")
	}

	// 同一 token 再次刷新应被拒绝（SetNX 原子黑名单）
	_, e2 := svc.Refresh(context.Background(), refreshToken)
	if e2 != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken on replay, got %+v", e2)
	}
}

// TestLogout_Success 验证登出后该 refresh token 无法再刷新。
func TestLogout_Success(t *testing.T) {
	svc, _, refreshToken := tokenTestSvc(t)

	if e := svc.Logout(context.Background(), refreshToken); e != nil {
		t.Fatalf("logout failed: %+v", e)
	}

	// 登出后 refresh 应被拒绝
	_, e := svc.Refresh(context.Background(), refreshToken)
	if e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken after logout, got %+v", e)
	}
}

// TestLogout_EmptyToken 验证空 token 登出是无操作。
func TestLogout_EmptyToken(t *testing.T) {
	svc, _, _ := tokenTestSvc(t)

	if e := svc.Logout(context.Background(), ""); e != nil {
		t.Fatalf("logout with empty token should be no-op, got %+v", e)
	}
}

// TestRefresh_UserNotFound 验证用户已被删除后 refresh 被拒绝。
func TestRefresh_UserNotFound(t *testing.T) {
	svc, user, refreshToken := tokenTestSvc(t)

	// 从 repo 中删除用户（模拟用户被删除）
	delete(svc.users.(*testutil.FakeUserRepo).Users, user.ID)

	_, e := svc.Refresh(context.Background(), refreshToken)
	if e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for deleted user, got %+v", e)
	}
}

// TestRefresh_PreservesSessionID 验证刷新后的 token 携带原 sessionID（用于会话延续）。
func TestRefresh_PreservesSessionID(t *testing.T) {
	_, rdb := newTestRDB(t)
	users := testutil.NewFakeUserRepo()
	issuer := testTokenIssuer()

	svc, ok := New(users, NewRedisCacheStore(rdb),
		WithTokenIssuer(issuer),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	uid := uuid.New()
	users.Users[uid] = &repo.User{
		ID:     uid,
		Role:   repo.RoleUser,
		Status: repo.StatusActive,
	}

	originalSessionID := "my-session-123"
	pair, err := issuer.IssueTokenPair(uid.String(), repo.RoleUser, originalSessionID)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	newPair, e := svc.Refresh(context.Background(), pair.RefreshToken)
	if e != nil {
		t.Fatalf("refresh failed: %+v", e)
	}

	// 解析新 refresh token，验证 sessionID 被保留
	claims, parseErr := issuer.ParseRefreshToken(newPair.RefreshToken)
	if parseErr != nil {
		t.Fatalf("parse new refresh token: %v", parseErr)
	}
	if claims.SessionID != originalSessionID {
		t.Errorf("sessionID = %q, want %q", claims.SessionID, originalSessionID)
	}
}

// ── 黑名单 TTL：跟随 token 自身寿命，而非硬编码 7 天 ────────────────────────────

// tokenSvcWithIssuers 装配「service 用 svcIssuer 校验、票由 tokenIssuer 签发」的组合。
// 两个签发器共用同一密钥才能互认——这正是「TTL 跟着票的 exp 走」与「跟着 jwt.refresh_ttl 走」
// 可以被区分开的原因。
func tokenSvcWithIssuers(t *testing.T, svcIssuer, tokenIssuer *auth.TokenIssuer) (*AuthServiceImpl, *miniredis.Miniredis, string) {
	t.Helper()
	mr, rdb := newTestRDB(t)
	users := testutil.NewFakeUserRepo()

	svc, ok := New(users, NewRedisCacheStore(rdb), WithTokenIssuer(svcIssuer)).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	uid := uuid.New()
	phone := "13700000009"
	users.Users[uid] = &repo.User{
		ID:       uid,
		Phone:    &phone,
		Nickname: "ttl-test-user",
		Role:     repo.RoleUser,
		Status:   repo.StatusActive,
	}

	pair, err := tokenIssuer.IssueTokenPair(uid.String(), repo.RoleUser, "sess-ttl")
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	return svc, mr, pair.RefreshToken
}

// TestRefresh_BlacklistTTLFollowsTokenExpiry 验证刷新写入的黑名单 key 只保留到这张票自然过期。
// 配置里的 jwt.refresh_ttl 是 1 小时，但票本身只有 12 分钟——TTL 应该跟票，不跟配置，
// 更不是修复前那个硬编码的 7 天。
func TestRefresh_BlacklistTTLFollowsTokenExpiry(t *testing.T) {
	svc, mr, refreshToken := tokenSvcWithIssuers(t,
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, time.Hour),
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, 12*time.Minute),
	)

	if _, e := svc.Refresh(context.Background(), refreshToken); e != nil {
		t.Fatalf("refresh failed: %+v", e)
	}

	ttl := mr.TTL(svc.blacklistKey(refreshToken))
	switch {
	case ttl <= 0:
		t.Fatal("expected a positive blacklist TTL (miniredis 里 0 表示没设过期/永不回收)")
	case ttl > 12*time.Minute:
		t.Errorf("blacklist TTL = %v, want <= 票的剩余寿命 12m（修复前是硬编码 7 天）", ttl)
	case ttl < 10*time.Minute:
		t.Errorf("blacklist TTL = %v, 比票的实际寿命短太多，防重放窗口会提前失效", ttl)
	}
}

// TestRefresh_BlacklistTTLCappedByConfiguredRefreshTTL 验证异常长的票不会让黑名单超过配置上限。
func TestRefresh_BlacklistTTLCappedByConfiguredRefreshTTL(t *testing.T) {
	svc, mr, refreshToken := tokenSvcWithIssuers(t,
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, 5*time.Minute),
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, time.Hour),
	)

	if _, e := svc.Refresh(context.Background(), refreshToken); e != nil {
		t.Fatalf("refresh failed: %+v", e)
	}

	if ttl := mr.TTL(svc.blacklistKey(refreshToken)); ttl <= 0 || ttl > 5*time.Minute {
		t.Errorf("blacklist TTL = %v, want (0, 5m]（不得超过配置里的 jwt.refresh_ttl）", ttl)
	}
}

// TestRefresh_ExpiredTokenLeavesNoBlacklistKey 验证过期票被拒绝时不在 Redis 留下任何 key。
func TestRefresh_ExpiredTokenLeavesNoBlacklistKey(t *testing.T) {
	svc, mr, refreshToken := tokenSvcWithIssuers(t,
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, time.Hour),
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, -time.Minute), // 签发即过期
	)

	if _, e := svc.Refresh(context.Background(), refreshToken); e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for expired token, got %+v", e)
	}
	if keys := mr.Keys(); len(keys) != 0 {
		t.Errorf("expired refresh left %d redis keys: %v", len(keys), keys)
	}
}

// TestLogout_BlacklistTTLFollowsTokenExpiry 验证登出的黑名单 TTL 同样跟着票的剩余寿命。
func TestLogout_BlacklistTTLFollowsTokenExpiry(t *testing.T) {
	svc, mr, refreshToken := tokenSvcWithIssuers(t,
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, time.Hour),
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, 20*time.Minute),
	)

	if e := svc.Logout(context.Background(), refreshToken); e != nil {
		t.Fatalf("logout failed: %+v", e)
	}
	if ttl := mr.TTL(svc.blacklistKey(refreshToken)); ttl <= 0 || ttl > 20*time.Minute {
		t.Errorf("logout blacklist TTL = %v, want (0, 20m]", ttl)
	}
}

// TestLogout_UnparseableTokenWritesNoBlacklistKey 验证乱码 token 登出是幂等空操作。
// 修复前它照样会按 sha256(token) 写一个 7 天 TTL 的 key，等于给攻击者一个免费的 Redis 内存放大器。
func TestLogout_UnparseableTokenWritesNoBlacklistKey(t *testing.T) {
	svc, mr, _ := tokenSvcWithIssuers(t,
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, time.Hour),
		auth.NewTokenIssuer(ttlTestSecret, time.Hour, time.Hour),
	)
	before := len(mr.Keys())

	for _, junk := range []string{"not-a-jwt", "a.b.c", "random garbage"} {
		if e := svc.Logout(context.Background(), junk); e != nil {
			t.Fatalf("logout(%q) = %+v, want nil", junk, e)
		}
	}
	if after := len(mr.Keys()); after != before {
		t.Errorf("junk logout created %d redis keys (before=%d, after=%d)", after-before, before, after)
	}
}
