package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIdempotencyRepo struct {
	mu      sync.Mutex
	records map[string]repositories.IdempotencyRecord
}

func newFakeIdempotencyRepo() *fakeIdempotencyRepo {
	return &fakeIdempotencyRepo{records: map[string]repositories.IdempotencyRecord{}}
}

func (f *fakeIdempotencyRepo) Get(_ context.Context, key string) (*repositories.IdempotencyRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[key]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}

func (f *fakeIdempotencyRepo) Save(_ context.Context, rec *repositories.IdempotencyRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[rec.Key] = *rec
	return nil
}

func newTestRouter(repo repositories.IdempotencyRepository) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	calls := 0
	r.POST("/orders", Idempotency(repo), func(c *gin.Context) {
		calls++
		c.JSON(http.StatusCreated, gin.H{"os_id": "abc", "call": calls})
	})
	return r, &calls
}

func TestIdempotency_NewKeyStoresAndReturns(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	r, calls := newTestRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"foo":"bar"}`))
	req.Header.Set(IdempotencyKeyHeader, "key-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, *calls)

	rec, err := repo.Get(context.Background(), "key-1")
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, http.StatusCreated, rec.StatusCode)
}

func TestIdempotency_SameKeySameHashReplays(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	r, calls := newTestRouter(repo)

	body := `{"foo":"bar"}`
	req1 := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	req1.Header.Set(IdempotencyKeyHeader, "key-2")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	req2.Header.Set(IdempotencyKeyHeader, "key-2")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusCreated, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())
	assert.Equal(t, 1, *calls, "handler must only run once")
}

func TestIdempotency_SameKeyDifferentHashConflicts(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	r, calls := newTestRouter(repo)

	req1 := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"foo":"bar"}`))
	req1.Header.Set(IdempotencyKeyHeader, "key-3")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"foo":"different"}`))
	req2.Header.Set(IdempotencyKeyHeader, "key-3")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusConflict, w2.Code)
	assert.Equal(t, 1, *calls, "handler must not run for conflicting request")
}

func TestIdempotency_NoHeaderPassesThrough(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	r, calls := newTestRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"foo":"bar"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, *calls)
}
