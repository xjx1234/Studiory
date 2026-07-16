package validator

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	govalidator "github.com/go-playground/validator/v10"
)

// engine 返回 Init() 接管后的全局 validator 引擎，用于直接调用自定义规则。
func engine(t *testing.T) *govalidator.Validate {
	t.Helper()
	Init()

	v, ok := binding.Validator.Engine().(*govalidator.Validate)
	if !ok {
		t.Fatal("expected gin binding validator engine to be *validator.Validate")
	}
	return v
}

func TestPhoneCN_ValidNumbers(t *testing.T) {
	v := engine(t)
	valid := []string{
		"13800138000",
		"15912345678",
		"19999999999",
		"18600000000",
	}
	for _, phone := range valid {
		if err := v.Var(phone, "phone_cn"); err != nil {
			t.Errorf("phone_cn(%q) should be valid, got error: %v", phone, err)
		}
	}
}

func TestPhoneCN_InvalidNumbers(t *testing.T) {
	v := engine(t)
	invalid := []string{
		"",               // 空
		"12800138000",    // 第二位不在 3-9 范围
		"1380013800",     // 10 位，少一位
		"138001380000",   // 12 位，多一位
		"abcdefghijk1",   // 非数字
		"+8613800138000", // 带国际前缀
		"023800138000",   // 不是以 1 开头
	}
	for _, phone := range invalid {
		if err := v.Var(phone, "phone_cn"); err == nil {
			t.Errorf("phone_cn(%q) should be invalid, got nil error", phone)
		}
	}
}

func TestStrongPassword_Valid(t *testing.T) {
	v := engine(t)
	valid := []string{
		"abc12345",
		"Password1",
		"a1b2c3d4",
		"StrongPass123!",
	}
	for _, pw := range valid {
		if err := v.Var(pw, "strong_password"); err != nil {
			t.Errorf("strong_password(%q) should be valid, got error: %v", pw, err)
		}
	}
}

func TestStrongPassword_Invalid(t *testing.T) {
	v := engine(t)
	invalid := []string{
		"",                  // 空
		"short1",            // 长度 < 8
		"alllettersnodigit", // 无数字
		"12345678",          // 无字母
		"1234567",           // 太短且无字母
	}
	for _, pw := range invalid {
		if err := v.Var(pw, "strong_password"); err == nil {
			t.Errorf("strong_password(%q) should be invalid, got nil error", pw)
		}
	}
}

// testRequest 用于验证字段名替换（JSON tag）与双语翻译是否生效。
type testRequest struct {
	Phone    string `json:"phone"    binding:"required,phone_cn"`
	Password string `json:"password" binding:"required,strong_password"`
}

func TestTranslateErrors_UsesJSONTagAsFieldName(t *testing.T) {
	v := engine(t)

	err := v.Struct(testRequest{Phone: "123", Password: "abc"})
	ve, ok := err.(govalidator.ValidationErrors)
	if !ok {
		t.Fatalf("expected validator.ValidationErrors, got %T (%v)", err, err)
	}

	fields := TranslateErrors(ve, "zh")
	if _, ok := fields["phone"]; !ok {
		t.Errorf("expected translated errors to be keyed by JSON tag %q, got keys %v", "phone", fields)
	}
	if _, ok := fields["password"]; !ok {
		t.Errorf("expected translated errors to be keyed by JSON tag %q, got keys %v", "password", fields)
	}
}

func TestTranslateErrors_ZhAndEnProduceDifferentMessages(t *testing.T) {
	v := engine(t)

	err := v.Struct(testRequest{Phone: "123", Password: "abc"})
	ve, ok := err.(govalidator.ValidationErrors)
	if !ok {
		t.Fatalf("expected validator.ValidationErrors, got %T (%v)", err, err)
	}

	zh := TranslateErrors(ve, "zh")
	en := TranslateErrors(ve, "en")

	if zh["phone"] == "" || en["phone"] == "" {
		t.Fatalf("expected non-empty translated messages, zh=%v en=%v", zh, en)
	}
	if zh["phone"] == en["phone"] {
		t.Errorf("expected zh and en translations to differ, both got %q", zh["phone"])
	}
}

func TestTranslateErrors_UnknownLanguageFallsBackToZh(t *testing.T) {
	v := engine(t)

	err := v.Struct(testRequest{Phone: "123", Password: "abc"})
	ve, ok := err.(govalidator.ValidationErrors)
	if !ok {
		t.Fatalf("expected validator.ValidationErrors, got %T (%v)", err, err)
	}

	zh := TranslateErrors(ve, "zh")
	fr := TranslateErrors(ve, "fr-FR") // 不支持的语言应退回 zh

	if fr["phone"] != zh["phone"] {
		t.Errorf("expected unsupported language to fall back to zh: fr=%q zh=%q", fr["phone"], zh["phone"])
	}
}

func TestValidation_PassesForValidRequest(t *testing.T) {
	v := engine(t)

	err := v.Struct(testRequest{Phone: "13800138000", Password: "Str0ng!Pass"})
	if err != nil {
		t.Errorf("expected valid request to pass validation, got: %v", err)
	}
}
