package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielAnFran/pos-os-service/internal/presentation/dto"
	"github.com/gin-gonic/gin"
)

func TestRecovery_PanicRendersProblemJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/boom", func(c *gin.Context) {
		panic("something broke")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var p dto.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("response body is not valid problem+json: %v", err)
	}
	if p.Status != http.StatusInternalServerError {
		t.Errorf("Problem.Status = %d, want %d", p.Status, http.StatusInternalServerError)
	}
	if p.Title != "Internal Server Error" {
		t.Errorf("Problem.Title = %q, want %q", p.Title, "Internal Server Error")
	}
	if p.Instance != "/boom" {
		t.Errorf("Problem.Instance = %q, want %q", p.Instance, "/boom")
	}
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
