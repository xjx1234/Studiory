package service

import (
	"bytes"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestUserIDField_OmitsEmpty(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("op", errTest{}, UserIDField(""))

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log, got %d", logs.Len())
	}
	for _, f := range logs.All()[0].Context {
		if f.Key == "user_id" {
			t.Fatal("user_id should be omitted when empty")
		}
	}
}

func TestUserIDField_IncludesValue(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("op", errTest{}, UserIDField("uid-1"))

	found := false
	for _, f := range logs.All()[0].Context {
		if f.Key == "user_id" && f.String == "uid-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected user_id=uid-1 in log context")
	}
}

type errTest struct{}

func (errTest) Error() string { return "boom" }

func TestActorUserIDField_OmitsEmpty(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("op", errTest{}, ActorUserIDField(""))

	for _, f := range logs.All()[0].Context {
		if f.Key == "actor_user_id" {
			t.Fatal("actor_user_id should be omitted when empty")
		}
	}
}

func TestActorUserIDField_IncludesValue(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("op", errTest{}, ActorUserIDField("admin-1"))

	found := false
	for _, f := range logs.All()[0].Context {
		if f.Key == "actor_user_id" && f.String == "admin-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected actor_user_id=admin-1 in log context")
	}
}

func TestTargetField_OmitsEmpty(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("op", errTest{}, TargetField(""))

	for _, f := range logs.All()[0].Context {
		if f.Key == "target" {
			t.Fatal("target should be omitted when empty")
		}
	}
}

func TestLogInternal_NilLoggerDoesNotPanic(t *testing.T) {
	var ls LogSupport // Logger 保持 nil：模拟未调用 SetLogger 的场景

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogInternal panicked with nil logger: %v", r)
		}
	}()
	ls.LogInternal("op", errTest{}, UserIDField("uid-1"))
}

func TestLogInternal_NilErrorDoesNotLog(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("op", nil, UserIDField("uid-1"))

	if logs.Len() != 0 {
		t.Fatalf("expected no log entries when err is nil, got %d", logs.Len())
	}
}

func TestSetLogger_UpdatesLogger(t *testing.T) {
	var ls LogSupport
	if ls.Logger != nil {
		t.Fatal("expected zero-value LogSupport to have nil Logger")
	}

	logger := zap.NewNop()
	ls.SetLogger(logger)
	if ls.Logger != logger {
		t.Fatal("SetLogger should update the embedded Logger field")
	}
}

func TestTargetField_IncludesValue(t *testing.T) {
	var buf bytes.Buffer
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(&buf), zapcore.ErrorLevel)
	logger := zap.New(core)

	var ls LogSupport
	ls.SetLogger(logger)
	ls.LogInternal("SendCode", errTest{}, TargetField("13800138000"))

	if !bytes.Contains(buf.Bytes(), []byte(`"target":"13800138000"`)) {
		t.Fatalf("log output = %s", buf.String())
	}
}
