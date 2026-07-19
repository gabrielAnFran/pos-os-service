package entities

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allStatuses() []OrderStatus {
	return []OrderStatus{
		StatusCreated, StatusBudgeting, StatusAwaitingApproval, StatusApproved,
		StatusPaying, StatusPaid, StatusInExecution, StatusCompleted,
		StatusCancelled, StatusFailed,
	}
}

func TestTransitionTo_TableDriven(t *testing.T) {
	for _, from := range allStatuses() {
		allowed := validTransitions[from]
		allowedSet := map[OrderStatus]bool{}
		for _, s := range allowed {
			allowedSet[s] = true
		}

		for _, to := range allStatuses() {
			from, to := from, to
			name := string(from) + "->" + string(to)
			t.Run(name, func(t *testing.T) {
				o := NewOrder(uuid.New(), uuid.New(), "desc")
				o.Status = from

				prev, err := o.TransitionTo(to)

				if allowedSet[to] {
					require.NoError(t, err)
					assert.Equal(t, from, prev)
					assert.Equal(t, to, o.Status)
				} else {
					require.ErrorIs(t, err, ErrInvalidTransition)
					assert.Equal(t, from, o.Status, "status must not change on invalid transition")
				}
			})
		}
	}
}

func TestTransitionTo_TerminalStatesRejectEverything(t *testing.T) {
	terminal := []OrderStatus{StatusCompleted, StatusCancelled, StatusFailed}
	for _, from := range terminal {
		for _, to := range allStatuses() {
			o := NewOrder(uuid.New(), uuid.New(), "desc")
			o.Status = from
			_, err := o.TransitionTo(to)
			require.ErrorIs(t, err, ErrInvalidTransition, "%s -> %s should be rejected", from, to)
		}
	}
}

func TestNewOrder(t *testing.T) {
	customerID := uuid.New()
	vehicleID := uuid.New()
	o := NewOrder(customerID, vehicleID, "brake pads replacement")

	assert.NotEqual(t, uuid.Nil, o.ID)
	assert.Equal(t, customerID, o.CustomerID)
	assert.Equal(t, vehicleID, o.VehicleID)
	assert.Equal(t, StatusCreated, o.Status)
	assert.False(t, o.CreatedAt.IsZero())
}

func TestIsValidStatus(t *testing.T) {
	assert.True(t, IsValidStatus("CREATED"))
	assert.True(t, IsValidStatus("CANCELLED"))
	assert.False(t, IsValidStatus("NOT_A_STATUS"))
	assert.False(t, IsValidStatus(""))
}
