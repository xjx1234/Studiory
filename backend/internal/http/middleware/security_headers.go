package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders 注入常见的 HTTP 安全响应头。
//
// 无论环境均写入：
//   - X-Content-Type-Options: nosniff          — 防止 MIME 嗅探攻击
//   - X-Frame-Options: DENY                    — 禁止页面被 iframe 嵌套（点击劫持防御）
//   - X-XSS-Protection: 0                      — 关闭旧式浏览器 XSS 过滤器（现代推荐依赖 CSP）
//   - Referrer-Policy: strict-origin-when-cross-origin — 跨域请求仅发送 Origin
//
// 仅生产环境（isProd=true）写入：
//   - Strict-Transport-Security: max-age=63072000; includeSubDomains
//     — 强制 HTTPS 两年；开发环境走 HTTP，写该头无意义且会导致浏览器拒绝非 HTTPS 请求
func SecurityHeaders(isProd bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "0")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		if isProd {
			// 2 年（63072000 秒），含子域
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		c.Next()
	}
}
