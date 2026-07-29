// ── 登录/注册暴力破解防护 ────────────────────────────────────────────────────
//
// 同一账号（手机号/邮箱）在 loginFailWindow 内失败 loginMaxFailAttempts 次后锁定
// loginLockDuration。覆盖：密码登录、验证码登录、验证码注册。

package authservice

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	loginMaxFailAttempts = 5
	loginFailWindow      = 10 * time.Minute
	loginLockDuration    = 15 * time.Minute

	// loginRecordFailLua 原子递增失败计数，首次写入时设置 TTL；达到阈值时写入锁定 key。
	// KEYS[1]=fail_key  KEYS[2]=lock_key
	// ARGV[1]=fail_window_sec  ARGV[2]=max_attempts  ARGV[3]=lock_ttl_sec
	loginRecordFailLua = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
end
if count >= tonumber(ARGV[2]) then
    redis.call('SET', KEYS[2], '1', 'EX', tonumber(ARGV[3]))
end
return count
`
)

// isLoginLocked 检查账号是否处于锁定状态。
// Redis 故障时按 failOpen 策略降级：true=放行（可用性优先）；false=拒绝（安全性优先）。
func (s *AuthServiceImpl) isLoginLocked(ctx context.Context, account string) bool {
	if s.cache == nil || account == "" {
		return false
	}
	exists, err := s.cache.Exists(ctx, s.loginLockKey(account))
	if err != nil {
		strategy := "fail-open"
		if !s.failOpen {
			strategy = "fail-closed"
		}
		if s.Logger != nil {
			// 只记账号哈希前缀，不落明文账号（与 Redis 键同一标准）
			h := sha256.Sum256([]byte(account))
			s.Logger.Warn("isLoginLocked redis error, "+strategy,
				zap.String("account_hash", fmt.Sprintf("%x", h[:4])),
				zap.Error(err))
		}
		// failOpen=true → 视为未锁定（放行）；failOpen=false → 视为已锁定（拒绝）
		return !s.failOpen
	}
	return exists
}

// recordLoginFail 原子递增失败计数，达到阈值时写入锁定 key。
func (s *AuthServiceImpl) recordLoginFail(ctx context.Context, account string) {
	if s.cache == nil || account == "" {
		return
	}
	failKey := s.loginFailKey(account)
	lockKey := s.loginLockKey(account)
	if _, err := s.cache.Eval(ctx, loginRecordFailLua, []string{failKey, lockKey},
		int64(loginFailWindow.Seconds()),
		loginMaxFailAttempts,
		int64(loginLockDuration.Seconds()),
	); err != nil {
		if s.Logger != nil {
			s.Logger.Warn("recordLoginFail redis error", zap.Error(err))
		}
	}
}

// clearLoginFail 登录成功后清除失败计数（锁定 key 等自然过期）。
func (s *AuthServiceImpl) clearLoginFail(ctx context.Context, account string) {
	if s.cache == nil || account == "" {
		return
	}
	if err := s.cache.Del(ctx, s.loginFailKey(account)); err != nil {
		if s.Logger != nil {
			s.Logger.Warn("clearLoginFail redis error", zap.Error(err))
		}
	}
}

// loginAccountID 从登录请求字段中提取用于防暴力破解计数的账号标识符。
func loginAccountID(phone, email, account string) string {
	switch {
	case phone != "":
		return strings.ToLower(strings.TrimSpace(phone))
	case email != "":
		return strings.ToLower(strings.TrimSpace(email))
	default:
		return strings.ToLower(strings.TrimSpace(account))
	}
}

func (s *AuthServiceImpl) loginFailKey(account string) string {
	h := sha256.Sum256([]byte(account))
	return fmt.Sprintf("%s:login:fail:%x", s.codePrefix, h)
}

func (s *AuthServiceImpl) loginLockKey(account string) string {
	h := sha256.Sum256([]byte(account))
	return fmt.Sprintf("%s:login:lock:%x", s.codePrefix, h)
}
