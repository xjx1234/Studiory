package strutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestNormalizePhoneGoldenTable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "phone_normalization_golden.tsv"))
	if err != nil {
		t.Fatalf("read golden table: %v", err)
	}

	seen := make(map[string]string)
	cases := 0
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cases++

		cols := strings.SplitN(line, "\t", 2)
		if len(cols) != 2 {
			t.Fatalf("line %d: want 2 tab-separated columns, got %q", i+1, line)
		}
		in := unescapeGoldenField(cols[0])
		want := unescapeGoldenField(cols[1])
		if want == "(empty)" {
			want = ""
		}
		if prev, ok := seen[in]; ok {
			t.Fatalf("line %d: duplicate input %q (previous expected %q)", i+1, in, prev)
		}
		seen[in] = want

		t.Run(fmt.Sprintf("line_%d", i+1), func(t *testing.T) {
			got := NormalizePhone(in)
			if got != want {
				t.Fatalf("NormalizePhone(%q) = %q, want %q", in, got, want)
			}
			if again := NormalizePhone(got); again != got {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}

	if cases == 0 {
		t.Fatal("golden table is empty")
	}
}

// unescapeGoldenField 只识别黄金样例表文档里约定的 \t 与 \n，其他反斜杠序列原样保留。
func unescapeGoldenField(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestNormalizeAccount(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"email", "Alice@Example.COM", "alice@example.com"},
		// 带分隔符的手机号含 '+'，用 LooksLikePhone 判形态会被误判成邮箱。
		{"phone with separators", " +86 138-0013-8000 ", "13800138000"},
		{"phone with comma", "138,0013,8000", "13800138000"},
		{"bare phone", "13800138000", "13800138000"},
		// 既不是邮箱也不是号码：只做去空白，不 lowercase——用户名大小写可能本就是身份的一部分。
		{"opaque username", "  Admin  ", "Admin"},
		{"username with digits", "user_123", "user_123"},
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
