package authservice

import (
	"context"
	"errors"
	"strings"

	"backend/internal/auth"
	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/pkg/errcode"
	"backend/pkg/strutil"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = bcrypt.DefaultCost

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
		s.LogInternal("registerWithPassword hash password", err,
			baseservice.TargetField(strutil.FirstNonEmpty(input.Phone, input.Email)),
		)
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
		s.LogInternal("registerWithPassword create user", createErr,
			baseservice.TargetField(strutil.FirstNonEmpty(input.Phone, input.Email)),
		)
		return nil, errcode.ErrInternal
	}

	return s.issueResult(ctx, user)
}

func (s *AuthServiceImpl) registerWithCode(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error) {
	if input.Code == "" || input.CodeType == "" {
		return nil, errcode.ErrBadRequest
	}

	target := input.Phone
	if input.CodeType == "email" {
		target = input.Email
	}
	if target == "" {
		return nil, errcode.ErrBadRequest
	}

	// 与验证码登录共用防暴力破解：同一 target 连续猜错会锁定，避免仅靠 IP 限流约束 6 位码。
	account := loginAccountID(
		condStr(input.CodeType == "sms", target, ""),
		condStr(input.CodeType == "email", target, ""),
		"",
	)
	if s.isLoginLocked(ctx, account) {
		return nil, errcode.ErrAccountLocked
	}

	if !s.verifyCode(ctx, input.CodeType, target, input.Code) {
		s.recordLoginFail(ctx, account)
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
		s.LogInternal("registerWithCode create user", createErr,
			baseservice.TargetField(strutil.FirstNonEmpty(input.Phone, input.Email)),
		)
		return nil, errcode.ErrInternal
	}

	s.clearLoginFail(ctx, account)
	return s.issueResult(ctx, user)
}

// checkAccountExists 检查手机号/邮箱是否已注册，已注册返回错误。
func (s *AuthServiceImpl) checkAccountExists(ctx context.Context, phone, email string) *errcode.Error {
	if phone != "" {
		_, err := s.users.GetByPhone(ctx, phone)
		if err == nil {
			return errcode.ErrAlreadyExists
		}
		if !errors.Is(err, repo.ErrNotFound) {
			s.LogInternal("checkAccountExists lookup phone", err, baseservice.TargetField(phone))
			return errcode.ErrInternal
		}
	}

	if email != "" {
		_, err := s.users.GetByEmail(ctx, email)
		if err == nil {
			return errcode.ErrAlreadyExists
		}
		if !errors.Is(err, repo.ErrNotFound) {
			s.LogInternal("checkAccountExists lookup email", err, baseservice.TargetField(email))
			return errcode.ErrInternal
		}
	}

	return nil
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
