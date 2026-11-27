package http

import (
	"net/http"

	"backend/internal/service/domainerr"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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
//
// 5xx 额外记一条带 request_id 的错误日志：domainerr 的 Msg/cause 只用于日志，
// 不写进响应；若这里不落日志，未映射的 Code 或非 service 来源的内部错误会只剩一个 500。
func failDomain(c *gin.Context, err error) {
	e := domainErrcode(err)
	if e.HTTPStatus >= http.StatusInternalServerError {
		fields := make([]zap.Field, 0, 5)
		if c.Request != nil {
			fields = append(fields,
				zap.String("request_id", requestid.Get(c)),
				zap.String("method", c.Request.Method),
				zap.String("path", c.FullPath()),
			)
		}
		fields = append(fields, zap.Int("biz_code", e.Code), zap.Error(err))
		zap.L().Error("请求以 5xx 结束：domainerr 未映射或内部错误", fields...)
	}
	resp.Fail(c, e)
}
