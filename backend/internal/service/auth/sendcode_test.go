package authservice

import (
	"context"
	"errors"
	"testing"
	"time"

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

	if e := svc.SendCode(context.Background(), "sms", "13800138000", ""); e != nil {
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

	if e := svc.SendCode(context.Background(), "email", "a@example.com", ""); e != nil {
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

	if e := svc.SendCode(context.Background(), "sms", "13800138000", ""); e != nil {
		t.Fatalf("first SendCode: %v", e)
	}
	e := svc.SendCode(context.Background(), "sms", "13800138000", "")
	if e != errcode.ErrTooManyRequests {
		t.Errorf("second SendCode error = %v, want ErrTooManyRequests", e)
	}
}

func TestSendCode_DispatchFailureRollsBack(t *testing.T) {
	_, rdb := newTestRDB(t)
	snd := &captureSender{err: errors.New("all carriers down")}
	svc := sendCodeSvc(t, rdb, snd, true)

	e := svc.SendCode(context.Background(), "sms", "13800138000", "")
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

// quotaSvc 构造带配额/渠道白名单/观察者的服务实例。
func quotaSvc(t *testing.T, rdb redis.UniversalClient, opts ...Option) (*AuthServiceImpl, *captureSender) {
	t.Helper()
	snd := &captureSender{}
	all := append([]Option{
		WithTokenIssuer(testTokenIssuer()),
		WithMockCodeFallback(true),
		WithCodeSender(snd),
	}, opts...)
	svc, ok := New(testutil.NewFakeUserRepo(), NewRedisCacheStore(rdb), all...).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}
	return svc, snd
}

type captureObserver struct {
	channels []string
	outcomes []string
}

func (c *captureObserver) ObserveCodeSend(channel, outcome string) {
	c.channels = append(c.channels, channel)
	c.outcomes = append(c.outcomes, outcome)
}

func TestSendCode_TargetDailyQuota(t *testing.T) {
	mr, rdb := newTestRDB(t)
	svc, snd := quotaSvc(t, rdb, WithCodeQuota(2, 0))

	ctx := context.Background()
	if e := svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1"); e != nil {
		t.Fatalf("1st SendCode: %v", e)
	}
	mr.FastForward(61 * time.Second) // 越过 60s 冷却，但仍处于 24h 配额窗口
	if e := svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1"); e != nil {
		t.Fatalf("2nd SendCode: %v", e)
	}
	mr.FastForward(61 * time.Second)

	if e := svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1"); e != errcode.ErrTooManyRequests {
		t.Fatalf("3rd SendCode error = %v, want ErrTooManyRequests", e)
	}
	if snd.calls != 2 {
		t.Errorf("sender calls = %d, want 2 (quota must block before dispatch)", snd.calls)
	}
	if got, _ := rdb.Get(ctx, svc.codeQuotaKey("sms", "13800138000")).Int(); got != 2 {
		t.Errorf("target quota counter = %d, want 2", got)
	}
}

func TestSendCode_IPDailyQuota(t *testing.T) {
	mr, rdb := newTestRDB(t)
	svc, _ := quotaSvc(t, rdb, WithCodeQuota(0, 2))

	ctx := context.Background()
	for i, target := range []string{"13800000001", "13800000002"} {
		if e := svc.SendCode(ctx, "sms", target, "2.2.2.2"); e != nil {
			t.Fatalf("SendCode #%d (%s): %v", i+1, target, e)
		}
		mr.FastForward(61 * time.Second)
	}

	if e := svc.SendCode(ctx, "sms", "13800000003", "2.2.2.2"); e != errcode.ErrTooManyRequests {
		t.Fatalf("3rd target from same IP error = %v, want ErrTooManyRequests", e)
	}
	// 换一个 IP 不应被上一个 IP 的配额影响
	if e := svc.SendCode(ctx, "sms", "13800000003", "3.3.3.3"); e != nil {
		t.Fatalf("same target from another IP should pass: %v", e)
	}
	if got, _ := rdb.Get(ctx, svc.codeIPQuotaKey("2.2.2.2")).Int(); got != 2 {
		t.Errorf("ip quota counter = %d, want 2", got)
	}
}

func TestSendCode_CooldownDoesNotConsumeQuota(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc, _ := quotaSvc(t, rdb, WithCodeQuota(10, 10))

	ctx := context.Background()
	if e := svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1"); e != nil {
		t.Fatalf("1st SendCode: %v", e)
	}
	if e := svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1"); e != errcode.ErrTooManyRequests {
		t.Fatalf("2nd SendCode error = %v, want ErrTooManyRequests", e)
	}
	if got, _ := rdb.Get(ctx, svc.codeQuotaKey("sms", "13800138000")).Int(); got != 1 {
		t.Errorf("quota counter = %d, want 1 (cooldown reject must not count)", got)
	}
}

func TestSendCode_RejectsDisabledChannel(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc, snd := quotaSvc(t, rdb, WithCodeChannels([]string{"email"}))

	e := svc.SendCode(context.Background(), "sms", "13800138000", "1.1.1.1")
	if e == nil || e.HTTPStatus != 400 || e.MsgID != "err_code_channel_unavailable" {
		t.Fatalf("SendCode error = %+v, want 400 err_code_channel_unavailable", e)
	}
	if snd.calls != 0 {
		t.Errorf("sender called %d times for a disabled channel", snd.calls)
	}

	// email 在白名单内，应正常下发
	if e := svc.SendCode(context.Background(), "email", "a@example.com", "1.1.1.1"); e != nil {
		t.Fatalf("email SendCode: %v", e)
	}
}

func TestSendCode_RecordsOutcomes(t *testing.T) {
	_, rdb := newTestRDB(t)
	obs := &captureObserver{}
	svc, _ := quotaSvc(t, rdb, WithCodeChannels([]string{"email"}), WithCodeSendObserver(obs))

	ctx := context.Background()
	_ = svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1")     // unsupported_channel
	_ = svc.SendCode(ctx, "email", "a@example.com", "1.1.1.1") // sent
	_ = svc.SendCode(ctx, "email", "a@example.com", "1.1.1.1") // cooldown
	_ = svc.SendCode(ctx, "rocket", "x", "1.1.1.1")            // unsupported_channel，渠道标签应归一为 unknown

	want := []string{codeOutcomeUnsupportedChannel, codeOutcomeSent, codeOutcomeCooldown, codeOutcomeUnsupportedChannel}
	if len(obs.outcomes) != len(want) {
		t.Fatalf("outcomes = %v, want %v", obs.outcomes, want)
	}
	for i := range want {
		if obs.outcomes[i] != want[i] {
			t.Errorf("outcome[%d] = %q, want %q", i, obs.outcomes[i], want[i])
		}
	}
	wantChannels := []string{"sms", "email", "email", "unknown"}
	for i := range wantChannels {
		if obs.channels[i] != wantChannels[i] {
			t.Errorf("channel[%d] = %q, want %q", i, obs.channels[i], wantChannels[i])
		}
	}
}
