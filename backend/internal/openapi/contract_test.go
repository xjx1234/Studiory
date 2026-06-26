package openapi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadDocument(t *testing.T) {
	doc, err := LoadDocument(openapiPath(t))
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if doc.Info == nil || doc.Info.Title == "" {
		t.Fatal("expected non-empty info.title")
	}
	if len(OperationsFromDocument(doc)) == 0 {
		t.Fatal("expected at least one operation in openapi.yaml")
	}
}

func TestCompareOperations_NoDiffWhenIdentical(t *testing.T) {
	ops := []Operation{
		{Method: "GET", Path: "/health"},
		{Method: "POST", Path: "/api/v1/auth/login"},
	}
	onlyDoc, onlyRouter := CompareOperations(ops, ops)
	if len(onlyDoc) != 0 || len(onlyRouter) != 0 {
		t.Fatalf("expected no diff, doc=%v router=%v", onlyDoc, onlyRouter)
	}
}

func TestNormalizePath(t *testing.T) {
	got := NormalizePath("/api/v1/user/todos/:id")
	want := "/api/v1/user/todos/{id}"
	if got != want {
		t.Fatalf("NormalizePath() = %q, want %q", got, want)
	}
}

func openapiPath(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	return filepath.Join(root, "docs", "api", "openapi.yaml")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// backend/internal/openapi/*_test.go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestOpenAPIFileExists(t *testing.T) {
	if _, err := os.Stat(openapiPath(t)); err != nil {
		t.Fatalf("openapi.yaml missing: %v", err)
	}
}
