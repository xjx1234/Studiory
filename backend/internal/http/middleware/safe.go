package middleware

import (
	"errors"
	"net/http"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

const (
	// defaultMaxBodyBytes 默认最大请求体大小：2 MB。
	// 文件上传等场景应在各自路由单独放大。
	defaultMaxBodyBytes = 2 << 20 // 2 MB
)

// Safe 注册通用安全规则：
//  1. 限制请求体大小，防止超大 payload 打爆内存
func Safe() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, defaultMaxBodyBytes)

		c.Next()

		// Gin 在 ShouldBind/ShouldBindJSON 解析时若触发 MaxBytesReader 限制，
		// 会返回 *http.MaxBytesError（Go 1.19+）。
		// 遍历 c.Errors，以标准 errors.As 检测，不依赖错误字符串。
		//
		// 这里只给「自己没写响应」的 handler 兜底：走 request.Bind 的接口命中超限时，
		// Bind 已经写好 400 + err_body_too_large 并把原文塞进 c.Errors 供日志，
		// 此时再写一次就是 superfluous WriteHeader + 拼接出畸形 JSON。
		for _, ginErr := range c.Errors {
			var maxBytesErr *http.MaxBytesError
			if errors.As(ginErr.Err, &maxBytesErr) {
				c.Abort()
				if !c.Writer.Written() {
					resp.Fail(c, errcode.ErrBadRequest.WithMessage("err_body_too_large"))
				}
				return
			}
		}
	}
}
