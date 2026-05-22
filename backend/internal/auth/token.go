package auth

import (
	"errors"
	"os"
	"sync"
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

// tokenConfig 是运行时可配置的 token 参数，通过 InitToken 注入。
type tokenConfig struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

var (
	globalTokenCfg     tokenConfig
	globalTokenCfgOnce sync.Once
)

// InitToken 在程序启动时（app.New 之前）注入 JWT 配置。
// 若未调用，则退回到从环境变量读取的默认值。
func InitToken(secret string, accessTTL, refreshTTL time.Duration) {
	globalTokenCfgOnce.Do(func() {
		globalTokenCfg = tokenConfig{
			secret:          []byte(secret),
			accessTokenTTL:  accessTTL,
			refreshTokenTTL: refreshTTL,
		}
	})
}

func getCfg() tokenConfig {
	// 如果 InitToken 未被调用，从环境变量读取兜底
	if len(globalTokenCfg.secret) == 0 {
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = "dev-secret-change-in-production"
		}
		return tokenConfig{
			secret:          []byte(secret),
			accessTokenTTL:  2 * time.Hour,
			refreshTokenTTL: 7 * 24 * time.Hour,
		}
	}
	return globalTokenCfg
}

// IssueTokenPair 为指定用户颁发一对 Access + Refresh Token。
func IssueTokenPair(userID, role string) (*TokenPair, error) {
	cfg := getCfg()

	access, err := signToken(userID, role, "access", cfg.accessTokenTTL, cfg.secret)
	if err != nil {
		return nil, err
	}

	refresh, err := signToken(userID, role, "refresh", cfg.refreshTokenTTL, cfg.secret)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(cfg.accessTokenTTL.Seconds()),
	}, nil
}

// ParseAccessToken 解析并校验 Access Token。
func ParseAccessToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, "access", getCfg().secret)
}

// ParseRefreshToken 解析并校验 Refresh Token。
func ParseRefreshToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, "refresh", getCfg().secret)
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
