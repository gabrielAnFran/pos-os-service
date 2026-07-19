package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
)

const eventOSCancelled = "OSCancelled"

type HandleCancelOSUseCase struct {
	orders          repositories.OrderRepository
	processedEvents repositories.ProcessedEventRepository
}

func NewHandleCancelOSUseCase(orders repositories.OrderRepository, processedEvents repositories.ProcessedEventRepository) *HandleCancelOSUseCase {
	return &HandleCancelOSUseCase{orders: orders, processedEvents: processedEvents}
}

type cancelOSCommandPayload struct {
	OSID   string `json:"os_id"`
	Reason string `json:"reason"`
}

type osCancelledPayload struct {
	OSID        string `json:"os_id"`
	CancelledAt string `json:"cancelled_at"`
	Reason      string `json:"reason"`
}

// Handle processes a CancelOSCommand event, idempotently. It checks the
// processed_events table before doing any work, and marks the event as
// processed after a successful transition; the table's primary key on
// event_id is also a safety net against races between the check and mark.
func (uc *HandleCancelOSUseCase) Handle(ctx context.Context, ev messaging.Event) error {
	eventID, err := uuid.Parse(ev.EventID)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	processed, err := uc.processedEvents.IsProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("check processed: %w", err)
	}
	if processed {
		return nil
	}

	var cmd cancelOSCommandPayload
	if err := json.Unmarshal(ev.Payload, &cmd); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	osID, err := uuid.Parse(cmd.OSID)
	if err != nil {
		return fmt.Errorf("invalid os_id: %w", err)
	}

	order, err := uc.orders.FindByID(ctx, osID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil // nothing to compensate, treat as processed
		}
		return fmt.Errorf("find order: %w", err)
	}

	prev, err := order.TransitionTo(entities.StatusCancelled)
	if err != nil {
		return fmt.Errorf("transition order: %w", err)
	}

	now := time.Now().UTC()
	history := &entities.OrderStatusHistory{
		OrderID:    order.ID,
		FromStatus: string(prev),
		ToStatus:   string(entities.StatusCancelled),
		Reason:     cmd.Reason,
		ChangedAt:  now,
	}

	payload := osCancelledPayload{
		OSID:        order.ID.String(),
		CancelledAt: now.Format("2006-01-02T15:04:05.000Z07:00"),
		Reason:      cmd.Reason,
	}
	outEv, err := messaging.NewEvent(eventOSCancelled, ev.CorrelationID, ev.SagaID, payload)
	if err != nil {
		return fmt.Errorf("build event: %w", err)
	}
	envelope, err := marshalEnvelope(outEv)
	if err != nil {
		return err
	}
	outboxEvent := &repositories.OutboxEvent{
		EventID:     uuid.MustParse(outEv.EventID),
		AggregateID: order.ID,
		EventName:   outEv.EventName,
		Payload:     envelope,
		Headers:     []byte(`{"content-type":"application/json"}`),
	}

	if err := uc.orders.Save(ctx, order, history, outboxEvent); err != nil {
		return fmt.Errorf("save order: %w", err)
	}

	if err := uc.processedEvents.MarkProcessed(ctx, eventID); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}
