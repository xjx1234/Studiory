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

// strong_password 校验密码：长度 ≥ 8，至少含一个字母和一个数字
func registerPassword(v *validator.Validate, zhTrans, enTrans ut.Translator) {
	_ = v.RegisterValidation("strong_password", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return len(s) >= 8 && rePasswordSafe.MatchString(s) && rePasswordNum.MatchString(s)
	})

	_ = v.RegisterTranslation("strong_password", zhTrans,
		func(ut ut.Translator) error {
			return ut.Add("strong_password", "{0} 至少 8 位，且需包含字母和数字", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("strong_password", fe.Field())
			return t
		},
	)

	_ = v.RegisterTranslation("strong_password", enTrans,
		func(ut ut.Translator) error {
			return ut.Add("strong_password", "{0} must be at least 8 characters with letters and numbers", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("strong_password", fe.Field())
			return t
		},
	)
}
