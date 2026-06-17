package sender

import (
	"context"

	"go.uber.org/zap"
)

// MockProvider 不真正发送，只把验证码打到日志，供开发/测试使用。
// 它支持所有渠道，因此可作为开发环境的默认 Provider。
type MockProvider struct {
	logger *zap.Logger
}

// NewMockProvider 创建一个记录日志的 mock Provider。
func NewMockProvider(logger *zap.Logger) *MockProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MockProvider{logger: logger}
}

func (p *MockProvider) Name() string { return "mock" }

func (p *MockProvider) Supports(_ Channel) bool { return true }

func (p *MockProvider) Send(_ context.Context, msg Message) error {
	p.logger.Info("[mock sender] 验证码下发（仅日志，不真实发送）",
		zap.String("channel", string(msg.Channel)),
		zap.String("target", msg.Target),
		zap.String("code", msg.Code),
	)
	return nil
}
