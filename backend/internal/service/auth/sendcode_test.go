package authservice

import (
	"context"
	"errors"
	"testing"

	"backend/internal/sender"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/redis/go-redis/v9"
)

// captureSender 记录最后一次下发的消息，可配置返回错误。
type captureSender struct {
	calls int
	last  sender.Message
	err   error
}

func (c *captureSender) Send(_ context.Context, msg sender.Message) error {
	c.calls++
	c.last = msg
	return c.err
}

func sendCodeSvc(t *testing.T, rdb redis.UniversalClient, snd sender.Sender, mockFallback bool) *AuthServiceImpl {
	t.Helper()
	svc, ok := New(testutil.NewFakeUserRepo(), NewRedisCacheStore(rdb),
		WithTokenIssuer(testTokenIssuer()),
		WithMockCodeFallback(mockFallback),
		WithCodeSender(snd),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}
	return svc
}

func TestSendCode_StoresAndDispatches(t *testing.T) {
	_, rdb := newTestRDB(t)
	snd := &captureSender{}
	svc := sendCodeSvc(t, rdb, snd, true) // mock fallback → 固定码 123456

	if e := svc.SendCode(context.Background(), "sms", "13800138000"); e != nil {
		t.Fatalf("SendCode error: %v", e)
	}

	if snd.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", snd.calls)
	}
	if snd.last.Channel != sender.ChannelSMS || snd.last.Target != "13800138000" {
		t.Errorf("unexpected message: %+v", snd.last)
	}

	// Redis 中存的验证码应与下发的一致
	stored, err := rdb.Get(context.Background(), svc.codeRedisKey("sms", "13800138000")).Result()
	if err != nil {
		t.Fatalf("get stored code: %v", err)
	}
	if stored != snd.last.Code {
		t.Errorf("stored code %q != dispatched code %q", stored, snd.last.Code)
	}
	if stored != MockVerificationCode {
		t.Errorf("mock fallback code = %q, want %q", stored, MockVerificationCode)
	}
}

func TestSendCode_RandomCodeWhenNotMock(t *testing.T) {
	_, rdb := newTestRDB(t)
	snd := &captureSender{}
	svc := sendCodeSvc(t, rdb, snd, false) // 非 mock → 6 位随机码

	if e := svc.SendCode(context.Background(), "email", "a@example.com"); e != nil {
		t.Fatalf("SendCode error: %v", e)
	}

	code := snd.last.Code
	if len(code) != 6 {
		t.Errorf("random code length = %d, want 6 (code=%q)", len(code), code)
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			t.Errorf("code contains non-digit: %q", code)
			break
		}
	}
	if code == MockVerificationCode {
		t.Errorf("expected random code, got mock code")
	}
}

func TestSendCode_CooldownBlocksSecond(t *testing.T) {
	_, rdb := newTestRDB(t)
	snd := &captureSender{}
	svc := sendCodeSvc(t, rdb, snd, true)

	if e := svc.SendCode(context.Background(), "sms", "13800138000"); e != nil {
		t.Fatalf("first SendCode: %v", e)
	}
	e := svc.SendCode(context.Background(), "sms", "13800138000")
	if e != errcode.ErrTooManyRequests {
		t.Errorf("second SendCode error = %v, want ErrTooManyRequests", e)
	}
}

func TestSendCode_DispatchFailureRollsBack(t *testing.T) {
	_, rdb := newTestRDB(t)
	snd := &captureSender{err: errors.New("all carriers down")}
	svc := sendCodeSvc(t, rdb, snd, true)

	e := svc.SendCode(context.Background(), "sms", "13800138000")
	if e != errcode.ErrInternal {
		t.Fatalf("SendCode error = %v, want ErrInternal", e)
	}

	// 失败后冷却键与验证码均应被清除，允许立即重试
	if n, _ := rdb.Exists(context.Background(),
		svc.codeCooldownKey("sms", "13800138000"),
		svc.codeRedisKey("sms", "13800138000"),
	).Result(); n != 0 {
		t.Errorf("expected cooldown and code cleared after dispatch failure, exists=%d", n)
	}
}
