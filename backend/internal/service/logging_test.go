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
