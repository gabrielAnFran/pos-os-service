package handlers

import (
	"context"
	"sync"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/google/uuid"
)

// fakeOrderRepository is a simple in-memory implementation of
// repositories.OrderRepository for handler tests.
type fakeOrderRepository struct {
	mu           sync.Mutex
	orders       map[uuid.UUID]entities.Order
	history      map[uuid.UUID][]entities.OrderStatusHistory
	saveErr      error
	findErr      error
	historyErr   error
	listErr      error
	lastListCall repositories.OrderFilter
}

func newFakeOrderRepository() *fakeOrderRepository {
	return &fakeOrderRepository{
		orders:  map[uuid.UUID]entities.Order{},
		history: map[uuid.UUID][]entities.OrderStatusHistory{},
	}
}

func (f *fakeOrderRepository) Save(_ context.Context, order *entities.Order, history *entities.OrderStatusHistory, _ *repositories.OutboxEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.orders[order.ID] = *order
	if history != nil {
		f.history[order.ID] = append(f.history[order.ID], *history)
	}
	return nil
}

func (f *fakeOrderRepository) FindByID(_ context.Context, id uuid.UUID) (*entities.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.findErr != nil {
		return nil, f.findErr
	}
	o, ok := f.orders[id]
	if !ok {
		return nil, repositories.ErrNotFound
	}
	return &o, nil
}

func (f *fakeOrderRepository) History(_ context.Context, orderID uuid.UUID) ([]entities.OrderStatusHistory, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history[orderID], nil
}

func (f *fakeOrderRepository) List(_ context.Context, filter repositories.OrderFilter) ([]entities.Order, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastListCall = filter
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	out := make([]entities.Order, 0, len(f.orders))
	for _, o := range f.orders {
		out = append(out, o)
	}
	return out, "", nil
}
