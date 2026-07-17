package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	pkgi18n "backend/pkg/i18n"

	"github.com/gin-gonic/gin"
)

func TestI18n_QueryParamTakesPriorityOverHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(I18n())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, pkgi18n.Localize(c, "success"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?lang=en", nil)
	req.Header.Set("Accept-Language", "zh-CN")
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "Success" {
		t.Errorf("body = %q, want %q (query lang should win over header)", got, "Success")
	}
}

func TestI18n_FallsBackToAcceptLanguageHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(I18n())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, pkgi18n.Localize(c, "success"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en")
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "Success" {
		t.Errorf("body = %q, want %q", got, "Success")
	}
}

func TestI18n_DefaultsToChineseWithoutAnyHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(I18n())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, pkgi18n.Localize(c, "success"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "成功" {
		t.Errorf("body = %q, want %q", got, "成功")
	}
}
