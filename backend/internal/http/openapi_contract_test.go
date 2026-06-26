package http

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"backend/internal/config"
	"backend/internal/openapi"

	"github.com/gin-gonic/gin"
)

func TestOpenAPIContractMatchesRouter(t *testing.T) {
	r := newContractRouter(t)

	doc, err := openapi.LoadDocument(openapiSpecPath(t))
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}

	docOps := openapi.OperationsFromDocument(doc)
	routerOps := openapi.OperationsFromRoutes(ginRoutes(r),
		[]string{"/metrics"}, // Prometheus 端点 intentionally 不写入 OpenAPI
		nil,
	)

	onlyDoc, onlyRouter := openapi.CompareOperations(docOps, routerOps)
	if len(onlyDoc) > 0 {
		t.Errorf("operations documented in openapi.yaml but missing in router:\n  %s", strings.Join(onlyDoc, "\n  "))
	}
	if len(onlyRouter) > 0 {
		t.Errorf("router operations missing in openapi.yaml:\n  %s", strings.Join(onlyRouter, "\n  "))
	}
}

// newContractRouter 构建与生产一致的路由表（含 /metrics），用于 OpenAPI 契约校验。
func newContractRouter(t *testing.T) *gin.Engine {
	t.Helper()

	ts := &testServer{
		auth:  &fakeAuthService{},
		user:  &fakeUserService{},
		todo:  &fakeTodoService{},
		admin: &fakeAdminService{},
	}

	deps := &Deps{
		Cfg:                     &config.Config{AppEnv: "test", CORSAllowOrigins: []string{"*"}},
		AuthService:             ts.auth,
		UserService:             ts.user,
		TodoService:             ts.todo,
		AdminService:            ts.admin,
		AuthMiddleware:          fakeAuthMiddleware,
		RateLimitMiddleware:     func(c *gin.Context) { c.Next() },
		UserRateLimitMiddleware: func(c *gin.Context) { c.Next() },
		MetricsHandler:          http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ReadyChecks: []ReadyCheck{
			{Name: "noop", Check: func(context.Context) error { return nil }},
		},
	}

	r, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return r
}

func ginRoutes(r *gin.Engine) []openapi.RouteInfo {
	raw := r.Routes()
	routes := make([]openapi.RouteInfo, len(raw))
	for i, route := range raw {
		routes[i] = openapi.RouteInfo{Method: route.Method, Path: route.Path}
	}
	return routes
}

func openapiSpecPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// backend/internal/http/*_test.go -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(root, "docs", "api", "openapi.yaml")
}
