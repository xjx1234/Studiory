package middleware

import (
	pkgi18n "backend/pkg/i18n"

	"github.com/gin-gonic/gin"
)

// I18n 检测请求语言并将 Localizer 注入 Gin Context。
//
// 语言检测优先级：
//  1. Query 参数 `lang`（如 ?lang=en）
//  2. 请求头 `Accept-Language`（浏览器/客户端自动带）
//  3. 默认简体中文
func I18n() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := c.Query("lang")
		if lang == "" {
			lang = c.GetHeader("Accept-Language")
		}

		localizer := pkgi18n.NewLocalizer(lang)
		c.Set(pkgi18n.LocalizerKey, localizer)
		c.Next()
	}
}
