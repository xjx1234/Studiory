package service

import "go.uber.org/zap"

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
