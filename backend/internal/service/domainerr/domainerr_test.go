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

// TestTypedNilIsSafe typed-nil 是 Go 接口的经典陷阱：err != nil 成立、
// errors.As 也返回 true，但取到的指针是 nil。这里钉住「不 panic、不误报 Code」。
func TestTypedNilIsSafe(t *testing.T) {
	var typedNil *Error
	// 故意制造 typed-nil：接口本身非 nil，但里面装的是 nil 指针。
	var err error = typedNil

	if code, ok := CodeOf(err); ok {
		t.Fatalf("CodeOf(typed nil) = %q, true; want false（不能把 typed-nil 当成有效领域错误）", code)
	}
	if IsCode(err, CodeInternal) {
		t.Error("IsCode(typed nil, CodeInternal) = true, want false")
	}
	if got := typedNil.Error(); got != "<nil>" {
		t.Fatalf("(*Error)(nil).Error() = %q, want %q", got, "<nil>")
	}

	// 包装链里也含 typed-nil 时同样不能 panic。
	wrapped := fmt.Errorf("handler: %w", err)
	if _, ok := CodeOf(wrapped); ok {
		t.Error("CodeOf(wrapped typed nil) reported a domain code")
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
