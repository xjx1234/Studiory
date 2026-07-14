package authservice

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"backend/internal/sender"
	baseservice "backend/internal/service"
	"backend/pkg/errcode"

	"go.uber.org/zap"
)

const (
	codeExpiry       = 5 * time.Minute
	codeSendCooldown = 60 * time.Second

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
)

// SendCode 生成验证码、写入 Redis，并通过下发器（多服务商路由）发送。
//
// 流程：冷却限频 → 生成验证码 → 写 Redis → 经 codeSender 下发。
// 下发失败时清除冷却键，允许用户立即重试。
func (s *AuthServiceImpl) SendCode(ctx context.Context, codeType, target string) *errcode.Error {
	if codeType == "" || target == "" {
		return errcode.ErrBadRequest
	}

	cooldownKey := s.codeCooldownKey(codeType, target)
	ok, err := s.rdb.SetNX(ctx, cooldownKey, "1", codeSendCooldown).Result()
	if err != nil {
		s.LogInternal("SendCode set cooldown", err,
			zap.String("code_type", codeType),
			baseservice.TargetField(target),
		)
		return errcode.ErrInternal
	}
	if !ok {
		return errcode.ErrTooManyRequests
	}

	code := s.generateCode()

	key := s.codeRedisKey(codeType, target)
	if err := s.rdb.Set(ctx, key, code, codeExpiry).Err(); err != nil {
		s.LogInternal("SendCode set code", err,
			zap.String("code_type", codeType),
			baseservice.TargetField(target),
		)
		_ = s.rdb.Del(ctx, cooldownKey).Err()
		return errcode.ErrInternal
	}

	if s.codeSender != nil {
		if err := s.codeSender.Send(ctx, sender.Message{
			Channel: sender.Channel(codeType),
			Target:  target,
			Code:    code,
		}); err != nil {
			s.LogInternal("SendCode dispatch", err,
				zap.String("code_type", codeType),
				baseservice.TargetField(target),
			)
			// 下发失败：回滚冷却与验证码，便于用户重试
			_ = s.rdb.Del(ctx, cooldownKey, key).Err()
			return errcode.ErrInternal
		}
	}

	return nil
}

// generateCode 生成验证码。
// 开发模式（allowMockCodeFallback）下返回固定的 MockVerificationCode 便于联调；
// 否则生成 6 位随机数字验证码。
func (s *AuthServiceImpl) generateCode() string {
	if s.allowMockCodeFallback {
		return MockVerificationCode
	}
	const digits = "0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			// crypto/rand 失败极罕见，退回固定码以不阻塞流程（已写日志）
			s.LogInternal("generateCode rand", err)
			return MockVerificationCode
		}
		b[i] = digits[n.Int64()]
	}
	return string(b)
}

// verifyCode 校验验证码。
// 策略：先查 Redis，key 不存在时在开发阶段允许 MockVerificationCode 通过（便于本地无 Redis 调试）。
// 验证成功后删除 key，确保一次有效。
func (s *AuthServiceImpl) verifyCode(ctx context.Context, codeType, target, code string) bool {
	key := s.codeRedisKey(codeType, target)
	result, err := s.rdb.Eval(ctx, verifyCodeLua, []string{key}, code).Int()
	if err != nil {
		return false
	}

	switch result {
	case 1:
		return true
	case -1:
		return s.allowMockCodeFallback && code == MockVerificationCode
	default:
		return false
	}
}

func (s *AuthServiceImpl) codeRedisKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:%s", s.codePrefix, codeType, target)
}

func (s *AuthServiceImpl) codeCooldownKey(codeType, target string) string {
	return fmt.Sprintf("%s:%s:cooldown:%s", s.codePrefix, codeType, target)
}
