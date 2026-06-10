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
