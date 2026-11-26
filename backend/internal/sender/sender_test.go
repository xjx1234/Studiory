package sender

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeProvider 是可配置行为的测试 Provider。
type fakeProvider struct {
	name     string
	channels map[Channel]bool
	err      error
	calls    int
	lastMsg  Message
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Supports(ch Channel) bool { return f.channels[ch] }

func (f *fakeProvider) Send(_ context.Context, msg Message) error {
	f.calls++
	f.lastMsg = msg
	return f.err
}

func smsProvider(name string, err error) *fakeProvider {
	return &fakeProvider{name: name, channels: map[Channel]bool{ChannelSMS: true}, err: err}
}

func TestRouter_RoutesByChannel(t *testing.T) {
	sms := smsProvider("sms1", nil)
	email := &fakeProvider{name: "email1", channels: map[Channel]bool{ChannelEmail: true}}

	r := NewRouter(nil, sms, email)

	if err := r.Send(context.Background(), Message{Channel: ChannelSMS, Target: "138", Code: "1"}); err != nil {
		t.Fatalf("sms send: %v", err)
	}
	if sms.calls != 1 || email.calls != 0 {
		t.Errorf("expected only sms provider called, sms=%d email=%d", sms.calls, email.calls)
	}

	if err := r.Send(context.Background(), Message{Channel: ChannelEmail, Target: "a@b.c", Code: "2"}); err != nil {
		t.Fatalf("email send: %v", err)
	}
	if email.calls != 1 {
		t.Errorf("expected email provider called once, got %d", email.calls)
	}
}

func TestRouter_FailoverToNextProvider(t *testing.T) {
	primary := smsProvider("primary", errors.New("carrier down"))
	backup := smsProvider("backup", nil)

	r := NewRouter(nil, primary, backup)

	if err := r.Send(context.Background(), Message{Channel: ChannelSMS, Target: "138", Code: "9"}); err != nil {
		t.Fatalf("expected failover success, got %v", err)
	}
	if primary.calls != 1 || backup.calls != 1 {
		t.Errorf("expected both tried in order, primary=%d backup=%d", primary.calls, backup.calls)
	}
	if backup.lastMsg.Code != "9" {
		t.Errorf("backup got wrong code: %q", backup.lastMsg.Code)
	}
}

func TestRouter_AllProvidersFail(t *testing.T) {
	p1 := smsProvider("p1", errors.New("e1"))
	p2 := smsProvider("p2", errors.New("e2"))

	r := NewRouter(nil, p1, p2)

	err := r.Send(context.Background(), Message{Channel: ChannelSMS, Target: "138", Code: "1"})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if p1.calls != 1 || p2.calls != 1 {
		t.Errorf("expected both tried, p1=%d p2=%d", p1.calls, p2.calls)
	}
}

func TestRouter_NoProviderForChannel(t *testing.T) {
	r := NewRouter(nil, smsProvider("sms", nil))

	err := r.Send(context.Background(), Message{Channel: ChannelEmail, Target: "a@b.c", Code: "1"})
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("expected ErrNoProvider, got %v", err)
	}
}

func TestMockProvider_SupportsAllAndSucceeds(t *testing.T) {
	p := NewMockProvider(nil)
	for _, ch := range AllChannels {
		if !p.Supports(ch) {
			t.Errorf("mock should support channel %s", ch)
		}
	}
	if err := p.Send(context.Background(), Message{Channel: ChannelSMS, Target: "138", Code: "123456"}); err != nil {
		t.Errorf("mock send error: %v", err)
	}
}

// budgetSpy 记录自己看到的上下文状态，可阻塞到上下文结束（模拟拖满预算的通道）。
type budgetSpy struct {
	*fakeProvider
	block   bool
	onEnter func()
	live    []bool
	budgets []time.Duration
}

func (b *budgetSpy) Send(ctx context.Context, msg Message) error {
	b.live = append(b.live, ctx.Err() == nil)
	if d, ok := ctx.Deadline(); ok {
		b.budgets = append(b.budgets, time.Until(d))
	} else {
		b.budgets = append(b.budgets, -1)
	}
	if b.onEnter != nil {
		b.onEnter()
	}
	if b.block {
		<-ctx.Done()
	}
	return b.fakeProvider.Send(ctx, msg)
}

