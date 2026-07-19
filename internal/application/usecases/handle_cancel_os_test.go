package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCancelCommand(t *testing.T, osID uuid.UUID, reason string) messaging.Event {
	t.Helper()
	ev, err := messaging.NewEvent("CancelOSCommand", "corr-x", "", cancelOSCommandPayload{OSID: osID.String(), Reason: reason})
	require.NoError(t, err)
	return ev
}

func TestHandleCancelOS_Success(t *testing.T) {
	repo := newFakeOrderRepository()
	processed := newFakeProcessedEventRepository()
	uc := NewHandleCancelOSUseCase(repo, processed)

	order := entities.NewOrder(uuid.New(), uuid.New(), "desc")
	require.NoError(t, repo.Save(context.Background(), order, nil, nil))

	ev := newCancelCommand(t, order.ID, "customer changed mind")

	require.NoError(t, uc.Handle(context.Background(), ev))

	saved, err := repo.FindByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, entities.StatusCancelled, saved.Status)

	history := repo.history[order.ID]
	require.Len(t, history, 1)
	assert.Equal(t, "CREATED", history[0].FromStatus)
	assert.Equal(t, "CANCELLED", history[0].ToStatus)
	assert.Equal(t, "customer changed mind", history[0].Reason)

	require.Len(t, repo.outbox, 1)
	var env messaging.Event
	require.NoError(t, json.Unmarshal(repo.outbox[0].Payload, &env))
	assert.Equal(t, eventOSCancelled, env.EventName)

	eventID, _ := uuid.Parse(ev.EventID)
	isProcessed, err := processed.IsProcessed(context.Background(), eventID)
	require.NoError(t, err)
	assert.True(t, isProcessed)
}

func TestHandleCancelOS_Idempotent(t *testing.T) {
	repo := newFakeOrderRepository()
	processed := newFakeProcessedEventRepository()
	uc := NewHandleCancelOSUseCase(repo, processed)

	order := entities.NewOrder(uuid.New(), uuid.New(), "desc")
	require.NoError(t, repo.Save(context.Background(), order, nil, nil))

	ev := newCancelCommand(t, order.ID, "reason")

	require.NoError(t, uc.Handle(context.Background(), ev))
	require.NoError(t, uc.Handle(context.Background(), ev))

	// Only one history row / outbox event should have been written.
	assert.Len(t, repo.history[order.ID], 1)
	assert.Len(t, repo.outbox, 1)
}

func TestHandleCancelOS_OrderNotFound(t *testing.T) {
	repo := newFakeOrderRepository()
	processed := newFakeProcessedEventRepository()
	uc := NewHandleCancelOSUseCase(repo, processed)

	ev := newCancelCommand(t, uuid.New(), "reason")

	err := uc.Handle(context.Background(), ev)
	require.NoError(t, err)
}

func TestHandleCancelOS_InvalidTransition(t *testing.T) {
	repo := newFakeOrderRepository()
	processed := newFakeProcessedEventRepository()
	uc := NewHandleCancelOSUseCase(repo, processed)

	order := entities.NewOrder(uuid.New(), uuid.New(), "desc")
	order.Status = entities.StatusCompleted
	require.NoError(t, repo.Save(context.Background(), order, nil, nil))

	ev := newCancelCommand(t, order.ID, "reason")

	err := uc.Handle(context.Background(), ev)
	require.Error(t, err)
}
