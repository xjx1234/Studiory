package strutil

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"empty", "", 8, ""},
		{"shorter", "abc", 8, "abc"},
		{"equal", "abcdefgh", 8, "abcdefgh"},
		{"longer", "abcdefghij", 8, "abcdefgh"},
		{"zero", "abc", 0, "abc"},
		{"negative", "abc", -1, "abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.input, tc.maxLen)
			if got != tc.want {
				t.Fatalf("Truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		vals []string
		want string
	}{
		{"all empty", []string{"", ""}, ""},
		{"no args", nil, ""},
		{"first wins", []string{"a", "b"}, "a"},
		{"skip leading empty", []string{"", "b", "c"}, "b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FirstNonEmpty(tc.vals...)
			if got != tc.want {
				t.Fatalf("FirstNonEmpty(%v) = %q, want %q", tc.vals, got, tc.want)
			}
		})
	}
}

func TestLooksLikePhone(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid 11 digits", "13800138000", true},
		{"valid 10 digits", "1234567890", true},
		{"too short", "123456789", false},
		{"too long", "1234567890123456", false},
		{"contains letters", "1380013800a", false},
		{"email", "a@b.com", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LooksLikePhone(tc.input)
			if got != tc.want {
				t.Fatalf("LooksLikePhone(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskPII(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"phone 11 digits", "13800138000", "138****8000"},
		{"phone 10 digits", "1234567890", "123***7890"},
		{"email normal", "alice@example.com", "al****@example.com"},
		{"email short local", "ab@test.com", "****@test.com"},
		{"openid", "wx_openid_abc123", "wx_o****"},
		{"short string", "abc", "****"},
		{"4 chars", "abcd", "****"},
		{"5 chars", "abcde", "abcd****"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskPII(tc.input)
			if got != tc.want {
				t.Fatalf("MaskPII(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"already lower", "alice@example.com", "alice@example.com"},
		{"upper domain", "Alice@EXAMPLE.com", "alice@example.com"},
		{"surrounding space", "  alice@example.com  ", "alice@example.com"},
		{"plus tag kept", "alice+news@example.com", "alice+news@example.com"},
		{"empty", "", ""},
		{"only space", "   ", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeEmail(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := NormalizeEmail(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bare", "13800138000", "13800138000"},
		{"dashes", "138-0013-8000", "13800138000"},
		{"spaces", "138 0013 8000", "13800138000"},
		// 三种中国前缀写法必须与裸号落到同一个标识，否则冷却/配额能被换写法绕过。
		{"plus 86", "+8613800138000", "13800138000"},
		{"plus 86 spaced", "+86 138-0013-8000", "13800138000"},
		{"0086", "008613800138000", "13800138000"},
		{"bare 86", "8613800138000", "13800138000"},
		// 前缀后不是合法大陆号段：不动数字，避免误伤以 86 开头的他国/短号码。
		{"86 not china", "8612345", "8612345"},
		{"us number", "+1 415 555 0132", "14155550132"},
		{"parens", "(010) 8567 1234", "01085671234"},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePhone(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizePhone(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := NormalizePhone(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestNormalizeAccount(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"email", "Alice@Example.COM", "alice@example.com"},
		// 带分隔符的手机号含 '+'，用 LooksLikePhone 判形态会被误判成邮箱。
		{"phone with separators", " +86 138-0013-8000 ", "13800138000"},
		{"bare phone", "13800138000", "13800138000"},
		// 既不是邮箱也不是号码：只做去空白，不 lowercase——用户名大小写可能本就是身份的一部分。
		{"opaque username", "  Admin  ", "Admin"},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeAccount(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeAccount(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := NormalizeAccount(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}
