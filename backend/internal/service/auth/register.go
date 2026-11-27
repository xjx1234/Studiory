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

// bcryptCost 是当前目标 cost：新密码、dummy hash 与登录时的升级都以它为准。
// 具体取值与理由见 auth.PasswordHashCost。
const bcryptCost = auth.PasswordHashCost

// dummyPasswordHash 是 cost 与线上 hash 完全一致的占位 hash。
//
// 它唯一的作用是让「账号不存在 / 账号没有密码」与「密码错误」付出同一次 bcrypt 计算：
// cost 12 的一次比较约 200–400ms，跳过它会让响应耗时直接变成账号存在性的判据。
// 用 bcryptCost 生成而非写死字面量，避免有人调整 cost 后 dummy 悄悄漂移
// （TestDummyPasswordHashCostMatchesBcryptCost 会盯住这一点）。
var dummyPasswordHash = mustBcryptHash("auth-timing-equalizer")

func mustBcryptHash(password string) []byte {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		// 只有 cost 越界才会失败，属编程错误：启动即崩比线上静默退化成不等时安全
		panic("auth: init dummy bcrypt hash: " + err.Error())
	}
	return h
}

// bcryptCompare 是包内可替换的 bcrypt 比较接缝：测试替换它即可确定性地断言
// 「每条失败分支恰好比较一次、且用的是 dummy」，不依赖时间断言（CI 上必然 flaky）。
var bcryptCompare = bcrypt.CompareHashAndPassword

// verifyPassword 始终执行一次 bcrypt 比较，返回密码是否正确。
//
// hash 为空（用户不存在，或账号只有 OAuth 身份、没有密码）时用 dummyPasswordHash
// 消耗同样的时间，并恒返回 false——dummy 的明文即使被猜到也不会成为登录凭据。
func verifyPassword(hash, password string) bool {
	if hash == "" {
		_ = bcryptCompare(dummyPasswordHash, []byte(password))
		return false
	}
	return bcryptCompare([]byte(hash), []byte(password)) == nil
}

// upgradePasswordHash 在登录成功后把低于目标 cost 的库内 hash 就地升级。
//
// 为什么放在登录：只有这一刻同时持有明文密码与已验证的 hash，不需要用户改密。
// 为什么用 CAS：并发的改密若已经写入新 hash，这里必须放弃——否则会把旧密码的
// hash 写回去，等于撤销用户刚做的改密（`UpdatePassword` 是无条件覆盖，不能用）。
// 失败一律只记日志：升级是后台维护动作，不能影响一次已经成功的登录。
func (s *AuthServiceImpl) upgradePasswordHash(ctx context.Context, user *repo.User, password string) {
	if user == nil || user.PasswordHash == nil || *user.PasswordHash == "" {
		return
	}
	oldHash := *user.PasswordHash
	cost, err := bcrypt.Cost([]byte(oldHash))
	if err != nil {
		// 库里的 hash 不合 bcrypt 格式：登录校验已经通过是不可能的，这里只可能
		// 出现在脏数据上；记日志但不阻断登录。
		s.LogInternal(ctx, "upgradePasswordHash bcrypt.Cost", err)
		return
	}
	if cost >= bcryptCost {
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		s.LogInternal(ctx, "upgradePasswordHash generate", err)
		return
	}
	newHashStr := string(newHash)
	updated, err := s.users.UpdatePasswordIfUnchanged(ctx, user.ID, oldHash, newHashStr)
	if err != nil {
		s.LogInternal(ctx, "upgradePasswordHash update", err)
		return
	}
	if !updated {
		// 并发改密已换掉 hash（或用户被删除）：保留库内现状。
		return
	}
	user.PasswordHash = &newHashStr
}

func (s *AuthServiceImpl) Register(ctx context.Context, input *RegisterInput) (*auth.LoginResult, *errcode.Error) {
	// 就地归一化标识：后续 registrationTarget/verifyCode/checkAccountExists 与写库
	// 用的必须是同一个形式，input 由 handler 每请求新建，不会跨请求复用。
	input.Canonicalize()

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
		s.LogInternal(ctx, "registerWithPassword hash password", hashErr,
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
		s.LogInternal(ctx, "registerWithPassword create user", createErr,
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
		s.LogInternal(ctx, "registerWithCode create user", createErr,
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
		valid, codeErr := s.verifyCode(ctx, codeType, target, input.Code)
		if codeErr != nil {
			return "", codeErr
		}
		if !valid {
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
//   - grant_type=code：沿用显式 code_type，但同样只允许一个标识——验证码只能证明其中一个的归属，
//     另一个若被原样存下，就等于允许「用自己收到的码 + 别人的邮箱/手机号」抢注。
//   - 其他（密码注册）：只允许一个标识，验证码类型由它推断——避免「手机号验了、邮箱悬空」，
//     也保证「没填任何标识」直接被拒（否则能注册出一个既不能登录也找不回密码的账号）。
func registrationTarget(input *RegisterInput) (codeType, target string, e *errcode.Error) {
	if input.GrantType == "code" {
		// 只允许一个标识：验证码只证明其中一个的归属，另一个会被原样存进账号。
		// 不拦的话，「用自己的手机号收码 + 填受害者的邮箱」仍然可以抢占邮箱。
		if input.Phone != "" && input.Email != "" {
			return "", "", errcode.ErrBadRequest
		}
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
//
// 这里再归一化一次，而不是依赖调用方先做过：查重与写库必须是同一个标识形式，
// 否则「大小写不同的两次注册」会同时通过查重、再被数据库唯一索引打回 500。
func (s *AuthServiceImpl) checkAccountExists(ctx context.Context, phone, email string) *errcode.Error {
	phone = strutil.NormalizePhone(phone)
	email = strutil.NormalizeEmail(email)

	if phone != "" {
		_, err := s.users.GetByPhone(ctx, phone)
		if err == nil {
			return errcode.ErrAlreadyExists
		}
		if !errors.Is(err, repo.ErrNotFound) {
			s.LogInternal(ctx, "checkAccountExists lookup phone", err, baseservice.TargetField(phone))
			return errcode.ErrInternal
		}
	}

	if email != "" {
		_, err := s.users.GetByEmail(ctx, email)
		if err == nil {
			return errcode.ErrAlreadyExists
		}
		if !errors.Is(err, repo.ErrNotFound) {
			s.LogInternal(ctx, "checkAccountExists lookup email", err, baseservice.TargetField(email))
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
