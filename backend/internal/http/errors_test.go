package http

import (
	"errors"
	"fmt"
	"testing"

	"backend/internal/service/domainerr"
	"backend/pkg/errcode"
)

func TestDomainErrcodeMapping(t *testing.T) {
	cases := []struct {
		code domainerr.Code
		want *errcode.Error
	}{
		{domainerr.CodeInvalidArgument, errcode.ErrBadRequest},
		{domainerr.CodeValidation, errcode.ErrValidation},
		{domainerr.CodeUnauthenticated, errcode.ErrUnauthorized},
		{domainerr.CodeForbidden, errcode.ErrForbidden},
		{domainerr.CodeNotFound, errcode.ErrNotFound},
		{domainerr.CodeAlreadyExists, errcode.ErrAlreadyExists},
		{domainerr.CodeTooManyRequests, errcode.ErrTooManyRequests},
		{domainerr.CodeUnavailable, errcode.ErrServiceUnavailable},
		{domainerr.CodeInternal, errcode.ErrInternal},
	}

	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			got := domainErrcode(domainerr.New(tc.code, "detail only for logs"))
			if got != tc.want {
				t.Fatalf("domainErrcode(%q) = %+v, want %+v", tc.code, got, tc.want)
			}
		})
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
