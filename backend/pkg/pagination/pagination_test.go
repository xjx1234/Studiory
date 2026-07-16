package pagination

import (
	"math"
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

func TestLimitInt32_MatchesPageSize(t *testing.T) {
	q := Query{Page: 1, PageSize: 20}
	if got := q.LimitInt32(); got != 20 {
		t.Errorf("LimitInt32() = %d, want 20", got)
	}
}

func TestOffsetInt32_MatchesOffset(t *testing.T) {
	q := Query{Page: 3, PageSize: 20}
	want := int32(q.Offset())
	if got := q.OffsetInt32(); got != want {
		t.Errorf("OffsetInt32() = %d, want %d", got, want)
	}
}

// TestOffsetInt32_ClampsOnOverflow 覆盖 Query 未经 ParseQuery 直接构造、
// Page 异常偏大导致 (Page-1)*PageSize 超出 int32 范围的场景（对应 gosec G115）。
func TestOffsetInt32_ClampsOnOverflow(t *testing.T) {
	q := Query{Page: math.MaxInt32, PageSize: 100}
	if got := q.OffsetInt32(); got != math.MaxInt32 {
		t.Errorf("OffsetInt32() = %d, want clamp to %d", got, int32(math.MaxInt32))
	}
}

func TestLimitInt32_ClampsNegativePageSizeToZero(t *testing.T) {
	q := Query{Page: 1, PageSize: -5}
	if got := q.LimitInt32(); got != 0 {
		t.Errorf("LimitInt32() = %d, want 0 for negative PageSize", got)
	}
}
