package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newCorrelationRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Correlation())
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"correlation_id": CorrelationIDFromContext(c)})
	})
	return r
}

func TestCorrelation_GeneratesIDWhenAbsent(t *testing.T) {
	r := newCorrelationRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	id := w.Header().Get(CorrelationIDHeader)
	if id == "" {
		t.Fatal("expected a generated correlation id header")
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("generated correlation id %q is not a valid uuid: %v", id, err)
	}
}

func TestCorrelation_EchoesExistingID(t *testing.T) {
	r := newCorrelationRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(CorrelationIDHeader, "given-id-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get(CorrelationIDHeader); got != "given-id-123" {
		t.Errorf("correlation id header = %q, want %q", got, "given-id-123")
	}
}

func TestCorrelationIDFromContext_MissingReturnsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/ping", nil)

	if got := CorrelationIDFromContext(c); got != "" {
		t.Errorf("CorrelationIDFromContext() = %q, want empty string", got)
	}
}
