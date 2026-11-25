package strutil

import (
	"regexp"
	"strings"
)

// reChinaMobile 与 pkg/validator 的 phone_cn 保持一致（裸 11 位中国大陆手机号）。
// 这里单独声明而不是复用校验器，是为了让「归一化」不依赖 validator 包。
var reChinaMobile = regexp.MustCompile(`^1[3-9]\d{9}$`)

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

// NormalizeEmail 归一化邮箱标识：去首尾空白并转小写。
//
// 域名大小写不敏感，local part 理论上可区分大小写，但主流服务商（Gmail/Outlook/qq 等）
// 一律按大小写不敏感投递，因此取小写作为项目内唯一标识。
// 不剥离 `+tag`：那是子地址策略，剥离会把两个真实不同的邮箱并成一个。
//
// 归一化是幂等的：NormalizeEmail(NormalizeEmail(x)) == NormalizeEmail(x)。
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizePhone 归一化手机号，使同一个号码的不同书写形式落到同一个标识：
//  1. 去掉首尾空白；
//  2. 去掉书写分隔符（空格、-、.、/、括号）与国际前缀符 +；
//  3. 中国大陆号码再去掉 0086 / +86 / 86 国家码前缀，得到与 phone_cn 校验一致的裸 11 位号码。
//
// 只对能明确判定为中国大陆手机号（去前缀后仍匹配 1[3-9]\d{9}）的号码剥国家码，
// 其他国家号码无法可靠推断国家码，只做第 1、2 步。
//
// 幂等：归一化结果不含分隔符与国家码前缀，再次归一化不变。
func NormalizePhone(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '-', '.', '/', '(', ')', '+':
			return -1
		}
		return r
	}, s)

	for _, prefix := range []string{"0086", "86"} {
		if strings.HasPrefix(s, prefix) {
			if rest := strings.TrimPrefix(s, prefix); reChinaMobile.MatchString(rest) {
				return rest
			}
		}
	}
	return s
}

// NormalizeAccount 归一化「账号」这种同时允许手机号与邮箱的字段。
//
// 以 @ 判定形态：邮箱必须含 @，手机号永远不含，因此不会误判。
// 不用 LooksLikePhone 判别——"+86 138-0013-8000" 含分隔符，恰好会被它判成非手机号。
func NormalizeAccount(s string) string {
	s = strings.TrimSpace(s)
	if strings.ContainsRune(s, '@') {
		return NormalizeEmail(s)
	}
	return NormalizePhone(s)
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
