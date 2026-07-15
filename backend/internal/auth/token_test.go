package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestIssuer() *TokenIssuer {
	return NewTokenIssuer("test-secret-key-for-unit-tests", time.Hour, 24*time.Hour)
}

func TestIssueTokenPair_ReturnsValidPair(t *testing.T) {
	issuer := newTestIssuer()

	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty access/refresh tokens")
	}
	if pair.AccessToken == pair.RefreshToken {
		t.Fatal("access and refresh tokens should differ")
	}
	if pair.ExpiresIn != int64(time.Hour.Seconds()) {
		t.Fatalf("ExpiresIn = %d, want %d", pair.ExpiresIn, int64(time.Hour.Seconds()))
	}
}

func TestTokenIssuer_TTLAccessors(t *testing.T) {
	issuer := NewTokenIssuer("secret", 30*time.Minute, 48*time.Hour)
	if issuer.AccessTokenTTL() != 30*time.Minute {
		t.Fatalf("AccessTokenTTL() = %v, want 30m", issuer.AccessTokenTTL())
	}
	if issuer.RefreshTokenTTL() != 48*time.Hour {
		t.Fatalf("RefreshTokenTTL() = %v, want 48h", issuer.RefreshTokenTTL())
	}
}

func TestParseAccessToken_ValidToken(t *testing.T) {
	issuer := newTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "admin", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}

	claims, err := issuer.ParseAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken failed: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-1")
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q, want %q", claims.Role, "admin")
	}
	if claims.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", claims.SessionID, "session-1")
	}
}

func TestParseRefreshToken_ValidToken(t *testing.T) {
	issuer := newTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}

	claims, err := issuer.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ParseRefreshToken failed: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-1")
	}
}

func TestParseAccessToken_RejectsRefreshToken(t *testing.T) {
	issuer := newTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}

	if _, err := issuer.ParseAccessToken(pair.RefreshToken); err == nil {
		t.Fatal("expected error when parsing refresh token as access token")
	}
}

func TestParseRefreshToken_RejectsAccessToken(t *testing.T) {
	issuer := newTestIssuer()
	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}

	if _, err := issuer.ParseRefreshToken(pair.AccessToken); err == nil {
		t.Fatal("expected error when parsing access token as refresh token")
	}
}

func TestParseAccessToken_RejectsWrongSecret(t *testing.T) {
	issuer := newTestIssuer()
	other := NewTokenIssuer("a-completely-different-secret", time.Hour, time.Hour)

	pair, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}

	if _, err := other.ParseAccessToken(pair.AccessToken); err == nil {
		t.Fatal("expected error when parsing token signed with a different secret")
	}
}

func TestParseAccessToken_RejectsMalformedToken(t *testing.T) {
	issuer := newTestIssuer()

	cases := []string{"", "not-a-token", "a.b.c", "a.b.c.d"}
	for _, tokenStr := range cases {
		if _, err := issuer.ParseAccessToken(tokenStr); err == nil {
			t.Errorf("expected error for malformed token %q", tokenStr)
		}
	}
}

func TestParseAccessToken_RejectsExpiredToken(t *testing.T) {
	issuer := newTestIssuer()

	expired, err := signToken("user-1", "user", "session-1", "access", -time.Hour, issuer.secret)
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	_, err = issuer.ParseAccessToken(expired)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Errorf("expected jwt.ErrTokenExpired, got: %v", err)
	}
}

// TestParseAccessToken_RejectsNoneAlgorithm 防止“alg=none”降级攻击：
// 即便攻击者构造了未签名 token，也必须被拒绝。
func TestParseAccessToken_RejectsNoneAlgorithm(t *testing.T) {
	issuer := newTestIssuer()

	claims := &Claims{
		UserID: "user-1",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{"access"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to build none-alg token: %v", err)
	}

	if _, err := issuer.ParseAccessToken(tokenStr); err == nil {
		t.Fatal("expected error for none-algorithm token")
	}
}

func TestIssueTokenPair_DifferentSessionsProduceDifferentTokens(t *testing.T) {
	issuer := newTestIssuer()

	pair1, err := issuer.IssueTokenPair("user-1", "user", "session-1")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}
	pair2, err := issuer.IssueTokenPair("user-1", "user", "session-2")
	if err != nil {
		t.Fatalf("IssueTokenPair failed: %v", err)
	}

	if pair1.AccessToken == pair2.AccessToken {
		t.Fatal("tokens for different sessions should differ")
	}

	claims1, err := issuer.ParseAccessToken(pair1.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken failed: %v", err)
	}
	claims2, err := issuer.ParseAccessToken(pair2.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken failed: %v", err)
	}
	if claims1.SessionID != "session-1" || claims2.SessionID != "session-2" {
		t.Fatalf("unexpected session ids: %q, %q", claims1.SessionID, claims2.SessionID)
	}
}
