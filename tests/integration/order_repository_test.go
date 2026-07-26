//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderRepository_SaveFindHistory(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)
	ctx := context.Background()

	order := entities.NewOrder(uuid.New(), uuid.New(), "align wheels")
	require.NoError(t, repo.Save(ctx, order, nil, nil))

	found, err := repo.FindByID(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, order.CustomerID, found.CustomerID)
	assert.Equal(t, entities.StatusCreated, found.Status)

	prev, err := found.TransitionTo(entities.StatusBudgeting)
	require.NoError(t, err)
	hist := &entities.OrderStatusHistory{
		OrderID:    found.ID,
		FromStatus: string(prev),
		ToStatus:   string(found.Status),
		ChangedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.Save(ctx, found, hist, nil))

	rows, err := repo.History(ctx, order.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "CREATED", rows[0].FromStatus)
	assert.Equal(t, "BUDGETING", rows[0].ToStatus)

	refetched, err := repo.FindByID(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, entities.StatusBudgeting, refetched.Status)
}

func TestOrderRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)

	_, err := repo.FindByID(context.Background(), uuid.New())
	assert.ErrorIs(t, err, repositories.ErrNotFound)
}

func TestOrderRepository_History_EmptyForUnknownOrder(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)

	rows, err := repo.History(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestOrderRepository_List_FiltersAndPagination(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)
	ctx := context.Background()

	customerID := uuid.New()
	for i := 0; i < 3; i++ {
		o := entities.NewOrder(customerID, uuid.New(), "job")
		require.NoError(t, repo.Save(ctx, o, nil, nil))
		time.Sleep(5 * time.Millisecond) // force distinct created_at for stable ordering
	}
	// A different customer's order must be excluded by the filter.
	other := entities.NewOrder(uuid.New(), uuid.New(), "other")
	require.NoError(t, repo.Save(ctx, other, nil, nil))

	page1, cursor1, err := repo.List(ctx, repositories.OrderFilter{CustomerID: &customerID, Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, cursor1)

	page2, cursor2, err := repo.List(ctx, repositories.OrderFilter{CustomerID: &customerID, Limit: 2, Cursor: cursor1})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Empty(t, cursor2)

	_, _, err = repo.List(ctx, repositories.OrderFilter{Cursor: "not-valid-base64!!"})
	assert.Error(t, err)
}

func TestOrderRepository_List_FilterByStatus(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)
	ctx := context.Background()

	created := entities.NewOrder(uuid.New(), uuid.New(), "job-created")
	require.NoError(t, repo.Save(ctx, created, nil, nil))

	budgeting := entities.NewOrder(uuid.New(), uuid.New(), "job-budgeting")
	_, err := budgeting.TransitionTo(entities.StatusBudgeting)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, budgeting, nil, nil))

	status := entities.StatusBudgeting
	rows, _, err := repo.List(ctx, repositories.OrderFilter{Status: &status})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, budgeting.ID, rows[0].ID)
}

func TestOrderRepository_Outbox_FetchUnpublished_MarkPublished(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)
	ctx := context.Background()

	order := entities.NewOrder(uuid.New(), uuid.New(), "job")
	outboxEvent := &repositories.OutboxEvent{
		EventID:     uuid.New(),
		AggregateID: order.ID,
		EventName:   "TestEvent",
		Payload:     []byte(`{"a":1}`),
		Headers:     []byte(`{}`),
	}
	require.NoError(t, repo.Save(ctx, order, nil, outboxEvent))

	rows, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "TestEvent", rows[0].EventName)

	require.NoError(t, repo.MarkPublished(ctx, []int64{rows[0].ID}))
	require.NoError(t, repo.MarkPublished(ctx, nil)) // no-op path must not error

	rows2, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, rows2)
}

func TestOrderRepository_ProcessedEvents(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewOrderRepository(db)
	ctx := context.Background()

	eventID := uuid.New()
	processed, err := repo.IsProcessed(ctx, eventID)
	require.NoError(t, err)
	assert.False(t, processed)

	require.NoError(t, repo.MarkProcessed(ctx, eventID))
	require.NoError(t, repo.MarkProcessed(ctx, eventID)) // must be idempotent on conflict

	processed, err = repo.IsProcessed(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, processed)
}
