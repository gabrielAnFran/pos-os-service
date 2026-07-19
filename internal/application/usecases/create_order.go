package usecases

import (
	"context"
	"fmt"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
)

const eventOSCreated = "OSCreated"

type CreateOrderUseCase struct {
	orders repositories.OrderRepository
}

func NewCreateOrderUseCase(orders repositories.OrderRepository) *CreateOrderUseCase {
	return &CreateOrderUseCase{orders: orders}
}

type osCreatedPayload struct {
	OSID        string `json:"os_id"`
	CustomerID  string `json:"customer_id"`
	VehicleID   string `json:"vehicle_id"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

func (uc *CreateOrderUseCase) CreateOrder(ctx context.Context, customerID, vehicleID uuid.UUID, description, correlationID string) (*entities.Order, error) {
	order := entities.NewOrder(customerID, vehicleID, description)

	payload := osCreatedPayload{
		OSID:        order.ID.String(),
		CustomerID:  order.CustomerID.String(),
		VehicleID:   order.VehicleID.String(),
		Description: order.Description,
		CreatedAt:   order.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}

	env, err := messaging.NewEvent(eventOSCreated, correlationID, "", payload)
	if err != nil {
		return nil, fmt.Errorf("build event: %w", err)
	}
	envelope, err := marshalEnvelope(env)
	if err != nil {
		return nil, err
	}

	outboxEvent := &repositories.OutboxEvent{
		EventID:     uuid.MustParse(env.EventID),
		AggregateID: order.ID,
		EventName:   env.EventName,
		Payload:     envelope,
		Headers:     []byte(`{"content-type":"application/json"}`),
	}

	if err := uc.orders.Save(ctx, order, nil, outboxEvent); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}
	return order, nil
}
