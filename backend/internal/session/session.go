// Package session 管理用户登录会话（多设备 / 单设备），基于 Redis。
//
// 多设备模式（multiDevice=true）：每次登录创建独立 session，互不影响；登出仅吊销当前 session。
// 单设备模式（multiDevice=false）：新登录覆盖旧 session，旧设备 token 立即失效。
//
// 有效期语义为「绝对过期」：session 在登录时写入 TTL，之后无论刷新多少次 token 都不再延长。
// 因此用户最长在线时间等于 sessionTTL（默认与 refresh token 一致，7 天），到期必须重新登录。
// 绝对过期避免「refresh token 每次续 7 天、session 却被悄悄拉长」导致会话无限续命；
// 若需要滑动过期，应在 Refresh 路径显式续期并同步更新本文件的键 TTL 与文档。
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	baseservice "backend/internal/service"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Store 是会话状态的 Redis 存储。
type Store struct {
	rdb         redis.UniversalClient
	prefix      string
	sessionTTL  time.Duration
	multiDevice bool
	failOpen    bool // Redis故障时：true=放行（可用性优先）；false=拒绝（安全性优先）
}

// NewStore 创建会话存储。sessionTTL 是会话的绝对寿命（登录时写入，刷新不续期），
// 建议与 refresh token 有效期一致。
// failOpen 控制 Redis 故障时的降级策略：true=放行所有 session（可用性优先）；
// false=拒绝所有 session（安全性优先）。
func NewStore(rdb redis.UniversalClient, prefix string, multiDevice bool, sessionTTL time.Duration, failOpen bool) *Store {
	return &Store{
		rdb:         rdb,
		prefix:      prefix,
		sessionTTL:  sessionTTL,
		multiDevice: multiDevice,
		failOpen:    failOpen,
	}
}

// MultiDevice 返回当前是否为多设备模式。
func (s *Store) MultiDevice() bool {
	return s.multiDevice
}

// NewSessionID 生成新的会话 ID。
func NewSessionID() string {
	return uuid.New().String()
}

