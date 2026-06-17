package sender

import (
	"context"
	"errors"
	"testing"
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
