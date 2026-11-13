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

	// 手机号/邮箱是「先到先得」的标识：只凭账号+密码就能建号时，任何人都能抢注别人的号码，
	// 等机主改用验证码登录时，账号已经落在攻击者名下。因此密码注册同样要先通过一次性验证码，
	// 除非显式配置 auth.require_code_for_password_register=false（生产环境禁止）。
	account, err := s.verifyRegistrationOwnership(ctx, input, s.requireCodeForPasswordRegister)
	if err != nil {
		return nil, err
	}

	hash, hashErr := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
	if hashErr != nil {
		s.LogInternal("registerWithPassword hash password", hashErr,
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

	s.clearLoginFail(ctx, account)
	return s.issueResult(ctx, user)
}

func (s *AuthServiceImpl) registerWithCode(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error) {
	if input.Code == "" || input.CodeType == "" {
		return nil, errcode.ErrBadRequest
	}

	account, err := s.verifyRegistrationOwnership(ctx, input, true)
	if err != nil {
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

// verifyRegistrationOwnership 注册前的归属校验 + 查重，返回用于暴力破解计数的 account。
//
// 顺序是「先验码、后查重」，这一点本身就是修复内容：反过来的话 /register 会变成
// 「这个手机号注册了吗」的免费查询接口（账号枚举 oracle），攻击者无需持有号码即可批量撞库。
// 先要求一次性验证码后，只有真正收到短信/邮件的人才会看到「已存在」。
//
// requireCode=false 只应来自开发/内网部署的显式配置；调用方负责把关。
func (s *AuthServiceImpl) verifyRegistrationOwnership(ctx context.Context, input *RegisterInput, requireCode bool) (string, *errcode.Error) {
	codeType, target, err := registrationTarget(input)
	if err != nil {
		return "", err
	}

	// 与验证码登录共用同一套锁定计数：同一 target 连续猜错会锁定，避免只靠 IP 限流约束 6 位码。
	account := loginAccountID(
		condStr(codeType == "sms", target, ""),
		condStr(codeType == "email", target, ""),
		"",
	)

	if requireCode {
		if s.isLoginLocked(ctx, account) {
			return "", errcode.ErrAccountLocked
		}
		if !s.verifyCode(ctx, codeType, target, input.Code) {
			s.recordLoginFail(ctx, account)
			return "", errcode.ErrInvalidCode
		}
	}

	if e := s.checkAccountExists(ctx, input.Phone, input.Email); e != nil {
		return "", e
	}

	return account, nil
}

// registrationTarget 推断本次注册要校验的验证码类型与目标。
//
//   - grant_type=code：沿用显式 code_type（历史上允许「两个标识都填、只验其中一个」，保持不变）。
//   - 其他（密码注册）：只允许一个标识，验证码类型由它推断——避免「手机号验了、邮箱悬空」，
//     也保证「没填任何标识」直接被拒（否则能注册出一个既不能登录也找不回密码的账号）。
func registrationTarget(input *RegisterInput) (codeType, target string, e *errcode.Error) {
	if input.GrantType == "code" {
		switch input.CodeType {
		case "sms":
			if input.Phone == "" {
				return "", "", errcode.ErrBadRequest
			}
			return "sms", input.Phone, nil
		case "email":
			if input.Email == "" {
				return "", "", errcode.ErrBadRequest
			}
			return "email", input.Email, nil
		default:
			return "", "", errcode.ErrBadRequest
		}
	}

	var derived string
	switch {
	case input.Phone != "" && input.Email != "":
		// 密码注册只校验一个标识，另一个就是未验证的悬空地址，直接拒绝更诚实。
		return "", "", errcode.ErrBadRequest
	case input.Phone != "":
		derived = "sms"
		target = input.Phone
	case input.Email != "":
		derived = "email"
		target = input.Email
	default:
		return "", "", errcode.ErrBadRequest
	}

	// 显式传了 code_type 就必须与标识一致，否则等于让客户端指着一个没填的渠道验码。
	if input.CodeType != "" && input.CodeType != derived {
		return "", "", errcode.ErrBadRequest
	}
	return derived, target, nil
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
