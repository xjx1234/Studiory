package authservice

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"backend/internal/sender"
	baseservice "backend/internal/service"
	"backend/pkg/errcode"
	"backend/pkg/strutil"

	"go.uber.org/zap"
)

const (
	codeExpiry       = 5 * time.Minute
	codeSendCooldown = 60 * time.Second
	// codeQuotaWindow 是下发配额的统计窗口（滚动窗口，从首次下发开始计时）。
	codeQuotaWindow = 24 * time.Hour

	// verifyCodeLua 原子校验验证码：匹配则删除并返回 1；key 不存在返回 -1；不匹配返回 0。
	verifyCodeLua = `
local val = redis.call('GET', KEYS[1])
if not val then
    return -1
end
if val == ARGV[1] then
    redis.call('DEL', KEYS[1])
    return 1
end
return 0
`

	// sendCodeQuotaLua 原子完成「目标配额 → IP 配额 → 冷却 → 计数」四步，避免并发请求
	// 同时通过检查导致配额被击穿，也避免先占冷却再因配额拒绝而白占 60 秒。
	// KEYS: 1=目标配额键 2=IP 配额键 3=冷却键
	// ARGV: 1=目标上限 2=IP 上限 3=配额窗口(ms) 4=冷却窗口(ms)；上限 < 0 表示跳过该维度
	// 返回: >=1 成功；-1 目标配额用尽；-2 IP 配额用尽；-3 冷却中
	sendCodeQuotaLua = `
local targetLimit = tonumber(ARGV[1])
if targetLimit >= 0 then
    local used = tonumber(redis.call('GET', KEYS[1]) or '0')
    if used >= targetLimit then
        return -1
    end
end
local ipLimit = tonumber(ARGV[2])
if ipLimit >= 0 then
    local used = tonumber(redis.call('GET', KEYS[2]) or '0')
    if used >= ipLimit then
        return -2
    end
end
if not redis.call('SET', KEYS[3], '1', 'NX', 'PX', ARGV[4]) then
    return -3
end
if targetLimit >= 0 then
    local n = redis.call('INCR', KEYS[1])
    if n == 1 then
        redis.call('PEXPIRE', KEYS[1], ARGV[3])
    end
end
if ipLimit >= 0 then
    local n = redis.call('INCR', KEYS[2])
    if n == 1 then
        redis.call('PEXPIRE', KEYS[2], ARGV[3])
    end
end
return 1
`
)

// 下发结果标签（指标 auth_code_send_total 的 outcome 维度，必须是有限集合）。
const (
	codeOutcomeSent               = "sent"
	codeOutcomeCooldown           = "cooldown"
	codeOutcomeTargetQuota        = "target_quota"
	codeOutcomeIPQuota            = "ip_quota"
	codeOutcomeUnsupportedChannel = "unsupported_channel"
	codeOutcomeInvalidRequest     = "invalid_request"
	codeOutcomeStorageError       = "storage_error"
	codeOutcomeDispatchError      = "dispatch_error"
)

// SendCode 生成验证码、写入 Redis，并通过下发器（多服务商路由）发送。
//
// 流程：目标/IP 日配额 → 冷却限频 → 生成验证码 → 写 Redis → 经 codeSender 下发。
// 配额与冷却在同一个 Lua 脚本里原子判定；下发失败时清除冷却与验证码，允许用户立即重试
// （配额已计入，避免用「失败重试」绕过上限）。
func (s *AuthServiceImpl) SendCode(ctx context.Context, codeType, target, clientIP string) *errcode.Error {
	codeType = strings.ToLower(strings.TrimSpace(codeType))
	// 先归一化再判空与配额：键、下发、计数必须共用同一个标识形式。
	target = canonicalTarget(codeType, target)

	if codeType == "" || target == "" {
		s.observeCodeSend(codeType, codeOutcomeInvalidRequest)
		return errcode.ErrBadRequest
	}
	if !s.codeChannelAllowed(codeType) {
		s.observeCodeSend(codeType, codeOutcomeUnsupportedChannel)
		return errcode.ErrBadRequest.WithMessage("err_code_channel_unavailable")
	}

	targetLimit := int64(s.codeDailyLimitPerTarget)
	if targetLimit <= 0 {
		targetLimit = -1 // 关闭该维度
	}
	ipLimit := int64(s.codeDailyLimitPerIP)
	if ipLimit <= 0 || clientIP == "" {
		ipLimit = -1
	}

	result, err := s.cache.Eval(ctx, sendCodeQuotaLua,
		[]string{
			s.codeQuotaKey(codeType, target),
			s.codeIPQuotaKey(clientIP),
			s.codeCooldownKey(codeType, target),
		},
		targetLimit, ipLimit,
		int64(codeQuotaWindow/time.Millisecond),
		int64(codeSendCooldown/time.Millisecond),
	)
	if err != nil {
		s.observeCodeSend(codeType, codeOutcomeStorageError)
		s.LogInternal(ctx, "SendCode quota check", err,
			zap.String("code_type", codeType),
			baseservice.TargetField(target),
		)
		return errcode.ErrInternal
	}

	switch result {
	case int64(-1):
		s.observeCodeSend(codeType, codeOutcomeTargetQuota)
		return errcode.ErrTooManyRequests
	case int64(-2):
		s.observeCodeSend(codeType, codeOutcomeIPQuota)
		return errcode.ErrTooManyRequests
	case int64(-3):
		s.observeCodeSend(codeType, codeOutcomeCooldown)
		return errcode.ErrTooManyRequests
	}

	cooldownKey := s.codeCooldownKey(codeType, target)
	code, codeErr := s.generateCode(ctx)
	if codeErr != nil {
		s.observeCodeSend(codeType, codeOutcomeStorageError)
		_ = s.cache.Del(ctx, cooldownKey)
		return codeErr
	}

	key := s.codeRedisKey(codeType, target)
	if err := s.cache.Set(ctx, key, code, codeExpiry); err != nil {
		s.observeCodeSend(codeType, codeOutcomeStorageError)
		s.LogInternal(ctx, "SendCode set code", err,
			zap.String("code_type", codeType),
			baseservice.TargetField(target),
		)
		_ = s.cache.Del(ctx, cooldownKey)
		return errcode.ErrInternal
	}

	if s.codeSender != nil {
		if err := s.codeSender.Send(ctx, sender.Message{
			Channel: sender.Channel(codeType),
			Target:  target,
			Code:    code,
		}); err != nil {
			s.observeCodeSend(codeType, codeOutcomeDispatchError)
			s.LogInternal(ctx, "SendCode dispatch", err,
				zap.String("code_type", codeType),
				baseservice.TargetField(target),
			)
			// 下发失败：回滚冷却与验证码，便于用户重试
			_ = s.cache.Del(ctx, cooldownKey, key)
			return errcode.ErrInternal
		}
	}

	s.observeCodeSend(codeType, codeOutcomeSent)
	return nil
}

