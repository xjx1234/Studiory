package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 JWT 的自定义载荷。
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"` // user / admin
	jwt.RegisteredClaims
}

// TokenPair 包含 Access Token 和 Refresh Token。
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // Access Token 有效秒数
}

var (
	// jwtSecret 从环境变量读取，生产环境务必设置。
	jwtSecret = func() []byte {
		if s := os.Getenv("JWT_SECRET"); s != "" {
			return []byte(s)
		}
		return []byte("dev-secret-change-in-production")
	}()

	accessTokenTTL  = 2 * time.Hour
	refreshTokenTTL = 7 * 24 * time.Hour
)

// IssueTokenPair 为指定用户颁发一对 Access + Refresh Token。
func IssueTokenPair(userID, role string) (*TokenPair, error) {
	access, err := signToken(userID, role, "access", accessTokenTTL)
	if err != nil {
		return nil, err
	}

	refresh, err := signToken(userID, role, "refresh", refreshTokenTTL)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(accessTokenTTL.Seconds()),
	}, nil
}

// ParseAccessToken 解析并校验 Access Token，返回 Claims。
func ParseAccessToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, "access")
}

// ParseRefreshToken 解析并校验 Refresh Token，返回 Claims。
func ParseRefreshToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, "refresh")
}

func signToken(userID, role, tokenType string, ttl time.Duration) (string, error) {
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
	return token.SignedString(jwtSecret)
}

func parseToken(tokenStr, expectedType string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非预期的签名算法")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token 无效")
	}

	// 校验 token 类型（防止把 refresh token 当 access token 用）
	for _, aud := range claims.Audience {
		if aud == expectedType {
			return claims, nil
		}
	}
	return nil, errors.New("token 类型不匹配")
}
