// Package authservice 实现认证业务：登录（密码/验证码/OAuth）、注册、验证码下发、
// Token 刷新与登出。具体流程按文件拆分：
//
//   - login.go       登录（密码/短信/邮箱验证码/OAuth）
//   - register.go    注册（密码/验证码）
//   - sendcode.go    验证码生成与下发
//   - token.go       Token 刷新、登出、登录成功后颁发 Token
//   - brute_force.go 登录暴力破解防护
//   - helpers.go     通用小工具函数
package authservice

import (
	"context"
	"strings"

	"backend/internal/auth"
	"backend/internal/oauth"
	"backend/internal/repo"
	"backend/internal/sender"
	baseservice "backend/internal/service"
	"backend/internal/session"
	"backend/pkg/errcode"

	"go.uber.org/zap"
)

// Service 定义认证业务入口。
type Service interface {
	Login(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error)
	Register(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error)
	SendCode(ctx context.Context, codeType, target string) *errcode.Error
	Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, *errcode.Error)
	Logout(ctx context.Context, refreshToken string) *errcode.Error
}

// AuthServiceImpl 持有 repo 与 redis，实现业务逻辑。
type AuthServiceImpl struct {
	baseservice.LogSupport

	users                 repo.UserRepo
	oauth                 repo.OAuthRepo
	oauthTx               repo.UserOAuthTxRunner
	tokens                *auth.TokenIssuer
	cache                 CacheStore
	sessions              *session.Store
	codeSender            sender.Sender
	codePrefix            string
	allowMockCodeFallback bool
	oauthVerifier         oauth.Verifier
	oauthDevMode          bool
	oauthProviders        map[string]struct{}
	failOpen              bool // Redis故障时鉴权策略：true=放行（可用性优先）；false=拒绝（安全性优先）
}

type Option func(*AuthServiceImpl)

// New 创建 AuthService，需注入 UserRepo 和 CacheStore。
func New(users repo.UserRepo, cache CacheStore, opts ...Option) Service {
	s := &AuthServiceImpl{
		users:                 users,
		cache:                 cache,
		codePrefix:            "app",
		allowMockCodeFallback: false,
		oauthDevMode:          false,
		oauthProviders:        defaultOAuthProviderSet(),
		failOpen:              true, // 默认 fail-open（可用性优先）
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func defaultOAuthProviderSet() map[string]struct{} {
	set := make(map[string]struct{}, len(DefaultOAuthProviders))
	for _, p := range DefaultOAuthProviders {
		set[p] = struct{}{}
	}
	return set
}

func WithCodePrefix(prefix string) Option {
	return func(s *AuthServiceImpl) {
		if prefix != "" {
			s.codePrefix = prefix
		}
	}
}

func WithMockCodeFallback(enabled bool) Option {
	return func(s *AuthServiceImpl) {
		s.allowMockCodeFallback = enabled
	}
}

// WithCodeSender 注入验证码下发器（多服务商路由）。
// 未注入时 SendCode 仅写入 Redis 而不真正下发（兼容无下发渠道的本地调试）。
func WithCodeSender(snd sender.Sender) Option {
	return func(s *AuthServiceImpl) {
		s.codeSender = snd
	}
}

func WithOAuthRepo(oauth repo.OAuthRepo) Option {
	return func(s *AuthServiceImpl) {
		s.oauth = oauth
	}
}

func WithUserOAuthTxRunner(runner repo.UserOAuthTxRunner) Option {
	return func(s *AuthServiceImpl) {
		s.oauthTx = runner
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(s *AuthServiceImpl) {
		s.SetLogger(logger)
	}
}

func WithTokenIssuer(issuer *auth.TokenIssuer) Option {
	return func(s *AuthServiceImpl) {
		s.tokens = issuer
	}
}

// WithSessionStore 注入会话存储（多/单设备 session 管理）。
func WithSessionStore(store *session.Store) Option {
	return func(s *AuthServiceImpl) {
		s.sessions = store
	}
}

func WithOAuthDevMode(enabled bool) Option {
	return func(s *AuthServiceImpl) {
		s.oauthDevMode = enabled
	}
}

func WithOAuthProviders(providers []string) Option {
	return func(s *AuthServiceImpl) {
		if len(providers) == 0 {
			return
		}
		set := make(map[string]struct{}, len(providers))
		for _, p := range providers {
			p = strings.ToLower(strings.TrimSpace(p))
			if p != "" {
				set[p] = struct{}{}
			}
		}
		s.oauthProviders = set
	}
}

// WithOAuthVerifier 注入第三方登录 token 校验器（多平台路由）。
func WithOAuthVerifier(v oauth.Verifier) Option {
	return func(s *AuthServiceImpl) {
		s.oauthVerifier = v
	}
}

// WithRedisFailOpen 设置 Redis 故障时的鉴权降级策略。
// true=放行（可用性优先，默认）；false=拒绝（安全性优先）。
func WithRedisFailOpen(failOpen bool) Option {
	return func(s *AuthServiceImpl) {
		s.failOpen = failOpen
	}
}
