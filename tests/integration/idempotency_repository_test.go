//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyRepository_GetMissingReturnsNil(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewIdempotencyRepository(db)

	rec, err := repo.Get(context.Background(), "missing-key")
	require.NoError(t, err)
	assert.Nil(t, rec)
}

func TestIdempotencyRepository_SaveAndGet_UpsertsOnConflict(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewIdempotencyRepository(db)
	ctx := context.Background()

	rec := &repositories.IdempotencyRecord{Key: "k1", RequestHash: "h1", ResponseBody: []byte(`{"a":1}`), StatusCode: 201}
	require.NoError(t, repo.Save(ctx, rec))

	got, err := repo.Get(ctx, "k1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "h1", got.RequestHash)
	assert.Equal(t, 201, got.StatusCode)

	// Saving the same key again (replay with a different hash, as the
	// idempotency middleware would after a stored record is overwritten)
	// must upsert rather than error.
	rec2 := &repositories.IdempotencyRecord{Key: "k1", RequestHash: "h2", ResponseBody: []byte(`{"a":2}`), StatusCode: 409}
	require.NoError(t, repo.Save(ctx, rec2))

	got2, err := repo.Get(ctx, "k1")
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, "h2", got2.RequestHash)
	assert.Equal(t, 409, got2.StatusCode)
}
