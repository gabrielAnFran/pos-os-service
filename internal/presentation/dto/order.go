package dto

import "time"

type CreateOrderRequest struct {
	CustomerID  string `json:"customer_id" binding:"required,uuid"`
	VehicleID   string `json:"vehicle_id" binding:"required,uuid"`
	Description string `json:"description" binding:"required"`
}

type CreateOrderResponse struct {
	OSID   string `json:"os_id"`
	Status string `json:"status"`
}

type StatusHistoryEntry struct {
	FromStatus string    `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status"`
	Reason     string    `json:"reason,omitempty"`
	ChangedAt  time.Time `json:"changed_at"`
}

type OrderResponse struct {
	OSID        string               `json:"os_id"`
	CustomerID  string               `json:"customer_id"`
	VehicleID   string               `json:"vehicle_id"`
	Description string               `json:"description"`
	Status      string               `json:"status"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	History     []StatusHistoryEntry `json:"history,omitempty"`
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason,omitempty"`
}

type OrderListResponse struct {
	Orders     []OrderResponse `json:"orders"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
