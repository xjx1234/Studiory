package errcode

import "net/http"

// Error 表示一个业务错误，包含错误码、i18n 消息 ID、HTTP 状态码。
type Error struct {
	Code       int
	MsgID      string
	HTTPStatus int
}

// WithMessage 返回一个携带自定义消息 ID 的新 Error（不改动原始定义）。
func (e *Error) WithMessage(msgID string) *Error {
	return &Error{Code: e.Code, MsgID: msgID, HTTPStatus: e.HTTPStatus}
}

// 错误码分段规则：
//   0       = 成功
//   1xxxx   = 认证 / 权限相关
//   2xxxx   = 请求参数 / 校验相关
//   3xxxx   = 业务资源相关
//   5xxxx   = 服务器内部错误

var (
	// ── 成功 ──────────────────────────────────────────────────────────────────
	OK = &Error{Code: 0, MsgID: "success", HTTPStatus: http.StatusOK}

	// ── 认证 / 权限  1xxxx ────────────────────────────────────────────────────
	ErrUnauthorized       = &Error{Code: 10001, MsgID: "err_unauthorized", HTTPStatus: http.StatusUnauthorized}
	ErrInvalidToken       = &Error{Code: 10002, MsgID: "err_invalid_token", HTTPStatus: http.StatusUnauthorized}
	ErrTokenExpired       = &Error{Code: 10003, MsgID: "err_token_expired", HTTPStatus: http.StatusUnauthorized}
	ErrInvalidCredentials = &Error{Code: 10004, MsgID: "err_invalid_credentials", HTTPStatus: http.StatusUnauthorized}
	ErrInvalidCode        = &Error{Code: 10005, MsgID: "err_invalid_code", HTTPStatus: http.StatusBadRequest}
	ErrUnsupportedGrant   = &Error{Code: 10006, MsgID: "err_unsupported_grant", HTTPStatus: http.StatusBadRequest}
	ErrForbidden          = &Error{Code: 10007, MsgID: "err_forbidden", HTTPStatus: http.StatusForbidden}

	// ── 请求 / 参数  2xxxx ────────────────────────────────────────────────────
	ErrBadRequest      = &Error{Code: 20001, MsgID: "err_bad_request", HTTPStatus: http.StatusBadRequest}
	ErrValidation      = &Error{Code: 20002, MsgID: "err_validation", HTTPStatus: http.StatusBadRequest}
	ErrTooManyRequests = &Error{Code: 20003, MsgID: "err_too_many_requests", HTTPStatus: http.StatusTooManyRequests}

	// ── 业务资源  3xxxx ───────────────────────────────────────────────────────
	ErrNotFound      = &Error{Code: 30001, MsgID: "err_not_found", HTTPStatus: http.StatusNotFound}
	ErrAlreadyExists = &Error{Code: 30002, MsgID: "err_already_exists", HTTPStatus: http.StatusConflict}

	// ── 服务器内部  5xxxx ─────────────────────────────────────────────────────
	ErrInternal           = &Error{Code: 50001, MsgID: "err_internal", HTTPStatus: http.StatusInternalServerError}
	ErrServiceUnavailable = &Error{Code: 50002, MsgID: "err_service_unavailable", HTTPStatus: http.StatusServiceUnavailable}
)
