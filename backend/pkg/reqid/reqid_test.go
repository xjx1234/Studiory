package reqid

import (
	"context"
	"testing"
)

func TestFromContext_RoundTrip(t *testing.T) {
	ctx := WithID(context.Background(), "req-123")
	if got := FromContext(ctx); got != "req-123" {
		t.Fatalf("FromContext() = %q, want req-123", got)
	}
}

func TestFromContext_MissingOrEmpty(t *testing.T) {
	if got := FromContext(context.Background()); got != "" {
		t.Errorf("FromContext(background) = %q, want empty", got)
	}
	if got := FromContext(context.TODO()); got != "" {
		t.Errorf("FromContext(todo) = %q, want empty", got)
	}
	if got := FromContext(WithID(context.Background(), "")); got != "" {
		t.Errorf("FromContext(empty id) = %q, want empty", got)
	}
}
