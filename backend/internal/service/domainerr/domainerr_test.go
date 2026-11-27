package domainerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeOfAndIsCode(t *testing.T) {
	base := New(CodeNotFound, "todo not found")

	if code, ok := CodeOf(base); !ok || code != CodeNotFound {
		t.Fatalf("CodeOf(base) = %q, %v; want %q, true", code, ok, CodeNotFound)
	}
	if !IsCode(base, CodeNotFound) {
		t.Error("IsCode(base, CodeNotFound) = false, want true")
	}
	if IsCode(base, CodeInternal) {
		t.Error("IsCode(base, CodeInternal) = true, want false")
	}

	wrapped := fmt.Errorf("handler: %w", base)
	if code, ok := CodeOf(wrapped); !ok || code != CodeNotFound {
		t.Fatalf("CodeOf(wrapped) = %q, %v; want %q, true", code, ok, CodeNotFound)
	}
	if !IsCode(wrapped, CodeNotFound) {
		t.Error("IsCode(wrapped, CodeNotFound) = false, want true")
	}

	if _, ok := CodeOf(errors.New("plain")); ok {
		t.Error("CodeOf(plain error) reported a domain code")
	}
	if _, ok := CodeOf(nil); ok {
		t.Error("CodeOf(nil) reported a domain code")
	}
}

func TestErrorString(t *testing.T) {
	cause := errors.New("db down")
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{"code only", New(CodeValidation, ""), "validation"},
		{"with msg", New(CodeValidation, "title required"), "validation: title required"},
		{"with cause", Wrap(CodeInternal, "", cause), "internal: db down"},
		{"with msg and cause", Wrap(CodeInternal, "list todos", cause), "internal: list todos: db down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWrapPreservesCause(t *testing.T) {
	cause := errors.New("db down")
	err := Wrap(CodeInternal, "list todos", cause)
	if !errors.Is(err, cause) {
		t.Fatal("wrapped domain error does not preserve cause")
	}
}
