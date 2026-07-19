package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/application/usecases"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/dto"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderHandler struct {
	createOrder *usecases.CreateOrderUseCase
	orders      repositories.OrderRepository
	db          *gorm.DB
}

func NewOrderHandler(createOrder *usecases.CreateOrderUseCase, orders repositories.OrderRepository, db *gorm.DB) *OrderHandler {
	return &OrderHandler{createOrder: createOrder, orders: orders, db: db}
}

func problem(c *gin.Context, status int, title, detail string) {
	c.AbortWithStatusJSON(status, dto.NewProblem(status, title, detail, c.Request.URL.Path))
}

func (h *OrderHandler) CreateOrder(c *gin.Context) {
	if c.GetHeader(middleware.IdempotencyKeyHeader) == "" {
		problem(c, http.StatusBadRequest, "Bad Request", "Idempotency-Key header is required")
		return
	}

	var req dto.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	customerID, err := uuid.Parse(req.CustomerID)
	if err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", "customer_id must be a valid UUID")
		return
	}
	vehicleID, err := uuid.Parse(req.VehicleID)
	if err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", "vehicle_id must be a valid UUID")
		return
	}

	order, err := h.createOrder.CreateOrder(c.Request.Context(), customerID, vehicleID, req.Description, middleware.CorrelationIDFromContext(c))
	if err != nil {
		problem(c, http.StatusInternalServerError, "Internal Server Error", "failed to create order")
		return
	}

	c.JSON(http.StatusCreated, dto.CreateOrderResponse{OSID: order.ID.String(), Status: string(order.Status)})
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", "id must be a valid UUID")
		return
	}

	order, err := h.orders.FindByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			problem(c, http.StatusNotFound, "Not Found", "order not found")
			return
		}
		problem(c, http.StatusInternalServerError, "Internal Server Error", "failed to fetch order")
		return
	}

	history, err := h.orders.History(c.Request.Context(), id)
	if err != nil {
		problem(c, http.StatusInternalServerError, "Internal Server Error", "failed to fetch order history")
		return
	}

	c.JSON(http.StatusOK, toOrderResponse(order, history))
}

func (h *OrderHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", "id must be a valid UUID")
		return
	}

	var req dto.UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if !entities.IsValidStatus(req.Status) {
		problem(c, http.StatusBadRequest, "Bad Request", "unknown status value")
		return
	}

	order, err := h.orders.FindByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			problem(c, http.StatusNotFound, "Not Found", "order not found")
			return
		}
		problem(c, http.StatusInternalServerError, "Internal Server Error", "failed to fetch order")
		return
	}

	prev, err := order.TransitionTo(entities.OrderStatus(req.Status))
	if err != nil {
		if errors.Is(err, entities.ErrInvalidTransition) {
			problem(c, http.StatusBadRequest, "Bad Request", "invalid status transition from "+string(prev)+" to "+req.Status)
			return
		}
		problem(c, http.StatusInternalServerError, "Internal Server Error", "failed to transition order")
		return
	}

	history := &entities.OrderStatusHistory{
		OrderID:    order.ID,
		FromStatus: string(prev),
		ToStatus:   string(order.Status),
		Reason:     req.Reason,
		ChangedAt:  time.Now().UTC(),
	}

	if err := h.orders.Save(c.Request.Context(), order, history, nil); err != nil {
		problem(c, http.StatusInternalServerError, "Internal Server Error", "failed to save order")
		return
	}

	c.JSON(http.StatusOK, toOrderResponse(order, nil))
}

func (h *OrderHandler) ListOrders(c *gin.Context) {
	filter := repositories.OrderFilter{Cursor: c.Query("cursor")}

	if v := c.Query("customer_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			problem(c, http.StatusBadRequest, "Bad Request", "customer_id must be a valid UUID")
			return
		}
		filter.CustomerID = &id
	}
	if v := c.Query("status"); v != "" {
		if !entities.IsValidStatus(v) {
			problem(c, http.StatusBadRequest, "Bad Request", "unknown status value")
			return
		}
		s := entities.OrderStatus(v)
		filter.Status = &s
	}
	if v := c.Query("limit"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			filter.Limit = n
		}
	}

	orders, nextCursor, err := h.orders.List(c.Request.Context(), filter)
	if err != nil {
		problem(c, http.StatusBadRequest, "Bad Request", "invalid pagination cursor")
		return
	}

	resp := dto.OrderListResponse{Orders: make([]dto.OrderResponse, 0, len(orders)), NextCursor: nextCursor}
	for i := range orders {
		resp.Orders = append(resp.Orders, toOrderResponse(&orders[i], nil))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *OrderHandler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *OrderHandler) Readyz(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func toOrderResponse(order *entities.Order, history []entities.OrderStatusHistory) dto.OrderResponse {
	resp := dto.OrderResponse{
		OSID:        order.ID.String(),
		CustomerID:  order.CustomerID.String(),
		VehicleID:   order.VehicleID.String(),
		Description: order.Description,
		Status:      string(order.Status),
		CreatedAt:   order.CreatedAt,
		UpdatedAt:   order.UpdatedAt,
	}
	for _, h := range history {
		resp.History = append(resp.History, dto.StatusHistoryEntry{
			FromStatus: h.FromStatus,
			ToStatus:   h.ToStatus,
			Reason:     h.Reason,
			ChangedAt:  h.ChangedAt,
		})
	}
	return resp
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, errors.New("must be positive")
	}
	return n, nil
}
