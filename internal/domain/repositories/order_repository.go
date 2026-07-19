package repositories

import (
	"context"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/google/uuid"
)

type OrderFilter struct {
	CustomerID *uuid.UUID
	Status     *entities.OrderStatus
	Limit      int
	Cursor     string
}

type OrderRepository interface {
	// Save persists the order, appends a status history row if the status changed,
	// and writes the outbox event in the same transaction.
	Save(ctx context.Context, order *entities.Order, history *entities.OrderStatusHistory, outboxEvent *OutboxEvent) error
	FindByID(ctx context.Context, id uuid.UUID) (*entities.Order, error)
	History(ctx context.Context, orderID uuid.UUID) ([]entities.OrderStatusHistory, error)
	List(ctx context.Context, filter OrderFilter) ([]entities.Order, string, error)
}

type OutboxEvent struct {
	EventID     uuid.UUID
	AggregateID uuid.UUID
	EventName   string
	Payload     []byte
	Headers     []byte
}

type OutboxRepository interface {
	FetchUnpublished(ctx context.Context, batch int) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, ids []int64) error
}

type OutboxRow struct {
	ID        int64
	EventID   uuid.UUID
	EventName string
	Payload   []byte
	Headers   []byte
}

type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*IdempotencyRecord, error)
	Save(ctx context.Context, rec *IdempotencyRecord) error
}

type IdempotencyRecord struct {
	Key          string
	RequestHash  string
	ResponseBody []byte
	StatusCode   int
}

type ProcessedEventRepository interface {
	IsProcessed(ctx context.Context, eventID uuid.UUID) (bool, error)
	MarkProcessed(ctx context.Context, eventID uuid.UUID) error
}