func (s *AuthServiceImpl) observeCodeSend(channel, outcome string) {
	if s.codeObserver != nil {
		s.codeObserver.ObserveCodeSend(codeChannelLabel(channel), outcome)
	}
}

// codeChannelLabel 把渠道标签收敛到有限集合。
// codeType 来自请求体，在渠道校验之前就可能被记录（invalid_request / unsupported_channel），
// 直接当标签会引入基数无上限的指标；与 HTTP method 归一化同理。
func codeChannelLabel(channel string) string {
	switch channel {
	case string(sender.ChannelSMS), string(sender.ChannelEmail):
		return channel
	default:
		return "unknown"
	}
}

func (s *AuthServiceImpl) codeChannelAllowed(codeType string) bool {
	if s.codeChannels == nil {
		return true // 未注入白名单（测试/兼容），保持旧行为
	}
	_, ok := s.codeChannels[codeType]
	return ok
}

// generateCode 生成验证码。
// 开发模式（allowMockCodeFallback）下返回固定的 MockVerificationCode 便于联调；
// 否则生成 6 位随机数字验证码。crypto/rand 失败时返回错误，不回退固定码。
func (s *AuthServiceImpl) generateCode(ctx context.Context) (string, *errcode.Error) {
	if s.allowMockCodeFallback {
		return MockVerificationCode, nil
	}
	const digits = "0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			// crypto/rand 失败极罕见，但必须拒绝生成而非回退固定码
			s.LogInternal(ctx, "generateCode rand", err)
			return "", errcode.ErrInternal
		}
		b[i] = digits[n.Int64()]
	}
	return string(b), nil
}

// verifyCode 校验验证码。
// 策略：先查 Redis，key 不存在时在开发阶段允许 MockVerificationCode 通过（便于本地无 Redis 调试）。
// 验证成功后删除 key，确保一次有效。
func (s *AuthServiceImpl) verifyCode(ctx context.Context, codeType, target, code string) bool {
	key := s.codeRedisKey(codeType, target)
	result, err := s.cache.Eval(ctx, verifyCodeLua, []string{key}, code)
	if err != nil {
		return false
	}

	count, ok := result.(int64)
	if !ok {
		return false
	}

	switch count {
	case 1:
		return true
	case -1:
		return s.allowMockCodeFallback && code == MockVerificationCode
	default:
		return false
	}
}

func (s *AuthServiceImpl) codeRedisKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:%s", s.codePrefix, codeType, canonicalTarget(codeType, target))
}

func (s *AuthServiceImpl) codeCooldownKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:cooldown:%s", s.codePrefix, codeType, canonicalTarget(codeType, target))
}

// codeQuotaKey 是「按目标」的 24 小时下发计数器。
func (s *AuthServiceImpl) codeQuotaKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:quota:%s", s.codePrefix, codeType, canonicalTarget(codeType, target))
}

// codeIPQuotaKey 是「按 IP」的 24 小时下发计数器，所有渠道共用（防止换渠道绕过）。
func (s *AuthServiceImpl) codeIPQuotaKey(clientIP string) string {
	return fmt.Sprintf("%s:code:quota:ip:%s", s.codePrefix, clientIP)
}

// canonicalTarget 把手机号/邮箱收敛成唯一标识形式，是所有验证码键与账号查找的必经之路。
//
// 不归一化时 "Alice@x.com" 与 "alice@x.com"、"13800138000" 与 "8613800138000" 会落到
// 不同的冷却/配额桶：同一部手机能被连发几十条短信而每桶都「没超上限」，
// 同一个邮箱还能注册出两个账号（PG 的 UNIQUE 区分大小写，MySQL 的 unicode_ci 却不区分）。
// 键构造点自身做归一化，新增调用方就无法绕过这条规则。
func canonicalTarget(codeType, target string) string {
	target = strings.TrimSpace(target)
	if codeType == string(sender.ChannelEmail) {
		return strutil.NormalizeEmail(target)
	}
	return strutil.NormalizePhone(target)
}