func smsSpy(name string, err error, block bool) *budgetSpy {
	return &budgetSpy{fakeProvider: smsProvider(name, err), block: block}
}

// TestRouter_SlowPrimaryCannotStarveBackup 修复前所有 provider 共用同一个请求 ctx：
// 主通道把预算拖光后备用通道拿到的是已过期的 ctx，故障转移只是名义上存在。
func TestRouter_SlowPrimaryCannotStarveBackup(t *testing.T) {
	const total = 400 * time.Millisecond

	primary := smsSpy("primary", errors.New("relay stalled"), true)
	backup := smsSpy("backup", nil, false)
	r := NewRouter(nil, primary, backup)

	ctx, cancel := context.WithTimeout(context.Background(), total)
	defer cancel()

	start := time.Now()
	if err := r.Send(ctx, Message{Channel: ChannelSMS, Target: "138", Code: "1"}); err != nil {
		t.Fatalf("backup must take over when the primary stalls: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= total {
		t.Errorf("Router.Send outlived the request budget: %s >= %s", elapsed, total)
	}
	if len(backup.live) != 1 || !backup.live[0] {
		t.Fatalf("backup provider ran on a dead context: live=%v", backup.live)
	}
	// 主通道只应拿到一部分预算（均分后约 200ms）；修复前它会看到完整的 400ms。
	if len(primary.budgets) != 1 || primary.budgets[0] <= 0 || primary.budgets[0] > total*3/4 {
		t.Errorf("primary budget = %v, want a slice clearly below the whole %s", primary.budgets, total)
	}
}

// TestRouter_DeadContextStopsFailoverAndSaysSo 上下文已结束时不再空转，
// 但必须在错误里讲清「备用通道根本没被尝试」，否则排查时会误以为它也失败过。
func TestRouter_DeadContextStopsFailoverAndSaysSo(t *testing.T) {
	primary := smsSpy("primary", errors.New("stalled"), true)
	backup := smsSpy("backup", nil, false)
	r := NewRouter(nil, primary, backup)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primary.onEnter = cancel

	err := r.Send(ctx, Message{Channel: ChannelSMS, Target: "138", Code: "1"})
	if err == nil {
		t.Fatal("expected error when the context is already done")
	}
	if !strings.Contains(err.Error(), "not attempted") {
		t.Errorf("error must report the skipped provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("error must count skipped providers, got: %v", err)
	}
	if backup.calls != 0 {
		t.Errorf("backup calls = %d, want 0", backup.calls)
	}
}

// TestRouter_NoDeadlineContextPassesThrough 调用方没设 deadline 时不发明超时：
// 否则 CLI/测试里一次正常的下发会被 Router 自己掐断。
func TestRouter_NoDeadlineContextPassesThrough(t *testing.T) {
	primary := smsSpy("primary", errors.New("e1"), false)
	backup := smsSpy("backup", nil, false)
	r := NewRouter(nil, primary, backup)

	if err := r.Send(context.Background(), Message{Channel: ChannelSMS, Target: "138", Code: "1"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	for i, budgets := range [][]time.Duration{primary.budgets, backup.budgets} {
		if len(budgets) != 1 || budgets[0] != -1 {
			t.Errorf("provider #%d saw deadline=%v, want no deadline (-1)", i, budgets)
		}
	}
}

// TestRouter_SingleProviderKeepsWholeBudget 只有一个通道时不能把它切成两半。
func TestRouter_SingleProviderKeepsWholeBudget(t *testing.T) {
	const total = 800 * time.Millisecond

	only := smsSpy("only", nil, false)
	r := NewRouter(nil, only)

	ctx, cancel := context.WithTimeout(context.Background(), total)
	defer cancel()

	if err := r.Send(ctx, Message{Channel: ChannelSMS, Target: "138", Code: "1"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(only.budgets) != 1 {
		t.Fatalf("budgets = %v, want one entry", only.budgets)
	}
	if only.budgets[0] <= total/2 {
		t.Errorf("single provider budget = %s, want the whole %s", only.budgets[0], total)
	}
}
