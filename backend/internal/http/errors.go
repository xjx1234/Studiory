package http

import (
	"backend/internal/service/domainerr"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

// domainErrcode 把 service 层领域错误映射为对外业务错误。
//
// 未识别的错误（非 domainerr，或新增 Code 漏配）一律 500：不泄漏内部细节，
// 并让漏配通过 errors_test.go 显性失败。
func domainErrcode(err error) *errcode.Error {
	code, ok := domainerr.CodeOf(err)
	if !ok {
		return errcode.ErrInternal
	}

	switch code {
	case domainerr.CodeInvalidArgument:
		return errcode.ErrBadRequest
	case domainerr.CodeValidation:
		return errcode.ErrValidation
	case domainerr.CodeUnauthenticated:
		return errcode.ErrUnauthorized
	case domainerr.CodeForbidden:
		return errcode.ErrForbidden
	case domainerr.CodeNotFound:
		return errcode.ErrNotFound
	case domainerr.CodeAlreadyExists:
		return errcode.ErrAlreadyExists
	case domainerr.CodeTooManyRequests:
		return errcode.ErrTooManyRequests
	case domainerr.CodeUnavailable:
		return errcode.ErrServiceUnavailable
	default:
		return errcode.ErrInternal
	}
}

// failDomain 是已迁移模块的统一错误出口。
// resp.Fail 仍负责请求超时改写 503 与 i18n 翻译。
func failDomain(c *gin.Context, err error) {
	resp.Fail(c, domainErrcode(err))
}
