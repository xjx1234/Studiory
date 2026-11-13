package service

import (
	"strings"
	"testing"

	"backend/pkg/errcode"

	"github.com/google/uuid"
)

func TestParseUUID_ValidUUID(t *testing.T) {
	want := uuid.New()

	got, errc := ParseUUID(want.String())
	if errc != nil {
		t.Fatalf("ParseUUID: %v", errc)
	}
	if got != want {
		t.Errorf("ParseUUID() = %v, want %v", got, want)
	}
}

func TestParseUUID_InvalidUUIDReturnsBadRequest(t *testing.T) {
	cases := []string{"", "not-a-uuid", "12345", "  "}
	for _, s := range cases {
		_, errc := ParseUUID(s)
		if errc == nil {
			t.Errorf("ParseUUID(%q): expected error, got nil", s)
			continue
		}
		if errc.Code != errcode.ErrBadRequest.Code {
			t.Errorf("ParseUUID(%q): errc.Code = %d, want %d", s, errc.Code, errcode.ErrBadRequest.Code)
		}
	}
}

func TestParseUUIDPair_BothValid(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	gotA, gotB, errc := ParseUUIDPair(a.String(), b.String())
	if errc != nil {
		t.Fatalf("ParseUUIDPair: %v", errc)
	}
	if gotA != a || gotB != b {
		t.Errorf("ParseUUIDPair() = (%v, %v), want (%v, %v)", gotA, gotB, a, b)
	}
}

func TestParseUUIDPair_FirstInvalidShortCircuits(t *testing.T) {
	_, _, errc := ParseUUIDPair("not-a-uuid", uuid.New().String())
	if errc == nil {
		t.Fatal("expected error when first UUID is invalid")
	}
}

func TestParseUUIDPair_SecondInvalidReturnsError(t *testing.T) {
	_, _, errc := ParseUUIDPair(uuid.New().String(), "not-a-uuid")
	if errc == nil {
		t.Fatal("expected error when second UUID is invalid")
	}
}

// TestCanonicalUserID 验证各种合法 UUID 写法都归一到同一个小写带连字符的形式，
// 非 UUID（历史数据、测试里的 "user-1"）原样返回，不会被改写成另一个键名。
func TestCanonicalUserID(t *testing.T) {
	id := uuid.New()
	canonical := id.String()
	upper := strings.ToUpper(canonical)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"canonical", canonical, canonical},
		{"uppercase", upper, canonical},
		{"braced", "{" + canonical + "}", canonical},
		{"braced uppercase", "{" + upper + "}", canonical},
		{"urn", "urn:uuid:" + canonical, canonical},
		{"urn uppercase", "urn:uuid:" + upper, canonical},
		{"hex without hyphens", strings.ReplaceAll(canonical, "-", ""), canonical},
		{"non uuid passthrough", "user-1", "user-1"},
		{"empty passthrough", "", ""},
		{"garbage passthrough", "not-a-uuid", "not-a-uuid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalUserID(tt.in); got != tt.want {
				t.Errorf("CanonicalUserID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
