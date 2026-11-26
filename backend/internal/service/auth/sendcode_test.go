package authservice

import (
	"context"
	"errors"
	"strings"
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
	cleanups []string
}

func (c *captureObserver) ObserveCodeSend(channel, outcome string) {
	c.channels = append(c.channels, channel)
	c.outcomes = append(c.outcomes, outcome)
}

func (c *captureObserver) ObserveCodeCleanup(channel, result string) {
	c.cleanups = append(c.cleanups, channel+":"+result)
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

// expireOnSendSender 在邮件真正发出去之前取消请求 ctx，复刻「慢 SMTP 把预算耗尽」的时序：
// 前面的配额与写码都用的活 ctx，只有走到清理这一步时 ctx 已经失效。
type expireOnSendSender struct {
	expire func()
}

func (e *expireOnSendSender) Send(ctx context.Context, _ sender.Message) error {
	e.expire()
	<-ctx.Done()
	return ctx.Err()
}

// recordingDelStore 记录每次 Del 携带的 ctx 状态，并可强制删除失败。
// 用来盯两件修复前无人看守的事：清理是否带着自己的 deadline、失败是否被观察到。
type recordingDelStore struct {
	CacheStore
	delErr    error
	budgets   []time.Duration
	cancelled []bool
}

func (r *recordingDelStore) Del(ctx context.Context, keys ...string) error {
	if d, ok := ctx.Deadline(); ok {
		r.budgets = append(r.budgets, time.Until(d))
	} else {
		r.budgets = append(r.budgets, 0)
	}
	r.cancelled = append(r.cancelled, ctx.Err() != nil)

	if r.delErr != nil {
		return r.delErr
	}
	return r.CacheStore.Del(ctx, keys...)
}

func sendCodeSvcWithStore(t *testing.T, store CacheStore, snd sender.Sender, obs *captureObserver) *AuthServiceImpl {
	t.Helper()
	svc, ok := New(testutil.NewFakeUserRepo(), store,
		WithTokenIssuer(testTokenIssuer()),
		WithMockCodeFallback(true),
		WithCodeSender(snd),
		WithCodeSendObserver(obs),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}
	return svc
}

// TestSendCode_CleanupSurvivesExpiredRequestContext 是超时路径的回归用例。
//
// 修复前是 `_ = s.cache.Del(ctx, ...)`：复用已过期的请求 ctx，go-redis 直接返回 ctx 错误
// 且被吞掉，于是「回滚」在最需要它的场景里从未生效——冷却键继续挡用户 60 秒、
// 验证码键再躺 5 分钟，而配额早已在下发前扣掉。用户观感是「没收到邮件、重试还被 429」。
func TestSendCode_CleanupSurvivesExpiredRequestContext(t *testing.T) {
	_, rdb := newTestRDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obs := &captureObserver{}
	svc := sendCodeSvcWithStore(t, NewRedisCacheStore(rdb), &expireOnSendSender{expire: cancel}, obs)

	if e := svc.SendCode(ctx, "email", "a@example.com", "1.1.1.1"); e != errcode.ErrInternal {
		t.Fatalf("SendCode error = %v, want ErrInternal", e)
	}
	// 前置条件：请求 ctx 必须真的已失效，否则用例退化成「拿活 ctx 也能通过」的假阳性。
	if ctx.Err() == nil {
		t.Fatal("sender did not expire the request ctx, the test proves nothing")
	}
	// 同样条件下「沿用请求 ctx 删除」必须失败——这正是修复前留下脏键的原因。
	if err := rdb.Del(ctx, svc.codeCooldownKey("email", "a@example.com")).Err(); err == nil {
		t.Error("Del with the expired request ctx unexpectedly succeeded, so this test can no longer distinguish the fix")
	}

	if n, _ := rdb.Exists(context.Background(),
		svc.codeCooldownKey("email", "a@example.com"),
		svc.codeRedisKey("email", "a@example.com"),
	).Result(); n != 0 {
		t.Errorf("expected cooldown and code keys cleared after timeout rollback, exists=%d", n)
	}
	if got := strings.Join(obs.cleanups, ","); got != "email:ok" {
		t.Errorf("cleanup observations = %q, want %q", got, "email:ok")
	}
}

// TestSendCode_CleanupUsesOwnBudgetAndReportsFailure 固定清理的两条契约：
// 独立预算（不裸奔、也不借用已失效的请求 deadline）与失败必须可观测。
func TestSendCode_CleanupUsesOwnBudgetAndReportsFailure(t *testing.T) {
	_, rdb := newTestRDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obs := &captureObserver{}
	store := &recordingDelStore{CacheStore: NewRedisCacheStore(rdb), delErr: errors.New("redis: i/o timeout")}
	svc := sendCodeSvcWithStore(t, store, &expireOnSendSender{expire: cancel}, obs)

	if e := svc.SendCode(ctx, "email", "a@example.com", "1.1.1.1"); e != errcode.ErrInternal {
		t.Fatalf("SendCode error = %v, want ErrInternal", e)
	}
	if len(store.budgets) == 0 {
		t.Fatal("cleanup never attempted")
	}
	for i, budget := range store.budgets {
		if budget <= 0 || budget > codeCleanupTimeout {
			t.Errorf("cleanup #%d budget = %s, want (0, %s]", i, budget, codeCleanupTimeout)
		}
	}
	for i, cancelled := range store.cancelled {
		if cancelled {
			t.Errorf("cleanup #%d ran on a cancelled ctx; 清理必须脱离请求 ctx 才能真的发出 DEL", i)
		}
	}
	if got := strings.Join(obs.cleanups, ","); got == "" || strings.Contains(got, "ok") {
		t.Errorf("cleanup observations = %q, want only error results", got)
	}
}
