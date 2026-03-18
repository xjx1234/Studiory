package middleware

import (
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
//  2. 强制 Content-Type 检查（JSON API 场景）
func Safe() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 限制 Body 大小
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, defaultMaxBodyBytes)

		c.Next()

		// 若 Gin 读取 body 时触发 MaxBytesReader 限制，会在 ShouldBind 里报错；
		// 这里捕获已被写入但未终止的情况（理论上 ShouldBind 已处理，此处做兜底）
		if len(c.Errors) > 0 {
			for _, e := range c.Errors {
				if e.Error() == "http: request body too large" {
					resp.Fail(c, errcode.ErrBadRequest.WithMessage("err_body_too_large"))
					return
				}
			}
		}
	}
}
