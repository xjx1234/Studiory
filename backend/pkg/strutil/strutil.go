package strutil

import "strings"

// NullableStr 将非空（trim 后）字符串转为指针，空字符串返回 nil。
func NullableStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// Truncate 截断字符串至最多 maxLen 个字符，避免切片越界。
// 适合用于日志中的敏感/长字段脱敏。
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// FirstNonEmpty 返回参数列表中第一个非空字符串，全为空时返回空字符串。
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// LooksLikePhone 简单判断字符串是否看起来像手机号（全数字且长度在 10-15 之间）。
// 仅用于账号字段自动识别（手机号 vs 邮箱）等启发式场景，不做严格号段校验。
func LooksLikePhone(s string) bool {
	if len(s) < 10 || len(s) > 15 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// LooksLikeEmail 简单判断字符串是否看起来像邮箱地址。
// 仅做基本格式校验（含 @ 和 .），不做 RFC 5322 完整校验。
func LooksLikeEmail(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	domain := s[at+1:]
	return strings.IndexByte(domain, '.') > 0
}

// MaskPII 对手机号/邮箱等 PII 进行脱敏，用于日志输出。
//   - 手机号：显示前 3 + 后 4（如 13800138000 → 138****8000）
//   - 邮箱：显示前 2 + 完整域名（如 alice@example.com → al****@example.com）
//   - 其他：显示前 4 + ****（如 openid_abc → open****）
func MaskPII(s string) string {
	if s == "" {
		return ""
	}
	if LooksLikeEmail(s) {
		at := strings.IndexByte(s, '@')
		if at <= 2 {
			return "****" + s[at:]
		}
		return s[:2] + "****" + s[at:]
	}
	if LooksLikePhone(s) {
		if len(s) <= 7 {
			return strings.Repeat("*", len(s)-3) + s[len(s)-3:]
		}
		return s[:3] + strings.Repeat("*", len(s)-7) + s[len(s)-4:]
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}
