package usecases

import (
	"context"
	"sync"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/google/uuid"
)

// fakeOrderRepository is a simple in-memory implementation of
// repositories.OrderRepository for unit tests.
type fakeOrderRepository struct {
	mu      sync.Mutex
	orders  map[uuid.UUID]entities.Order
	history map[uuid.UUID][]entities.OrderStatusHistory
	outbox  []repositories.OutboxEvent
	saveErr error
}

func newFakeOrderRepository() *fakeOrderRepository {
	return &fakeOrderRepository{
		orders:  map[uuid.UUID]entities.Order{},
		history: map[uuid.UUID][]entities.OrderStatusHistory{},
	}
}

func (f *fakeOrderRepository) Save(_ context.Context, order *entities.Order, history *entities.OrderStatusHistory, outboxEvent *repositories.OutboxEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.orders[order.ID] = *order
	if history != nil {
		f.history[order.ID] = append(f.history[order.ID], *history)
	}
	if outboxEvent != nil {
		f.outbox = append(f.outbox, *outboxEvent)
	}
	return nil
}

func (f *fakeOrderRepository) FindByID(_ context.Context, id uuid.UUID) (*entities.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.orders[id]
	if !ok {
		return nil, repositories.ErrNotFound
	}
	return &o, nil
}

func (f *fakeOrderRepository) History(_ context.Context, orderID uuid.UUID) ([]entities.OrderStatusHistory, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history[orderID], nil
}

func (f *fakeOrderRepository) List(_ context.Context, _ repositories.OrderFilter) ([]entities.Order, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]entities.Order, 0, len(f.orders))
	for _, o := range f.orders {
		out = append(out, o)
	}
	return out, "", nil
}

// fakeProcessedEventRepository is an in-memory ProcessedEventRepository.
type fakeProcessedEventRepository struct {
	mu        sync.Mutex
	processed map[uuid.UUID]bool
}

func newFakeProcessedEventRepository() *fakeProcessedEventRepository {
	return &fakeProcessedEventRepository{processed: map[uuid.UUID]bool{}}
}

func (f *fakeProcessedEventRepository) IsProcessed(_ context.Context, eventID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.processed[eventID], nil
}

func (f *fakeProcessedEventRepository) MarkProcessed(_ context.Context, eventID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.processed[eventID] = true
	return nil
}
