package http

import (
	"errors"
	"fmt"
	"testing"

	"backend/internal/service/domainerr"
	"backend/pkg/errcode"
)

// TestDomainErrcodeMapping 遍历 domainerr.AllCodes：新增 Code 时只要忘了在
// internal/http/errors.go 里配映射，或在下面漏登记期望值，这条测试就会失败。
func TestDomainErrcodeMapping(t *testing.T) {
	want := map[domainerr.Code]*errcode.Error{
		domainerr.CodeInvalidArgument: errcode.ErrBadRequest,
		domainerr.CodeValidation:      errcode.ErrValidation,
		domainerr.CodeUnauthenticated: errcode.ErrUnauthorized,
		domainerr.CodeForbidden:       errcode.ErrForbidden,
		domainerr.CodeNotFound:        errcode.ErrNotFound,
		domainerr.CodeAlreadyExists:   errcode.ErrAlreadyExists,
		domainerr.CodeTooManyRequests: errcode.ErrTooManyRequests,
		domainerr.CodeUnavailable:     errcode.ErrServiceUnavailable,
		domainerr.CodeInternal:        errcode.ErrInternal,
	}

	covered := make(map[domainerr.Code]bool, len(want))
	for _, code := range domainerr.AllCodes {
		covered[code] = true
		t.Run(string(code), func(t *testing.T) {
			expected, ok := want[code]
			if !ok {
				t.Fatalf("domainerr.AllCodes 里有 %q，但映射测试没有登记期望值："+
					"先在 internal/http/errors.go 加 case，再在这里补期望", code)
			}
			if got := domainErrcode(domainerr.New(code, "detail only for logs")); got != expected {
				t.Fatalf("domainErrcode(%q) = %+v, want %+v", code, got, expected)
			}
		})
	}

	// 反向检查：测试里登记的每个 Code 都必须真的在 AllCodes 中，防止清单漏项。
	for code := range want {
		if !covered[code] {
			t.Fatalf("映射测试登记了 %q，但它不在 domainerr.AllCodes 里：请同步补齐清单", code)
		}
	}
}

func TestDomainErrcodeFallsBackToInternal(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"plain error", errors.New("db down")},
		{"unmapped code", domainerr.New(domainerr.Code("not_mapped"), "")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domainErrcode(tc.err); got != errcode.ErrInternal {
				t.Fatalf("domainErrcode(%v) = %+v, want ErrInternal", tc.err, got)
			}
		})
	}
}

func TestDomainErrcodeThroughWrap(t *testing.T) {
	err := fmt.Errorf("handler: %w", domainerr.New(domainerr.CodeNotFound, "todo not found"))
	if got := domainErrcode(err); got != errcode.ErrNotFound {
		t.Fatalf("domainErrcode(wrapped) = %+v, want ErrNotFound", got)
	}
}
