package authservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/sender"
	baseservice "backend/internal/service"
	"backend/pkg/errcode"
	"backend/pkg/strutil"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const (
	codeExpiry          = 5 * time.Minute
	codeSendCooldown    = 60 * time.Second
	refreshBlacklistTTL = 7 * 24 * time.Hour
	bcryptCost          = bcrypt.DefaultCost

	// 账号密码暴力破解防护参数。
	// 同一账号在 loginFailWindow 内失败 loginMaxFailAttempts 次后
	// 锁定 loginLockDuration，期间拒绝任何密码登录尝试。
	loginMaxFailAttempts = 5
	loginFailWindow      = 10 * time.Minute
	loginLockDuration    = 15 * time.Minute

	// verifyCodeLua 原子校验验证码：匹配则删除并返回 1；key 不存在返回 -1；不匹配返回 0。
	verifyCodeLua = `
local val = redis.call('GET', KEYS[1])
if not val then
    return -1
end
if val == ARGV[1] then
    redis.call('DEL', KEYS[1])
    return 1
end
return 0
`

	// loginRecordFailLua 原子递增失败计数，首次写入时设置 TTL；达到阈值时写入锁定 key。
	// KEYS[1]=fail_key  KEYS[2]=lock_key
	// ARGV[1]=fail_window_sec  ARGV[2]=max_attempts  ARGV[3]=lock_ttl_sec
	loginRecordFailLua = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
end
if count >= tonumber(ARGV[2]) then
    redis.call('SET', KEYS[2], '1', 'EX', tonumber(ARGV[3]))
end
return count
`
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
	rdb                   redis.UniversalClient
	codeSender            sender.Sender
	codePrefix            string
	allowMockCodeFallback bool
	oauthDevMode          bool
	oauthProviders        map[string]struct{}
}

type Option func(*AuthServiceImpl)

// New 创建 AuthService，需注入 UserRepo 和 Redis client。
func New(users repo.UserRepo, rdb redis.UniversalClient, opts ...Option) Service {
	s := &AuthServiceImpl{
		users:                 users,
		rdb:                   rdb,
		codePrefix:            "app",
		allowMockCodeFallback: true,
		oauthDevMode:          false,
		oauthProviders:        defaultOAuthProviderSet(),
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

// ── Login ─────────────────────────────────────────────────────────────────────

func (s *AuthServiceImpl) Login(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
	switch req.GrantType {
	case auth.GrantTypePassword:
		return s.loginWithPassword(ctx, req)
	case auth.GrantTypeSMSCode:
		return s.loginWithCode(ctx, "sms", req.Phone, req.Code)
	case auth.GrantTypeEmailCode:
		return s.loginWithCode(ctx, "email", req.Email, req.Code)
	case auth.GrantTypeOAuth:
		return s.loginWithOAuth(ctx, req)
	default:
		return nil, errcode.ErrUnsupportedGrant
	}
}

func (s *AuthServiceImpl) loginWithPassword(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
	account := loginAccountID(req.Phone, req.Email, req.Account)

	// 检查账号是否处于锁定状态
	if s.isLoginLocked(ctx, account) {
		return nil, errcode.ErrAccountLocked
	}

	user, svcErr := s.findUserByAccount(ctx, req.Phone, req.Email, req.Account)
	if svcErr != nil {
		// 用户不存在也记为失败，防止通过响应时间枚举账号
		if svcErr.Code == errcode.ErrInvalidCredentials.Code {
			s.recordLoginFail(ctx, account)
		}
		return nil, svcErr
	}

	if user.PasswordHash == nil || *user.PasswordHash == "" {
		s.recordLoginFail(ctx, account)
		return nil, errcode.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		s.recordLoginFail(ctx, account)
		return nil, errcode.ErrInvalidCredentials
	}

	// 登录成功，清除失败计数
	s.clearLoginFail(ctx, account)
	return s.issueResult(user)
}

func (s *AuthServiceImpl) loginWithCode(ctx context.Context, codeType, target, code string) (*auth.LoginResult, *errcode.Error) {
	if target == "" || code == "" {
		return nil, errcode.ErrBadRequest
	}

	if !s.verifyCode(ctx, codeType, target, code) {
		return nil, errcode.ErrInvalidCode
	}

	var (
		user *repo.User
		err  error
	)
	if codeType == "sms" {
		user, err = s.users.GetByPhone(ctx, target)
	} else {
		user, err = s.users.GetByEmail(ctx, target)
	}
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound.WithMessage("err_user_not_found")
		}
		s.LogInternal("loginWithCode lookup user", err)
		return nil, errcode.ErrInternal
	}

	return s.issueResult(user)
}

func (s *AuthServiceImpl) loginWithOAuth(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
	if s.oauth == nil {
		return nil, errcode.ErrInternal
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	openID := strings.TrimSpace(req.OpenID)
	if provider == "" || openID == "" {
		return nil, errcode.ErrBadRequest
	}
	if !s.isAllowedOAuthProvider(provider) {
		return nil, errcode.ErrUnsupportedGrant
	}
	if !s.oauthDevMode {
		// 生产环境应在此校验第三方 access_token，并解析出 open_id。
		return nil, errcode.ErrUnsupportedGrant
	}

	user, err := s.oauth.GetUserByOAuth(ctx, provider, openID)
	if err == nil {
		return s.issueResult(user)
	}
	if !errors.Is(err, repo.ErrNotFound) {
		s.LogInternal("loginWithOAuth lookup user", err)
		return nil, errcode.ErrInternal
	}

	nickname := oauthDefaultNickname(provider, openID)
	created, createErr := s.createOAuthUser(ctx, nickname, provider, openID)
	if createErr != nil {
		s.LogInternal("loginWithOAuth create user", createErr)
		return nil, errcode.ErrInternal
	}

	return s.issueResult(created)
}

func (s *AuthServiceImpl) createOAuthUser(ctx context.Context, nickname, provider, openID string) (*repo.User, error) {
	createAndBind := func(users repo.UserRepo, oauthRepo repo.OAuthRepo) (*repo.User, error) {
		created, err := users.Create(ctx, &repo.CreateUserParams{
			Nickname: nickname,
			Role:     repo.RoleUser,
		})
		if err != nil {
			return nil, err
		}
		if _, err := oauthRepo.CreateOAuth(ctx, created.ID, provider, openID); err != nil {
			return nil, err
		}
		return created, nil
	}

	if s.oauthTx != nil {
		var created *repo.User
		txErr := s.oauthTx.WithUserOAuthTx(ctx, func(users repo.UserRepo, oauthRepo repo.OAuthRepo) error {
			u, err := createAndBind(users, oauthRepo)
			if err != nil {
				return err
			}
			created = u
			return nil
		})
		if txErr == nil {
			return created, nil
		}
		// 并发注册时可能已被其他请求绑定，回查一次。
		if !errors.Is(txErr, repo.ErrAlreadyExists) {
			return nil, txErr
		}
		existing, lookupErr := s.oauth.GetUserByOAuth(ctx, provider, openID)
		if lookupErr != nil {
			return nil, txErr
		}
		return existing, nil
	}

	created, createErr := createAndBind(s.users, s.oauth)
	if createErr == nil {
		return created, nil
	}
	if !errors.Is(createErr, repo.ErrAlreadyExists) {
		return nil, createErr
	}
	existing, lookupErr := s.oauth.GetUserByOAuth(ctx, provider, openID)
	if lookupErr != nil {
		return nil, createErr
	}
	return existing, nil
}

func (s *AuthServiceImpl) isAllowedOAuthProvider(provider string) bool {
	_, ok := s.oauthProviders[provider]
	return ok
}

func oauthDefaultNickname(provider, openID string) string {
	suffix := openID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return fmt.Sprintf("%s_%s", provider, suffix)
}

// ── Register ──────────────────────────────────────────────────────────────────

func (s *AuthServiceImpl) Register(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error) {
	switch input.GrantType {
	case "password":
		return s.registerWithPassword(ctx, input)
	case "code":
		return s.registerWithCode(ctx, input)
	default:
		return nil, errcode.ErrUnsupportedGrant
	}
}

func (s *AuthServiceImpl) registerWithPassword(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error) {
	if input.Password == "" {
		return nil, errcode.ErrBadRequest
	}

	// 检查账号是否已存在
	if err := s.checkAccountExists(ctx, input.Phone, input.Email); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
	if err != nil {
		s.LogInternal("registerWithPassword hash password", err)
		return nil, errcode.ErrInternal
	}

	hashStr := string(hash)
	nickname := s.defaultNickname(input)
	user, createErr := s.users.Create(ctx, &repo.CreateUserParams{
		Phone:        strutil.NullableStr(input.Phone),
		Email:        strutil.NullableStr(input.Email),
		PasswordHash: &hashStr,
		Nickname:     nickname,
		Role:         repo.RoleUser,
	})
	if createErr != nil {
		if errors.Is(createErr, repo.ErrAlreadyExists) {
			return nil, errcode.ErrAlreadyExists
		}
		s.LogInternal("registerWithPassword create user", createErr)
		return nil, errcode.ErrInternal
	}

	return s.issueResult(user)
}

func (s *AuthServiceImpl) registerWithCode(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error) {
	if input.Code == "" || input.CodeType == "" {
		return nil, errcode.ErrBadRequest
	}

	target := input.Phone
	if input.CodeType == "email" {
		target = input.Email
	}

	if !s.verifyCode(ctx, input.CodeType, target, input.Code) {
		return nil, errcode.ErrInvalidCode
	}

	if err := s.checkAccountExists(ctx, input.Phone, input.Email); err != nil {
		return nil, err
	}

	nickname := s.defaultNickname(input)
	user, createErr := s.users.Create(ctx, &repo.CreateUserParams{
		Phone:    strutil.NullableStr(input.Phone),
		Email:    strutil.NullableStr(input.Email),
		Nickname: nickname,
		Role:     repo.RoleUser,
	})
	if createErr != nil {
		if errors.Is(createErr, repo.ErrAlreadyExists) {
			return nil, errcode.ErrAlreadyExists
		}
		s.LogInternal("registerWithCode create user", createErr)
		return nil, errcode.ErrInternal
	}

	return s.issueResult(user)
}

// ── SendCode ──────────────────────────────────────────────────────────────────

// SendCode 生成验证码、写入 Redis，并通过下发器（多服务商路由）发送。
//
// 流程：冷却限频 → 生成验证码 → 写 Redis → 经 codeSender 下发。
// 下发失败时清除冷却键，允许用户立即重试。
func (s *AuthServiceImpl) SendCode(ctx context.Context, codeType, target string) *errcode.Error {
	if codeType == "" || target == "" {
		return errcode.ErrBadRequest
	}

	cooldownKey := s.codeCooldownKey(codeType, target)
	ok, err := s.rdb.SetNX(ctx, cooldownKey, "1", codeSendCooldown).Result()
	if err != nil {
		s.LogInternal("SendCode set cooldown", err)
		return errcode.ErrInternal
	}
	if !ok {
		return errcode.ErrTooManyRequests
	}

	code := s.generateCode()

	key := s.codeRedisKey(codeType, target)
	if err := s.rdb.Set(ctx, key, code, codeExpiry).Err(); err != nil {
		s.LogInternal("SendCode set code", err)
		_ = s.rdb.Del(ctx, cooldownKey).Err()
		return errcode.ErrInternal
	}

	if s.codeSender != nil {
		if err := s.codeSender.Send(ctx, sender.Message{
			Channel: sender.Channel(codeType),
			Target:  target,
			Code:    code,
		}); err != nil {
			s.LogInternal("SendCode dispatch", err)
			// 下发失败：回滚冷却与验证码，便于用户重试
			_ = s.rdb.Del(ctx, cooldownKey, key).Err()
			return errcode.ErrInternal
		}
	}

	return nil
}

// generateCode 生成验证码。
// 开发模式（allowMockCodeFallback）下返回固定的 MockVerificationCode 便于联调；
// 否则生成 6 位随机数字验证码。
func (s *AuthServiceImpl) generateCode() string {
	if s.allowMockCodeFallback {
		return MockVerificationCode
	}
	const digits = "0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			// crypto/rand 失败极罕见，退回固定码以不阻塞流程（已写日志）
			s.LogInternal("generateCode rand", err)
			return MockVerificationCode
		}
		b[i] = digits[n.Int64()]
	}
	return string(b)
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (s *AuthServiceImpl) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, *errcode.Error) {
	// 检查黑名单
	blackKey := s.blacklistKey(refreshToken)
	exists, err := s.rdb.Exists(ctx, blackKey).Result()
	if err == nil && exists > 0 {
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
			s.LogInternal("Refresh lookup user", lookupErr)
			return nil, errcode.ErrInternal
		}
	}

	if err := s.rdb.Set(ctx, blackKey, "1", refreshBlacklistTTL).Err(); err != nil {
		s.LogInternal("Refresh blacklist old token", err)
		return nil, errcode.ErrInternal
	}

	pair, issueErr := s.tokens.IssueTokenPair(claims.UserID, claims.Role)
	if issueErr != nil {
		s.LogInternal("Refresh issue token pair", issueErr)
		return nil, errcode.ErrInternal
	}

	return pair, nil
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (s *AuthServiceImpl) Logout(ctx context.Context, refreshToken string) *errcode.Error {
	if refreshToken == "" {
		return nil
	}

	var logoutErr *errcode.Error

	// 将 refresh token 加入 Redis 黑名单，TTL 等于 refresh token 有效期
	key := s.blacklistKey(refreshToken)
	if err := s.rdb.Set(ctx, key, "1", refreshBlacklistTTL).Err(); err != nil {
		s.LogInternal("Logout blacklist refresh token", err)
		logoutErr = errcode.ErrInternal
	}

	if claims, err := s.tokens.ParseRefreshToken(refreshToken); err == nil {
		revokeKey := s.revokeUserKey(claims.UserID)
		revokeTTL := s.tokens.AccessTokenTTL()
		if err := s.rdb.Set(ctx, revokeKey, time.Now().Unix(), revokeTTL).Err(); err != nil {
			s.LogInternal("Logout revoke access tokens", err)
			logoutErr = errcode.ErrInternal
		}
	}

	return logoutErr
}

// ── 内部辅助 ──────────────────────────────────────────────────────────────────

// verifyCode 校验验证码。
// 策略：先查 Redis，key 不存在时在开发阶段允许 MockVerificationCode 通过（便于本地无 Redis 调试）。
// 验证成功后删除 key，确保一次有效。
func (s *AuthServiceImpl) verifyCode(ctx context.Context, codeType, target, code string) bool {
	key := s.codeRedisKey(codeType, target)
	result, err := s.rdb.Eval(ctx, verifyCodeLua, []string{key}, code).Int()
	if err != nil {
		return false
	}

	switch result {
	case 1:
		return true
	case -1:
		return s.allowMockCodeFallback && code == MockVerificationCode
	default:
		return false
	}
}

// findUserByAccount 按优先级查找用户：phone → email → account（自动识别）。
func (s *AuthServiceImpl) findUserByAccount(ctx context.Context, phone, email, account string) (*repo.User, *errcode.Error) {
	var (
		user *repo.User
		err  error
	)

	switch {
	case phone != "":
		user, err = s.users.GetByPhone(ctx, phone)
	case email != "":
		user, err = s.users.GetByEmail(ctx, email)
	case account != "":
		// account 字段同时支持手机号和邮箱
		if looksLikePhone(account) {
			user, err = s.users.GetByPhone(ctx, account)
		} else {
			user, err = s.users.GetByEmail(ctx, account)
		}
	default:
		return nil, errcode.ErrBadRequest
	}

	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrInvalidCredentials
		}
		s.LogInternal("findUserByAccount lookup user", err)
		return nil, errcode.ErrInternal
	}

	return user, nil
}

// checkAccountExists 检查手机号/邮箱是否已注册，已注册返回错误。
func (s *AuthServiceImpl) checkAccountExists(ctx context.Context, phone, email string) *errcode.Error {
	if phone != "" {
		_, err := s.users.GetByPhone(ctx, phone)
		if err == nil {
			return errcode.ErrAlreadyExists
		}
		if !errors.Is(err, repo.ErrNotFound) {
			s.LogInternal("checkAccountExists lookup phone", err)
			return errcode.ErrInternal
		}
	}

	if email != "" {
		_, err := s.users.GetByEmail(ctx, email)
		if err == nil {
			return errcode.ErrAlreadyExists
		}
		if !errors.Is(err, repo.ErrNotFound) {
			s.LogInternal("checkAccountExists lookup email", err)
			return errcode.ErrInternal
		}
	}

	return nil
}

// issueResult 颁发 Token 并组装登录结果。
func (s *AuthServiceImpl) issueResult(user *repo.User) (*auth.LoginResult, *errcode.Error) {
	if user.Status == repo.StatusDisabled {
		return nil, errcode.ErrAccountDisabled
	}

	pair, err := s.tokens.IssueTokenPair(user.ID.String(), user.Role)
	if err != nil {
		s.LogInternal("issueResult issue token pair", err)
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

// defaultNickname 生成默认昵称。
func (s *AuthServiceImpl) defaultNickname(input *RegisterInput) string {
	if input.Nickname != "" {
		return input.Nickname
	}
	if input.Phone != "" && len(input.Phone) >= 4 {
		return "用户" + input.Phone[len(input.Phone)-4:]
	}
	if input.Email != "" {
		parts := strings.SplitN(input.Email, "@", 2)
		return "用户_" + parts[0]
	}
	return "新用户"
}

func (s *AuthServiceImpl) codeRedisKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:%s", s.codePrefix, codeType, target)
}

func (s *AuthServiceImpl) codeCooldownKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:cooldown:%s", s.codePrefix, codeType, target)
}

func (s *AuthServiceImpl) blacklistKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%s:blacklist:refresh:%x", s.codePrefix, h)
}

func (s *AuthServiceImpl) revokeUserKey(userID string) string {
	return fmt.Sprintf("%s:revoke:uid:%s", s.codePrefix, userID)
}

// ── 登录暴力破解防护 ──────────────────────────────────────────────────────────

// isLoginLocked 检查账号是否处于锁定状态。
// Redis 不可用时 fail-open（仅打 Warn 日志），避免 Redis 故障完全阻断登录。
func (s *AuthServiceImpl) isLoginLocked(ctx context.Context, account string) bool {
	if s.rdb == nil || account == "" {
		return false
	}
	exists, err := s.rdb.Exists(ctx, s.loginLockKey(account)).Result()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("isLoginLocked redis error, fail-open", zap.String("account_hash", account[:8]), zap.Error(err))
		}
		return false
	}
	return exists > 0
}

// recordLoginFail 原子递增失败计数，达到阈值时写入锁定 key。
func (s *AuthServiceImpl) recordLoginFail(ctx context.Context, account string) {
	if s.rdb == nil || account == "" {
		return
	}
	failKey := s.loginFailKey(account)
	lockKey := s.loginLockKey(account)
	if err := s.rdb.Eval(ctx, loginRecordFailLua, []string{failKey, lockKey},
		int64(loginFailWindow.Seconds()),
		loginMaxFailAttempts,
		int64(loginLockDuration.Seconds()),
	).Err(); err != nil {
		if s.Logger != nil {
			s.Logger.Warn("recordLoginFail redis error", zap.Error(err))
		}
	}
}

// clearLoginFail 登录成功后清除失败计数（锁定 key 等自然过期）。
func (s *AuthServiceImpl) clearLoginFail(ctx context.Context, account string) {
	if s.rdb == nil || account == "" {
		return
	}
	if err := s.rdb.Del(ctx, s.loginFailKey(account)).Err(); err != nil {
		if s.Logger != nil {
			s.Logger.Warn("clearLoginFail redis error", zap.Error(err))
		}
	}
}

// loginAccountID 从登录请求字段中提取用于防暴力破解计数的账号标识符。
func loginAccountID(phone, email, account string) string {
	switch {
	case phone != "":
		return strings.ToLower(strings.TrimSpace(phone))
	case email != "":
		return strings.ToLower(strings.TrimSpace(email))
	default:
		return strings.ToLower(strings.TrimSpace(account))
	}
}

func (s *AuthServiceImpl) loginFailKey(account string) string {
	h := sha256.Sum256([]byte(account))
	return fmt.Sprintf("%s:login:fail:%x", s.codePrefix, h)
}

func (s *AuthServiceImpl) loginLockKey(account string) string {
	h := sha256.Sum256([]byte(account))
	return fmt.Sprintf("%s:login:lock:%x", s.codePrefix, h)
}

// looksLikePhone 简单判断字符串是否看起来像手机号（全数字且长度在10-15之间）。
func looksLikePhone(s string) bool {
	if len(s) < 10 || len(s) > 15 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
