package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 JWT 的自定义载荷。
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// TokenPair 包含 Access Token 和 Refresh Token。
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // Access Token 有效秒数
}

// TokenIssuer 负责 JWT 签发与解析，通过依赖注入传入各层。
type TokenIssuer struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewTokenIssuer 创建 TokenIssuer。
func NewTokenIssuer(secret string, accessTTL, refreshTTL time.Duration) *TokenIssuer {
	return &TokenIssuer{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

// AccessTokenTTL 返回 Access Token 有效期。
func (t *TokenIssuer) AccessTokenTTL() time.Duration {
	return t.accessTokenTTL
}

// IssueTokenPair 为指定用户颁发一对 Access + Refresh Token。
func (t *TokenIssuer) IssueTokenPair(userID, role string) (*TokenPair, error) {
	access, err := signToken(userID, role, "access", t.accessTokenTTL, t.secret)
	if err != nil {
		return nil, err
	}

	refresh, err := signToken(userID, role, "refresh", t.refreshTokenTTL, t.secret)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(t.accessTokenTTL.Seconds()),
	}, nil
}

// ParseAccessToken 解析并校验 Access Token。
func (t *TokenIssuer) ParseAccessToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, "access", t.secret)
}

// ParseRefreshToken 解析并校验 Refresh Token。
func (t *TokenIssuer) ParseRefreshToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, "refresh", t.secret)
}

func signToken(userID, role, tokenType string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Audience:  jwt.ClaimStrings{tokenType},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func parseToken(tokenStr, expectedType string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非预期的签名算法")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token 无效")
	}

	for _, aud := range claims.Audience {
		if aud == expectedType {
			return claims, nil
		}
	}
	return nil, errors.New("token 类型不匹配")
}
