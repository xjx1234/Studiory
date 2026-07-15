package authservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"backend/internal/auth"
	"backend/internal/oauth"
	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/pkg/errcode"
	"backend/pkg/strutil"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

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
	return s.issueResult(ctx, user)
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
		s.LogInternal("loginWithCode lookup user", err, baseservice.TargetField(target))
		return nil, errcode.ErrInternal
	}

	return s.issueResult(ctx, user)
}

func (s *AuthServiceImpl) loginWithOAuth(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
	if s.oauth == nil {
		return nil, errcode.ErrInternal
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		return nil, errcode.ErrBadRequest
	}
	if !s.isAllowedOAuthProvider(provider) {
		return nil, errcode.ErrUnsupportedGrant
	}

	openID, nickname, err := s.resolveOAuthIdentity(ctx, provider, req)
	if err != nil {
		if errors.Is(err, oauth.ErrInvalidToken) {
			return nil, errcode.ErrInvalidToken
		}
		if errors.Is(err, oauth.ErrNoProvider) || errors.Is(err, oauth.ErrNotConfigured) {
			return nil, errcode.ErrUnsupportedGrant
		}
		s.LogInternal("loginWithOAuth verify token", err, zap.String("provider", provider))
		return nil, errcode.ErrInternal
	}
	if openID == "" {
		return nil, errcode.ErrBadRequest
	}

	user, lookupErr := s.oauth.GetUserByOAuth(ctx, provider, openID)
	if lookupErr == nil {
		return s.issueResult(ctx, user)
	}
	if !errors.Is(lookupErr, repo.ErrNotFound) {
		s.LogInternal("loginWithOAuth lookup user", lookupErr,
			zap.String("provider", provider),
			baseservice.TargetField(openID),
		)
		return nil, errcode.ErrInternal
	}

	if nickname == "" {
		nickname = oauthDefaultNickname(provider, openID)
	}
	created, createErr := s.createOAuthUser(ctx, nickname, provider, openID)
	if createErr != nil {
		s.LogInternal("loginWithOAuth create user", createErr,
			zap.String("provider", provider),
			baseservice.TargetField(openID),
		)
		return nil, errcode.ErrInternal
	}

	return s.issueResult(ctx, created)
}

func (s *AuthServiceImpl) resolveOAuthIdentity(ctx context.Context, provider string, req *auth.LoginRequest) (openID, nickname string, err error) {
	verifyReq := oauth.VerifyRequest{
		Provider:    provider,
		AccessToken: strings.TrimSpace(req.AccessToken),
		IDToken:     strings.TrimSpace(req.IDToken),
		OpenID:      strings.TrimSpace(req.OpenID),
	}

	if s.oauthVerifier != nil {
		identity, verifyErr := s.oauthVerifier.Verify(ctx, verifyReq)
		if verifyErr != nil {
			return "", "", verifyErr
		}
		return identity.OpenID, identity.Nickname, nil
	}

	// 未注入 Verifier 时兼容旧测试：仅 dev_mode + open_id 直传。
	if s.oauthDevMode && verifyReq.OpenID != "" && verifyReq.AccessToken == "" && verifyReq.IDToken == "" {
		return verifyReq.OpenID, "", nil
	}
	return "", "", oauth.ErrNoProvider
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
		if strutil.LooksLikePhone(account) {
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
		s.LogInternal("findUserByAccount lookup user", err,
			baseservice.TargetField(strutil.FirstNonEmpty(phone, email, account)),
		)
		return nil, errcode.ErrInternal
	}

	return user, nil
}
