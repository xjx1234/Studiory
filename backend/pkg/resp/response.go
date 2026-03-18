// Package resp 提供统一的 HTTP 响应结构与辅助函数。
//
// 响应格式：
//
//	{
//	  "code":       0,           // 0=成功，非 0=业务错误码
//	  "message":    "成功",      // i18n 翻译后的消息
//	  "data":       { ... },     // 业务数据，错误时为 null
//	  "request_id": "xxx"        // 来自 X-Request-Id 请求头
//	}
package resp

import (
	"net/http"

	"backend/pkg/errcode"
	pkgi18n "backend/pkg/i18n"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

// Response 是统一响应体结构。
type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

// OK 返回成功响应，HTTP 状态码 200。
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:      0,
		Message:   pkgi18n.Localize(c, "success"),
		Data:      data,
		RequestID: requestid.Get(c),
	})
}

// Fail 返回业务错误响应，HTTP 状态码由 errcode.Error.HTTPStatus 决定，
// 并终止后续 Handler（AbortWithStatusJSON）。
func Fail(c *gin.Context, err *errcode.Error) {
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusBadRequest
	}
	c.AbortWithStatusJSON(status, Response{
		Code:      err.Code,
		Message:   pkgi18n.Localize(c, err.MsgID),
		Data:      nil,
		RequestID: requestid.Get(c),
	})
}

// FailWithMessage 返回带自定义文本消息的错误响应（不经过 i18n 翻译）。
// 适合在业务层直接传入已格式化好的错误描述。
func FailWithMessage(c *gin.Context, err *errcode.Error, msg string) {
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusBadRequest
	}
	c.AbortWithStatusJSON(status, Response{
		Code:      err.Code,
		Message:   msg,
		Data:      nil,
		RequestID: requestid.Get(c),
	})
}

// FailWithValidation 返回参数校验失败响应，data.fields 包含每个字段的错误详情。
//
// 响应示例：
//
//	{
//	  "code": 20002,
//	  "message": "参数校验失败",
//	  "data": { "fields": { "phone": "phone 必须是有效的手机号" } },
//	  "request_id": "xxx"
//	}
func FailWithValidation(c *gin.Context, fields map[string]string) {
	c.AbortWithStatusJSON(http.StatusBadRequest, Response{
		Code:      errcode.ErrValidation.Code,
		Message:   pkgi18n.Localize(c, errcode.ErrValidation.MsgID),
		Data:      gin.H{"fields": fields},
		RequestID: requestid.Get(c),
	})
}
