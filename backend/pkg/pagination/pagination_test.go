package pagination

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseQueryDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/items", nil)

	q := ParseQuery(c)
	if q.Page != DefaultPage || q.PageSize != DefaultPageSize {
		t.Fatalf("unexpected defaults: %+v", q)
	}
}

func TestParseQueryCapsPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/items?page=2&page_size=500", nil)

	q := ParseQuery(c)
	if q.Page != 2 || q.PageSize != MaxPageSize {
		t.Fatalf("unexpected query: %+v", q)
	}
	if q.Offset() != MaxPageSize {
		t.Fatalf("unexpected offset: %d", q.Offset())
	}
}
