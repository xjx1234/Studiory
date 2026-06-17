// Package sender 提供验证码下发的抽象与多服务商路由。
//
// 设计：
//   - Provider 是单个下发服务商（短信：阿里云/腾讯云/……；邮件：SMTP 等）。
//   - Router 按渠道选择 Provider，并支持同一渠道注册多个 Provider 做顺序故障转移
//     （主通道失败自动切备用通道）——即“接入多家运营商”的落点。
//   - 业务层（auth service）只依赖 Sender 接口，不关心具体服务商。
package sender

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// Channel 表示验证码下发渠道。
type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
)

// AllChannels 列出全部受支持渠道，便于注册时遍历。
var AllChannels = []Channel{ChannelSMS, ChannelEmail}

// ErrNoProvider 表示该渠道未注册任何可用 Provider。
var ErrNoProvider = errors.New("sender: no provider for channel")

// Message 是一条验证码下发请求。
type Message struct {
	Channel Channel
	Target  string // 手机号或邮箱
	Code    string
}

// Provider 是单个下发服务商。
type Provider interface {
	// Name 返回服务商标识（用于日志/指标）。
	Name() string
	// Supports 返回该 Provider 是否能处理指定渠道。
	Supports(ch Channel) bool
	// Send 下发一条验证码消息。
	Send(ctx context.Context, msg Message) error
}

// Sender 是对外统一入口，业务层依赖它。
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Router 按渠道路由到 Provider，同渠道多 Provider 时顺序故障转移。
type Router struct {
	byChannel map[Channel][]Provider
	logger    *zap.Logger
}

// NewRouter 按 providers 声明的 Supports 渠道建立路由表。
// 同一渠道内 Provider 的顺序即故障转移优先级（先注册者优先）。
func NewRouter(logger *zap.Logger, providers ...Provider) *Router {
	if logger == nil {
		logger = zap.NewNop()
	}
	r := &Router{
		byChannel: make(map[Channel][]Provider),
		logger:    logger,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		for _, ch := range AllChannels {
			if p.Supports(ch) {
				r.byChannel[ch] = append(r.byChannel[ch], p)
			}
		}
	}
	return r
}

// Send 选择对应渠道的 Provider 依次尝试，任一成功即返回；全部失败返回最后一个错误。
func (r *Router) Send(ctx context.Context, msg Message) error {
	providers := r.byChannel[msg.Channel]
	if len(providers) == 0 {
		return fmt.Errorf("%w: %s", ErrNoProvider, msg.Channel)
	}

	var lastErr error
	for _, p := range providers {
		if err := p.Send(ctx, msg); err != nil {
			lastErr = err
			r.logger.Warn("code provider send failed, trying next",
				zap.String("provider", p.Name()),
				zap.String("channel", string(msg.Channel)),
				zap.Error(err),
			)
			continue
		}
		return nil
	}
	return fmt.Errorf("all providers failed for channel %s: %w", msg.Channel, lastErr)
}
