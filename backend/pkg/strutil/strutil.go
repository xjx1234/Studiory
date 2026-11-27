package strutil

import (
	"regexp"
	"strings"
)

// reChinaMobile 与 pkg/validator 的 phone_cn 保持一致（裸 11 位中国大陆手机号）。
// 这里单独声明而不是复用校验器，是为了让「归一化」不依赖 validator 包。
var reChinaMobile = regexp.MustCompile(`^1[3-9]\d{9}$`)

// reNonDigits 与 000007 迁移、scripts/check_phone_collisions.sql 的清洗口径一致：
// 删除所有非 ASCII 数字字符。
var reNonDigits = regexp.MustCompile(`[^0-9]`)

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
//  1. 删除所有非 ASCII 数字字符（空格、标点、换行、全角数字等一并删除）；
//  2. 剩余部分能明确判定为中国大陆手机号时，剥掉 0086 / 86 国家码，得到与
//     phone_cn 校验一致的裸 11 位号码。
//
// 第 1 步必须与 000007 迁移、scripts/check_phone_collisions.sql 同口径（都删除所有
// 非 ASCII 数字）：运行时拼出的查询值与存储键，要和迁移清洗后的 users.phone 对得上。
// 不要收窄成固定分隔符集合——逗号、下划线、换行等写法会被漏掉，导致这部分存量账号查不到。
// 两端共用的样例见 testdata/phone_normalization_golden.tsv。
//
// 其他国家号码无法可靠推断国家码，只做第 1 步，不做前缀剥离。
// 幂等：结果只含 ASCII 数字，且不再带可剥离的国家码。
func NormalizePhone(s string) string {
	s = reNonDigits.ReplaceAllString(s, "")

	for _, prefix := range []string{"0086", "86"} {
		if strings.HasPrefix(s, prefix) {
			if rest := strings.TrimPrefix(s, prefix); reChinaMobile.MatchString(rest) {
				return rest
			}
		}
	}
	return s
}

// NormalizeAccount 归一化「账号」这种同时允许手机号与邮箱（以及历史用户名）的字段。
//
// 以 @ 判定邮箱；不含 @ 时先按手机号规则清洗，只有清洗结果确实像号码（10-15 位数字）
// 才采用——否则原样返回 trim 后的输入，避免把 "Admin"、"user_123" 这类用户名洗成空串/残片。
// 不能直接用 LooksLikePhone 判原串："+86 138-0013-8000" 含分隔符，会被误判成非号码。
func NormalizeAccount(s string) string {
	s = strings.TrimSpace(s)
	if strings.ContainsRune(s, '@') {
		return NormalizeEmail(s)
	}
	if phone := NormalizePhone(s); LooksLikePhone(phone) {
		return phone
	}
	return s
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
