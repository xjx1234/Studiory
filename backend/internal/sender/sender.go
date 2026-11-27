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
	"time"

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

// Channels 返回当前至少有一个 Provider 的渠道（按 AllChannels 顺序）。
// 启动阶段用它推导「实际可用渠道」，避免把未接运营商的渠道暴露给调用方。
func (r *Router) Channels() []Channel {
	out := make([]Channel, 0, len(r.byChannel))
	for _, ch := range AllChannels {
		if len(r.byChannel[ch]) > 0 {
			out = append(out, ch)
		}
	}
	return out
}

// Send 选择对应渠道的 Provider 依次尝试，任一成功即返回；全部失败返回最后一个错误。
//
// 同渠道多个 Provider 共用调用方的 deadline，因此每次尝试只切「剩余预算 ÷ 剩余尝试数」：
// 否则主通道把时间耗光后，备用通道拿到的是已过期的 ctx，故障转移形同虚设
// （线上表现为「配了备用 SMTP 却从来没救回来过」）。主通道快速失败时，
// 省下的时间会自动让给后面的通道。
// 不真实外呼的 Provider（如开发用 mock）通过 BudgetNeutral 声明不参与切分，
// 否则「SMTP + mock」的组合会让真实 SMTP 只能拿到一半预算。
func (r *Router) Send(ctx context.Context, msg Message) error {
	providers := r.byChannel[msg.Channel]
	if len(providers) == 0 {
		return fmt.Errorf("%w: %s", ErrNoProvider, msg.Channel)
	}

	var lastErr error
	for i, p := range providers {
		attemptCtx, cancel := attemptBudget(ctx, budgetedAttempts(providers[i:]))
		if attemptCtx == nil {
			// 上下文已结束（请求超时/客户端断开）或剩余预算已耗尽：剩下的 provider 一个都不会被尝试。
			// 不能静默当作「和上次一样失败」，否则排查时会以为备用通道也试过。
			return fmt.Errorf("sender: no time budget left for channel %s, %d of %d provider(s) not attempted (request context done or deadline exhausted; last error: %v)",
				msg.Channel, len(providers)-i, len(providers), lastErr)
		}

		err := p.Send(attemptCtx, msg)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		r.logger.Warn("code provider send failed, trying next",
			zap.String("provider", p.Name()),
			zap.String("channel", string(msg.Channel)),
			zap.Error(err),
		)
	}
	return fmt.Errorf("all providers failed for channel %s: %w", msg.Channel, lastErr)
}

// budgetNeutral 由「不真实外呼、不需要时间预算」的 Provider 实现（如开发用 mock）。
type budgetNeutral interface {
	BudgetNeutral() bool
}

// budgetedAttempts 统计还需要真实占用预算的尝试数（调用方传入的是「当前及之后的 Provider」）。
// 只剩 mock 时返回 1，避免除零，也让它沿用剩余预算即可。
func budgetedAttempts(rest []Provider) int {
	n := 0
	for _, p := range rest {
		if bn, ok := p.(budgetNeutral); ok && bn.BudgetNeutral() {
			continue
		}
		n++
	}
	if n == 0 {
		return 1
	}
	return n
}

// attemptBudget 为单次尝试派生上下文：调用方没有 deadline 时原样透传（不在这里发明超时），
// 有 deadline 时按剩余尝试数均分剩余时间；上下文已结束则返回 nil ctx 表示不该继续尝试。
func attemptBudget(ctx context.Context, attemptsLeft int) (context.Context, context.CancelFunc) {
	noop := func() {}
	if ctx.Err() != nil {
		return nil, noop
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx, noop
	}
	if share := time.Until(deadline) / time.Duration(attemptsLeft); share > 0 {
		return context.WithTimeout(ctx, share)
	}
	return nil, noop
}
