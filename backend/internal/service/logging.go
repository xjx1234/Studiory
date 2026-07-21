package service

import (
	"backend/pkg/strutil"

	"go.uber.org/zap"
)

// LogSupport 提供 service 层通用内部错误日志能力。
// 各业务 service 可匿名嵌入该结构体，避免重复实现 logger 字段和 LogInternal。
type LogSupport struct {
	Logger *zap.Logger
}

func (l *LogSupport) SetLogger(logger *zap.Logger) {
	l.Logger = logger
}

func (l *LogSupport) LogInternal(op string, err error, fields ...zap.Field) {
	if l.Logger != nil && err != nil {
		fields = append(fields, zap.Error(err))
		l.Logger.Error(op, fields...)
	}
}

// UserIDField 记录关联用户 ID，便于与 HTTP 访问日志串联；空字符串时省略。
func UserIDField(userID string) zap.Field {
	if userID == "" {
		return zap.Skip()
	}
	return zap.String("user_id", userID)
}

// ActorUserIDField 记录操作者用户 ID（如 admin 改他人角色）；空字符串时省略。
func ActorUserIDField(userID string) zap.Field {
	if userID == "" {
		return zap.Skip()
	}
	return zap.String("actor_user_id", userID)
}

// TargetField 记录登录/验证码等尚未关联 user_id 时的账号标识（手机号、邮箱等）。
// 自动对 PII 脱敏，防止手机号/邮箱明文写入日志。
func TargetField(target string) zap.Field {
	if target == "" {
		return zap.Skip()
	}
	return zap.String("target", strutil.MaskPII(target))
}
