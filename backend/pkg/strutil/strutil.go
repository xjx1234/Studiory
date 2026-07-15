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
