package entities

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	StatusCreated          OrderStatus = "CREATED"
	StatusBudgeting        OrderStatus = "BUDGETING"
	StatusAwaitingApproval OrderStatus = "AWAITING_APPROVAL"
	StatusApproved         OrderStatus = "APPROVED"
	StatusPaying           OrderStatus = "PAYING"
	StatusPaid             OrderStatus = "PAID"
	StatusInExecution      OrderStatus = "IN_EXECUTION"
	StatusCompleted        OrderStatus = "COMPLETED"
	StatusCancelled        OrderStatus = "CANCELLED"
	StatusFailed           OrderStatus = "FAILED"
)

var validTransitions = map[OrderStatus][]OrderStatus{
	StatusCreated:          {StatusBudgeting, StatusCancelled, StatusFailed},
	StatusBudgeting:        {StatusAwaitingApproval, StatusCancelled, StatusFailed},
	StatusAwaitingApproval: {StatusApproved, StatusCancelled, StatusFailed},
	StatusApproved:         {StatusPaying, StatusCancelled, StatusFailed},
	StatusPaying:           {StatusPaid, StatusCancelled, StatusFailed},
	StatusPaid:             {StatusInExecution, StatusCancelled, StatusFailed},
	StatusInExecution:      {StatusCompleted, StatusCancelled, StatusFailed},
	StatusCompleted:        {},
	StatusCancelled:        {},
	StatusFailed:           {},
}

var ErrInvalidTransition = errors.New("invalid order status transition")

type Order struct {
	ID          uuid.UUID
	CustomerID  uuid.UUID
	VehicleID   uuid.UUID
	Description string
	Status      OrderStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewOrder(customerID, vehicleID uuid.UUID, description string) *Order {
	now := time.Now()
	return &Order{
		ID:          uuid.New(),
		CustomerID:  customerID,
		VehicleID:   vehicleID,
		Description: description,
		Status:      StatusCreated,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// TransitionTo validates and applies a status change, returning the previous status.
func (o *Order) TransitionTo(next OrderStatus) (OrderStatus, error) {
	allowed, ok := validTransitions[o.Status]
	if !ok {
		return o.Status, ErrInvalidTransition
	}
	found := false
	for _, s := range allowed {
		if s == next {
			found = true
			break
		}
	}
	if !found {
		return o.Status, ErrInvalidTransition
	}
	prev := o.Status
	o.Status = next
	o.UpdatedAt = time.Now()
	return prev, nil
}

func IsValidStatus(s string) bool {
	_, ok := validTransitions[OrderStatus(s)]
	return ok
}

type OrderStatusHistory struct {
	ID         int64
	OrderID    uuid.UUID
	FromStatus string
	ToStatus   string
	Reason     string
	ChangedAt  time.Time
}
