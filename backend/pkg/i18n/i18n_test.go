package i18n

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
)

func newTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

func TestLocalize_DefaultsToChineseWithoutLocalizer(t *testing.T) {
	c := newTestContext()

	if got := Localize(c, "success"); got != "成功" {
		t.Errorf("Localize(success) = %q, want %q", got, "成功")
	}
}

func TestLocalize_UnknownMsgIDFallsBackToMsgIDItself(t *testing.T) {
	c := newTestContext()

	const unknown = "this_msg_id_does_not_exist"
	if got := Localize(c, unknown); got != unknown {
		t.Errorf("Localize(unknown) = %q, want fallback to msgID %q", got, unknown)
	}
}

func TestLocalize_UsesLocalizerFromContext(t *testing.T) {
	c := newTestContext()
	c.Set(LocalizerKey, NewLocalizer("en"))

	if got := Localize(c, "success"); got != "Success" {
		t.Errorf("Localize(success) with en localizer = %q, want %q", got, "Success")
	}
}

func TestGetLocalizer_ReturnsDefaultWhenContextValueHasWrongType(t *testing.T) {
	c := newTestContext()
	c.Set(LocalizerKey, "not-a-localizer")

	l := GetLocalizer(c)
	if l == nil {
		t.Fatal("expected a non-nil default localizer")
	}
	msg, err := l.Localize(&goi18n.LocalizeConfig{MessageID: "success"})
	if err != nil {
		t.Fatalf("unexpected error localizing with fallback localizer: %v", err)
	}
	if msg != "成功" {
		t.Errorf("fallback localizer message = %q, want %q", msg, "成功")
	}
}

func TestNewLocalizer_FallsBackToChineseForUnsupportedLanguage(t *testing.T) {
	l := NewLocalizer("fr-FR")
	msg, err := l.Localize(&goi18n.LocalizeConfig{MessageID: "success"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "成功" {
		t.Errorf("message for unsupported language = %q, want zh fallback %q", msg, "成功")
	}
}

func TestLocalizeWithData_TranslatesTemplateAndFallsBackOnUnknownID(t *testing.T) {
	c := newTestContext()

	// success 消息本身不含模板变量，这里只验证传入 data 不会导致翻译失败或 panic。
	if got := LocalizeWithData(c, "success", map[string]any{"Name": "test"}); got != "成功" {
		t.Errorf("LocalizeWithData(success) = %q, want %q", got, "成功")
	}

	const unknown = "another_unknown_msg_id"
	if got := LocalizeWithData(c, unknown, map[string]any{"Name": "test"}); got != unknown {
		t.Errorf("LocalizeWithData(unknown) = %q, want fallback to msgID %q", got, unknown)
	}
}
