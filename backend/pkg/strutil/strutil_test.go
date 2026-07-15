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
