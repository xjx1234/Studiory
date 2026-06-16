package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/repo"

	"github.com/gin-gonic/gin"
)

func TestRequireRoleAllowsExpectedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set(ContextKeyUserRole, repo.RoleAdmin)
	}, RequireRole(repo.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
}

func TestRequireRoleRejectsUnexpectedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set(ContextKeyUserRole, repo.RoleUser)
	}, RequireRole(repo.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Code)
	}
}
