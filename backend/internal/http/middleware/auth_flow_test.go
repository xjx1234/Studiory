package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/session"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func newAuthTestIssuer() *auth.TokenIssuer {
	return auth.NewTokenIssuer("auth-middleware-test-secret", time.Hour, 24*time.Hour)
}

func newAuthRouter(issuer *auth.TokenIssuer, sessions *session.Store, rdb redis.UniversalClient, keyPrefix string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", Auth(issuer, sessions, rdb, keyPrefix, zap.NewNop()), func(c *gin.Context) {
		uid, _ := c.Get(ContextKeyUserID)
		role, _ := c.Get(ContextKeyUserRole)
		c.JSON(http.StatusOK, gin.H{"user_id": uid, "role": role})
	})
	return r
}

func doAuthRequest(r *gin.Engine, authHeader string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAuth_MissingHeaderReturnsUnauthorized(t *testing.T) {
	r := newAuthRouter(newAuthTestIssuer(), nil, nil, "test")

	w := doAuthRequest(r, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuth_MalformedHeaderReturnsInvalidToken(t *testing.T) {
	r := newAuthRouter(newAuthTestIssuer(), nil, nil, "test")

	cases := []string{"Bearer", "Basic abc123", "abc123", "Bearer  "}
	for _, h := range cases {
		w := doAuthRequest(r, h)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", h, w.Code)
		}
	}
}

func TestAuth_InvalidTokenReturnsUnauthorized(t *testing.T) {
	r := newAuthRouter(newAuthTestIssuer(), nil, nil, "test")

	w := doAuthRequest(r, "Bearer not-a-real-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuth_ValidTokenWithoutRedisOrSessionSucceeds(t *testing.T) {
	issuer := newAuthTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "admin", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair: %v", err)
	}

	r := newAuthRouter(issuer, nil, nil, "test")
	w := doAuthRequest(r, "Bearer "+pair.AccessToken)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestAuth_RevokedAccessTokenReturnsUnauthorized(t *testing.T) {
	issuer := newAuthTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// 全量吊销：把 revoke 时间戳设为未来，任何早于该时刻签发的 token 都视为已吊销。
	future := time.Now().Add(time.Hour).Unix()
	if err := rdb.Set(context.Background(), "test:revoke:uid:user-1", future, 0).Err(); err != nil {
		t.Fatalf("seed revoke key: %v", err)
	}

	r := newAuthRouter(issuer, nil, rdb, "test")
	w := doAuthRequest(r, "Bearer "+pair.AccessToken)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for revoked token", w.Code)
	}
}

func TestAuth_UnsetRevokeKeyDoesNotBlock(t *testing.T) {
	issuer := newAuthTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	// 没有设置任何 revoke key（redis.Nil）：不应阻断请求。

	r := newAuthRouter(issuer, nil, rdb, "test")
	w := doAuthRequest(r, "Bearer "+pair.AccessToken)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when no revoke key set", w.Code)
	}
}

func TestAuth_RedisErrorFailsOpen(t *testing.T) {
	issuer := newAuthTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mr.Close() // 让后续 Redis 操作报错（非 redis.Nil）

	r := newAuthRouter(issuer, nil, rdb, "test")
	w := doAuthRequest(r, "Bearer "+pair.AccessToken)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fail-open) when redis is unavailable", w.Code)
	}
}

func TestAuth_InvalidatedSessionReturnsUnauthorized(t *testing.T) {
	issuer := newAuthTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-old")
	if err != nil {
		t.Fatalf("IssueTokenPair: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sessions := session.NewStore(rdb, "test", false, time.Hour) // 单设备模式

	// 用户在别的设备重新登录，注册了一个新的 session，旧 session 随之失效。
	if err := sessions.Register(context.Background(), "user-1", "session-new"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	r := newAuthRouter(issuer, sessions, rdb, "test")
	w := doAuthRequest(r, "Bearer "+pair.AccessToken)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for invalidated session", w.Code)
	}
}

func TestAuth_ValidSessionSucceeds(t *testing.T) {
	issuer := newAuthTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sessions := session.NewStore(rdb, "test", true, time.Hour) // 多设备模式

	if err := sessions.Register(context.Background(), "user-1", "session-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	r := newAuthRouter(issuer, sessions, rdb, "test")
	w := doAuthRequest(r, "Bearer "+pair.AccessToken)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for valid session, body=%s", w.Code, w.Body.String())
	}
}

// ── isAccessTokenRevoked 直接单测 ──────────────────────────────────────────────

func TestIsAccessTokenRevoked_NilInputsReturnFalse(t *testing.T) {
	ctx := context.Background()
	if isAccessTokenRevoked(ctx, nil, "test", &auth.Claims{}, nil) {
		t.Error("nil rdb should return false")
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if isAccessTokenRevoked(ctx, rdb, "test", nil, nil) {
		t.Error("nil claims should return false")
	}
	if isAccessTokenRevoked(ctx, rdb, "test", &auth.Claims{}, nil) {
		t.Error("claims without IssuedAt should return false")
	}
}

func TestIsAccessTokenRevoked_IssuedBeforeRevokeIsRevoked(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	issuedAt := time.Now().Add(-time.Hour)
	claims := &auth.Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(issuedAt),
		},
	}

	revokeAt := time.Now()
	if err := rdb.Set(ctx, "test:revoke:uid:user-1", revokeAt.Unix(), 0).Err(); err != nil {
		t.Fatalf("seed revoke key: %v", err)
	}

	if !isAccessTokenRevoked(ctx, rdb, "test", claims, zap.NewNop()) {
		t.Error("expected token issued before revoke timestamp to be revoked")
	}
}

func TestIsAccessTokenRevoked_IssuedAfterRevokeIsNotRevoked(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	revokeAt := time.Now().Add(-time.Hour)
	if err := rdb.Set(ctx, "test:revoke:uid:user-1", revokeAt.Unix(), 0).Err(); err != nil {
		t.Fatalf("seed revoke key: %v", err)
	}

	// 用户在吊销之后重新登录拿到的新 token，签发时间晚于 revoke 时间戳，不应被判定为吊销。
	claims := &auth.Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}

	if isAccessTokenRevoked(ctx, rdb, "test", claims, zap.NewNop()) {
		t.Error("expected token issued after revoke timestamp to not be revoked")
	}
}

func TestIsAccessTokenRevoked_RedisErrorFailsOpen(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mr.Close()

	claims := &auth.Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}

	if isAccessTokenRevoked(context.Background(), rdb, "test", claims, zap.NewNop()) {
		t.Error("expected fail-open (not revoked) when redis is unavailable")
	}
}
