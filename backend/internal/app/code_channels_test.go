package app

import (
	"testing"

	"backend/internal/config"
	"backend/internal/sender"

	"go.uber.org/zap"
)

// resolveCodeChannels 负责把「已配置的 Provider」映射成「对业务可见的渠道」。
// 这里不依赖 Docker/真实 SMTP，只验证装配期的推导与失败语义。

func TestResolveCodeChannels_AutoDisablesMissingSMS(t *testing.T) {
	router := sender.NewRouter(zap.NewNop(),
		sender.NewSMTPProvider("smtp.example.com", 587, "u", "p", "noreply@example.com"),
	)

	got, err := resolveCodeChannels(&config.Config{}, router, zap.NewNop())
	if err != nil {
		t.Fatalf("resolveCodeChannels: %v", err)
	}
	if len(got) != 1 || got[0] != "email" {
		t.Fatalf("channels = %v, want [email]", got)
	}
}

func TestResolveCodeChannels_ExplicitChannelWithoutProviderFails(t *testing.T) {
	router := sender.NewRouter(zap.NewNop(),
		sender.NewSMTPProvider("smtp.example.com", 587, "u", "p", "noreply@example.com"),
	)

	_, err := resolveCodeChannels(&config.Config{AuthCodeChannels: []string{"sms"}}, router, zap.NewNop())
	if err == nil {
		t.Fatal("expected startup error for explicit sms channel without provider")
	}
}

func TestResolveCodeChannels_MockSupportsBoth(t *testing.T) {
	router := sender.NewRouter(zap.NewNop(), sender.NewMockProvider(zap.NewNop()))

	got, err := resolveCodeChannels(&config.Config{}, router, zap.NewNop())
	if err != nil {
		t.Fatalf("resolveCodeChannels: %v", err)
	}
	if len(got) != 2 || got[0] != "sms" || got[1] != "email" {
		t.Fatalf("channels = %v, want [sms email]", got)
	}
}

func TestResolveCodeChannels_DedupesAndNormalizes(t *testing.T) {
	router := sender.NewRouter(zap.NewNop(), sender.NewMockProvider(zap.NewNop()))

	got, err := resolveCodeChannels(&config.Config{
		AuthCodeChannels: []string{" EMAIL ", "email", "sms"},
	}, router, zap.NewNop())
	if err != nil {
		t.Fatalf("resolveCodeChannels: %v", err)
	}
	if len(got) != 2 || got[0] != "email" || got[1] != "sms" {
		t.Fatalf("channels = %v, want [email sms]", got)
	}
}
