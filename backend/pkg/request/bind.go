// Package request 提供统一的请求绑定与参数校验辅助函数。
//
// 使用方式：
//
//	var req MyRequest
//	if !request.Bind(c, &req) {
//	    return  // 已自动写入错误响应，直接 return 即可
//	}
package request

import (
	"errors"
	"strings"

	"backend/pkg/errcode"
	"backend/pkg/resp"
	pkgvalidator "backend/pkg/validator"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Bind 绑定 JSON Body 并执行校验。
// 失败时自动写入标准错误响应并返回 false，调用方直接 return 即可。
func Bind(c *gin.Context, obj any) bool {
	return bindWith(c, obj, func() error {
		return c.ShouldBindJSON(obj)
	})
}

// BindQuery 绑定 Query 参数并执行校验。
func BindQuery(c *gin.Context, obj any) bool {
	return bindWith(c, obj, func() error {
		return c.ShouldBindQuery(obj)
	})
}

// BindURI 绑定路径参数并执行校验。
func BindURI(c *gin.Context, obj any) bool {
	return bindWith(c, obj, func() error {
		return c.ShouldBindUri(obj)
	})
}

// bindWith 通用绑定逻辑，区分 validator.ValidationErrors 和其他错误。
func bindWith(c *gin.Context, _ any, bindFn func() error) bool {
	err := bindFn()
	if err == nil {
		return true
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		lang := detectLang(c)
		fields := pkgvalidator.TranslateErrors(ve, lang)
		resp.FailWithValidation(c, fields)
	} else {
		resp.FailWithMessage(c, errcode.ErrBadRequest, err.Error())
	}

	return false
}

// detectLang 从请求中提取语言标签（优先 ?lang=，其次 Accept-Language）。
func detectLang(c *gin.Context) string {
	if lang := c.Query("lang"); lang != "" {
		return lang
	}
	// Accept-Language 可能是 "zh-CN,zh;q=0.9,en;q=0.8"，取第一个
	accept := c.GetHeader("Accept-Language")
	if accept != "" {
		return strings.SplitN(accept, ",", 2)[0]
	}
	return "zh"
}
