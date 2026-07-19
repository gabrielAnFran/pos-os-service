package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateOrder_Success(t *testing.T) {
	repo := newFakeOrderRepository()
	uc := NewCreateOrderUseCase(repo)

	customerID := uuid.New()
	vehicleID := uuid.New()

	order, err := uc.CreateOrder(context.Background(), customerID, vehicleID, "oil change", "corr-1")

	require.NoError(t, err)
	assert.Equal(t, entities.StatusCreated, order.Status)
	assert.Equal(t, customerID, order.CustomerID)
	assert.Equal(t, vehicleID, order.VehicleID)

	saved, err := repo.FindByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, order.ID, saved.ID)

	require.Len(t, repo.outbox, 1)
	assert.Equal(t, order.ID, repo.outbox[0].AggregateID)

	var env messaging.Event
	require.NoError(t, json.Unmarshal(repo.outbox[0].Payload, &env))
	assert.Equal(t, eventOSCreated, env.EventName)
	assert.Equal(t, "corr-1", env.CorrelationID)

	var payload osCreatedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, order.ID.String(), payload.OSID)
	assert.Equal(t, "oil change", payload.Description)
}

func TestCreateOrder_SaveFails(t *testing.T) {
	repo := newFakeOrderRepository()
	repo.saveErr = errors.New("db down")
	uc := NewCreateOrderUseCase(repo)

	_, err := uc.CreateOrder(context.Background(), uuid.New(), uuid.New(), "desc", "corr-2")

	require.Error(t, err)
}
