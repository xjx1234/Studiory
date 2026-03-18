// Package validator 封装 go-playground/validator/v10 的初始化。
//
// 功能：
//  1. 接管 Gin 默认的 Validator，注册 zh/en 双语翻译
//  2. 将 struct 字段的 JSON tag 名作为错误消息中的字段名（更对前端友好）
//  3. 暴露 TranslateErrors，将 ValidationErrors 翻译为 map[字段名]错误消息
package validator

import (
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin/binding"
	enLocale "github.com/go-playground/locales/en"
	zhLocale "github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
	zhTranslations "github.com/go-playground/validator/v10/translations/zh"
)

var (
	uni      *ut.UniversalTranslator
	initOnce sync.Once
)

// Init 初始化全局翻译器，并替换 Gin 的 Validator 实现。
// 应在 main 中、NewRouter 之前调用一次。
func Init() {
	initOnce.Do(func() {
		zh := zhLocale.New()
		en := enLocale.New()

		// 默认语言 zh，同时支持 en
		uni = ut.New(zh, zh, en)

		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}

		// 用 JSON tag 名替代 Go 字段名，让错误消息对前端更友好
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return fld.Name
			}
			return name
		})

		// 注册双语翻译
		zhTrans, _ := uni.GetTranslator("zh")
		enTrans, _ := uni.GetTranslator("en")

		_ = zhTranslations.RegisterDefaultTranslations(v, zhTrans)
		_ = enTranslations.RegisterDefaultTranslations(v, enTrans)

		// 注册自定义校验规则
		registerCustomRules(v, zhTrans, enTrans)
	})
}

// TranslateErrors 将 ValidationErrors 翻译为 map[字段名]错误消息。
// lang 传入语言标签，如 "zh"、"en"；不匹配时退回 zh。
func TranslateErrors(errs validator.ValidationErrors, lang string) map[string]string {
	result := make(map[string]string, len(errs))

	// 取语言标签前缀（如 "zh-CN" → "zh"，"en-US" → "en"）
	prefix := strings.SplitN(strings.ToLower(lang), "-", 2)[0]
	trans, found := uni.GetTranslator(prefix)
	if !found {
		trans, _ = uni.GetTranslator("zh")
	}

	for _, e := range errs {
		result[e.Field()] = e.Translate(trans)
	}
	return result
}
