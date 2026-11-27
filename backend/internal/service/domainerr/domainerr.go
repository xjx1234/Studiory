// Package domainerr 定义与 HTTP / i18n 解耦的领域错误。
//
// service 层只返回 Code（可附带仅用于日志的 Msg 与 cause），不再构造 errcode.Error：
// Code 到 HTTP 状态码与 i18n 消息的映射集中在 internal/http/errors.go。
// 迁移按模块推进：已迁移模块的 Service 接口返回 error，handler 用 failDomain；
// 未迁移模块继续返回 *errcode.Error，两者在过渡期共存。
//
// 新增 Code 时必须同步更新 http 层映射；漏配会退化为 500，并由 errors_test.go 兜底。
package domainerr

import (
	"errors"
	"fmt"
)

// Code 是稳定的、协议无关的领域错误码，只描述「发生了哪类错误」，
// 不携带 HTTP 状态码或 i18n 文案 ID。
type Code string

const (
	CodeInvalidArgument Code = "invalid_argument"
	CodeValidation      Code = "validation"
	CodeUnauthenticated Code = "unauthenticated"
	CodeForbidden       Code = "forbidden"
	CodeNotFound        Code = "not_found"
	CodeAlreadyExists   Code = "already_exists"
	CodeTooManyRequests Code = "too_many_requests"
	CodeUnavailable     Code = "unavailable"
	CodeInternal        Code = "internal"
)

// AllCodes 列出全部领域错误码，供 HTTP 映射测试遍历。
//
// 新增 Code 时必须同时追加到这里。否则映射测试不会覆盖它，而 internal/http/errors.go
// 里漏配的分支会静默退化成 500——只有遍历清单才能把「漏配」变成红测试。
var AllCodes = []Code{
	CodeInvalidArgument,
	CodeValidation,
	CodeUnauthenticated,
	CodeForbidden,
	CodeNotFound,
	CodeAlreadyExists,
	CodeTooManyRequests,
	CodeUnavailable,
	CodeInternal,
}

// Error 是领域错误。Msg 与 Err 只用于日志/调试；
// HTTP 适配层只读取 Code，不会把 Msg 或 cause 写进响应。
type Error struct {
	Code Code
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		// typed-nil（var e *Error; var err error = e）在日志里很常见：
		// 直接解引用会 panic，把一次可解释的 500 变成进程级事故。
		return "<nil>"
	}
	switch {
	case e.Msg != "" && e.Err != nil:
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Msg, e.Err)
	case e.Msg != "":
		return fmt.Sprintf("%s: %s", e.Code, e.Msg)
	case e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	default:
		return string(e.Code)
	}
}

func (e *Error) Unwrap() error { return e.Err }

// New 构造领域错误；msg 只用于日志，不参与对外文案。
func New(code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

// Wrap 在领域错误上保留底层 cause，便于日志与 errors.Is 追溯。
func Wrap(code Code, msg string, err error) *Error {
	return &Error{Code: code, Msg: msg, Err: err}
}

// CodeOf 沿 %w 链查找领域错误码；非领域错误返回 false。
//
// de != nil 的判断不能省：接口里装着 typed-nil（var e *Error; err = e）时 err != nil
// 成立、errors.As 也返回 true，但 de 是 nil；不判空就会 nil pointer dereference。
func CodeOf(err error) (Code, bool) {
	var de *Error
	if errors.As(err, &de) && de != nil {
		return de.Code, true
	}
	return "", false
}

// IsCode 判断错误链上是否存在指定领域错误码。
func IsCode(err error, code Code) bool {
	got, ok := CodeOf(err)
	return ok && got == code
}
