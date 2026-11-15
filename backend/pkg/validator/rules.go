package validator

import (
	"regexp"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

var (
	reChinaPhone   = regexp.MustCompile(`^1[3-9]\d{9}$`)
	rePasswordSafe = regexp.MustCompile(`[a-zA-Z]`) // 至少含一个字母
	rePasswordNum  = regexp.MustCompile(`[0-9]`)    // 至少含一个数字
)

// maxPasswordBytes 是 bcrypt 能处理的密码字节上限，超过会返回 ErrPasswordTooLong。
const maxPasswordBytes = 72

// registerCustomRules 注册项目特有的校验规则。
// 新增规则时按同样模式扩展即可。
func registerCustomRules(v *validator.Validate, zhTrans, enTrans ut.Translator) {
	registerPhoneCN(v, zhTrans, enTrans)
	registerPassword(v, zhTrans, enTrans)
}

// phone_cn 校验中国大陆手机号（1[3-9]xxxxxxxxx）
func registerPhoneCN(v *validator.Validate, zhTrans, enTrans ut.Translator) {
	_ = v.RegisterValidation("phone_cn", func(fl validator.FieldLevel) bool {
		return reChinaPhone.MatchString(fl.Field().String())
	})

	_ = v.RegisterTranslation("phone_cn", zhTrans,
		func(ut ut.Translator) error {
			return ut.Add("phone_cn", "{0} 必须是有效的中国大陆手机号", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("phone_cn", fe.Field())
			return t
		},
	)

	_ = v.RegisterTranslation("phone_cn", enTrans,
		func(ut ut.Translator) error {
			return ut.Add("phone_cn", "{0} must be a valid China mobile number", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("phone_cn", fe.Field())
			return t
		},
	)
}

// strong_password 校验密码：8 ≤ 长度 ≤ 72 字节，至少含一个字母和一个数字。
//
// 上限 72 来自 bcrypt：x/crypto/bcrypt 对超过 72 字节的密码直接返回
// ErrPasswordTooLong，若不在此拦截，注册/改密会变成 500。这里按字节计数
// （len 而非 utf8.RuneCountInString），与 bcrypt 的实际限制保持一致。
func registerPassword(v *validator.Validate, zhTrans, enTrans ut.Translator) {
	_ = v.RegisterValidation("strong_password", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return len(s) >= 8 && len(s) <= maxPasswordBytes && rePasswordSafe.MatchString(s) && rePasswordNum.MatchString(s)
	})

	_ = v.RegisterTranslation("strong_password", zhTrans,
		func(ut ut.Translator) error {
			return ut.Add("strong_password", "{0} 需为 8-72 字节，且包含字母和数字", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("strong_password", fe.Field())
			return t
		},
	)

	_ = v.RegisterTranslation("strong_password", enTrans,
		func(ut ut.Translator) error {
			return ut.Add("strong_password", "{0} must be 8-72 bytes and contain both letters and numbers", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("strong_password", fe.Field())
			return t
		},
	)
}
