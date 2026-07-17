package service

import (
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
