// Package session 管理用户登录会话（多设备 / 单设备），基于 Redis。
//
// 多设备模式（multiDevice=true）：每次登录创建独立 session，互不影响；登出仅吊销当前 session。
// 单设备模式（multiDevice=false）：新登录覆盖旧 session，旧设备 token 立即失效。
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Store 是会话状态的 Redis 存储。
type Store struct {
	rdb         redis.UniversalClient
	prefix      string
	sessionTTL  time.Duration
	multiDevice bool
}

// NewStore 创建会话存储。sessionTTL 建议与 refresh token 有效期一致。
func NewStore(rdb redis.UniversalClient, prefix string, multiDevice bool, sessionTTL time.Duration) *Store {
	return &Store{
		rdb:         rdb,
		prefix:      prefix,
		sessionTTL:  sessionTTL,
		multiDevice: multiDevice,
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
	if err := s.rdb.Set(ctx, sessionKey, userID, s.sessionTTL).Err(); err != nil {
		return err
	}

	if s.multiDevice {
		setKey := s.userSessionsKey(userID)
		if err := s.rdb.SAdd(ctx, setKey, sessionID).Err(); err != nil {
			return err
		}
		_ = s.rdb.Expire(ctx, setKey, s.sessionTTL).Err()
	} else {
		activeKey := s.activeSessionKey(userID)
		if err := s.rdb.Set(ctx, activeKey, sessionID, s.sessionTTL).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Validate 校验会话是否仍有效。
func (s *Store) Validate(ctx context.Context, userID, sessionID string) bool {
	if s.rdb == nil || userID == "" || sessionID == "" {
		return true // 无 Redis 时不阻断（与 revoke fail-open 一致）
	}

	if s.multiDevice {
		setKey := s.userSessionsKey(userID)
		ok, err := s.rdb.SIsMember(ctx, setKey, sessionID).Result()
		if err == nil && ok {
			return true
		}
	} else {
		activeKey := s.activeSessionKey(userID)
		active, err := s.rdb.Get(ctx, activeKey).Result()
		if err == nil && active == sessionID {
			return true
		}
	}

	// 兜底：session 键仍存在也视为有效（应对 set/active 过期但 session 键尚在的边界）
	uid, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Result()
	return err == nil && uid == userID
}

// Revoke 吊销单个会话（登出当前设备）。
func (s *Store) Revoke(ctx context.Context, userID, sessionID string) error {
	if s.rdb == nil || userID == "" || sessionID == "" {
		return nil
	}

	_ = s.rdb.Del(ctx, s.sessionKey(sessionID)).Err()

	if s.multiDevice {
		_ = s.rdb.SRem(ctx, s.userSessionsKey(userID), sessionID).Err()
	} else {
		activeKey := s.activeSessionKey(userID)
		active, err := s.rdb.Get(ctx, activeKey).Result()
		if err == nil && active == sessionID {
			_ = s.rdb.Del(ctx, activeKey).Err()
		}
	}
	return nil
}

// RevokeAll 吊销用户全部会话（改密、禁用账号、单设备新登录前）。
func (s *Store) RevokeAll(ctx context.Context, userID string) error {
	if s.rdb == nil || userID == "" {
		return nil
	}

	if s.multiDevice {
		setKey := s.userSessionsKey(userID)
		ids, err := s.rdb.SMembers(ctx, setKey).Result()
		if err != nil && err != redis.Nil {
			return err
		}
		for _, sid := range ids {
			_ = s.rdb.Del(ctx, s.sessionKey(sid)).Err()
		}
		_ = s.rdb.Del(ctx, setKey).Err()
	} else {
		activeKey := s.activeSessionKey(userID)
		active, err := s.rdb.Get(ctx, activeKey).Result()
		if err == nil && active != "" {
			_ = s.rdb.Del(ctx, s.sessionKey(active), activeKey).Err()
		}
	}
	return nil
}

func (s *Store) sessionKey(sessionID string) string {
	return fmt.Sprintf("%s:session:%s", s.prefix, sessionID)
}

func (s *Store) userSessionsKey(userID string) string {
	return fmt.Sprintf("%s:sessions:uid:%s", s.prefix, userID)
}

func (s *Store) activeSessionKey(userID string) string {
	return fmt.Sprintf("%s:active_session:uid:%s", s.prefix, userID)
}