// Register 注册新会话。单设备模式下会先吊销该用户所有旧会话。
func (s *Store) Register(ctx context.Context, userID, sessionID string) error {
	if s.rdb == nil || userID == "" || sessionID == "" {
		return nil
	}

	if !s.multiDevice {
		if err := s.RevokeAll(ctx, userID); err != nil {
			return err
		}
	}

	sessionKey := s.sessionKey(sessionID)
	// 值存归一化后的 ID：Validate 拿它和当前请求的 userID 做字符串比较。
	if err := s.rdb.Set(ctx, sessionKey, baseservice.CanonicalUserID(userID), s.sessionTTL).Err(); err != nil {
		return err
	}

	if s.multiDevice {
		setKey := s.userSessionsKey(userID)
		if err := s.rdb.SAdd(ctx, setKey, sessionID).Err(); err != nil {
			return err
		}
		// 集合只是 RevokeAll 的枚举索引，TTL 取「覆盖最新一次登录」即可。
		// 延长集合不会延长任何旧 session：Validate 只认 session:{sid} 自身的 TTL，
		// 旧 sid 的 session 键到期后即使仍在集合里也会被拒绝。
		if err := s.rdb.Expire(ctx, setKey, s.sessionTTL).Err(); err != nil {
			return fmt.Errorf("expire user sessions set: %w", err)
		}
	} else {
		activeKey := s.activeSessionKey(userID)
		if err := s.rdb.Set(ctx, activeKey, sessionID, s.sessionTTL).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Validate 校验会话是否仍有效。
//
// 判断依据只有一条：session:{sid} 键存在，且 value 与当前 userID 一致。
//   - 绝对过期：键在登录时写入 TTL，刷新不会续期，过期后必然查不到；
//   - 吊销：Revoke/RevokeAll 删除 session 键；单设备模式新登录也会先 RevokeAll；
//   - sessions:uid 集合只用于 RevokeAll 枚举，不参与有效性判断，
//     避免「集合被续期 → 过期旧 sid 复活」这类隐式副作用。
//
// Redis 故障时按 failOpen 策略降级：
//   - failOpen=true：放行（可用性优先，session 校验暂时跳过）
//   - failOpen=false：拒绝（安全性优先，所有请求被视为未认证）
func (s *Store) Validate(ctx context.Context, userID, sessionID string) bool {
	if s.rdb == nil || userID == "" || sessionID == "" {
		return true
	}

	uid, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Result()
	if err == nil {
		return uid == baseservice.CanonicalUserID(userID)
	}
	if errors.Is(err, redis.Nil) {
		return false
	}
	// Redis 连接故障：按配置策略降级
	return s.failOpen
}

// Revoke 吊销单个会话（登出当前设备）。
// 返回底层 Redis 操作中遇到的所有错误（errors.Join），调用方应记录日志：
// 吊销失败意味着该会话在 TTL 到期前可能仍被 Validate 判定为有效。
func (s *Store) Revoke(ctx context.Context, userID, sessionID string) error {
	if s.rdb == nil || userID == "" || sessionID == "" {
		return nil
	}

	var errs []error

	if err := s.rdb.Del(ctx, s.sessionKey(sessionID)).Err(); err != nil {
		errs = append(errs, fmt.Errorf("del session key: %w", err))
	}

	if s.multiDevice {
		if err := s.rdb.SRem(ctx, s.userSessionsKey(userID), sessionID).Err(); err != nil {
			errs = append(errs, fmt.Errorf("srem user sessions set: %w", err))
		}
	} else {
		activeKey := s.activeSessionKey(userID)
		active, err := s.rdb.Get(ctx, activeKey).Result()
		if err == nil && active == sessionID {
			if delErr := s.rdb.Del(ctx, activeKey).Err(); delErr != nil {
				errs = append(errs, fmt.Errorf("del active session key: %w", delErr))
			}
		}
	}
	return errors.Join(errs...)
}

// RevokeAll 吊销用户全部会话（改密、禁用账号、单设备新登录前）。
// 返回底层 Redis 操作中遇到的所有错误（errors.Join）：这是安全相关的关键操作
// （改密/禁用账号后应立即失效旧会话），调用方必须记录日志，不能静默丢弃。
func (s *Store) RevokeAll(ctx context.Context, userID string) error {
	if s.rdb == nil || userID == "" {
		return nil
	}

	var errs []error

	if s.multiDevice {
		setKey := s.userSessionsKey(userID)
		ids, err := s.rdb.SMembers(ctx, setKey).Result()
		if err != nil && err != redis.Nil {
			return err
		}
		for _, sid := range ids {
			if err := s.rdb.Del(ctx, s.sessionKey(sid)).Err(); err != nil {
				errs = append(errs, fmt.Errorf("del session key %s: %w", sid, err))
			}
		}
		if err := s.rdb.Del(ctx, setKey).Err(); err != nil {
			errs = append(errs, fmt.Errorf("del user sessions set: %w", err))
		}
	} else {
		activeKey := s.activeSessionKey(userID)
		active, err := s.rdb.Get(ctx, activeKey).Result()
		if err == nil && active != "" {
			if err := s.rdb.Del(ctx, s.sessionKey(active), activeKey).Err(); err != nil {
				errs = append(errs, fmt.Errorf("del active session: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Store) sessionKey(sessionID string) string {
	return fmt.Sprintf("%s:session:%s", s.prefix, sessionID)
}

func (s *Store) userSessionsKey(userID string) string {
	return fmt.Sprintf("%s:sessions:uid:%s", s.prefix, baseservice.CanonicalUserID(userID))
}

func (s *Store) activeSessionKey(userID string) string {
	return fmt.Sprintf("%s:active_session:uid:%s", s.prefix, baseservice.CanonicalUserID(userID))
}
