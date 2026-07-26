package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielAnFran/pos-os-service/internal/application/usecases"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/dto"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestHandler(t *testing.T, repo *fakeOrderRepository) (*OrderHandler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	uc := usecases.NewCreateOrderUseCase(repo)
	return NewOrderHandler(uc, repo, db), db
}

func newTestEngine(h *OrderHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Correlation())
	v1 := r.Group("/api/v1")
	{
		v1.POST("/orders", h.CreateOrder)
		v1.GET("/orders", h.ListOrders)
		v1.GET("/orders/:id", h.GetOrder)
		v1.PATCH("/orders/:id/status", h.UpdateStatus)
	}
	r.GET("/healthz", h.Healthz)
	r.GET("/readyz", h.Readyz)
	return r
}

func doRequest(r *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	var reqBody *bytes.Reader
	if body != "" {
		reqBody = bytes.NewReader([]byte(body))
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateOrder_Success(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	body := fmt.Sprintf(`{"customer_id":%q,"vehicle_id":%q,"description":"oil change"}`, uuid.New(), uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/orders", body, map[string]string{middleware.IdempotencyKeyHeader: "key-1"})

	require.Equal(t, http.StatusCreated, w.Code)
	var resp dto.CreateOrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "CREATED", resp.Status)
	assert.NotEmpty(t, resp.OSID)
}

func TestCreateOrder_MissingIdempotencyKey(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPost, "/api/v1/orders", `{}`, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrder_InvalidBody(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPost, "/api/v1/orders", `not-json`, map[string]string{middleware.IdempotencyKeyHeader: "key-1"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrder_InvalidCustomerID(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	body := fmt.Sprintf(`{"customer_id":"not-a-uuid","vehicle_id":%q,"description":"oil change"}`, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/orders", body, map[string]string{middleware.IdempotencyKeyHeader: "key-1"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrder_InvalidVehicleID(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	body := fmt.Sprintf(`{"customer_id":%q,"vehicle_id":"not-a-uuid","description":"oil change"}`, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/orders", body, map[string]string{middleware.IdempotencyKeyHeader: "key-1"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrder_SaveError(t *testing.T) {
	repo := newFakeOrderRepository()
	repo.saveErr = errors.New("db down")
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	body := fmt.Sprintf(`{"customer_id":%q,"vehicle_id":%q,"description":"oil change"}`, uuid.New(), uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/orders", body, map[string]string{middleware.IdempotencyKeyHeader: "key-1"})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetOrder_Success(t *testing.T) {
	repo := newFakeOrderRepository()
	order := entities.NewOrder(uuid.New(), uuid.New(), "brake check")
	repo.orders[order.ID] = *order
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders/"+order.ID.String(), "", nil)

	require.Equal(t, http.StatusOK, w.Code)
	var resp dto.OrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, order.ID.String(), resp.OSID)
}

func TestGetOrder_InvalidID(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders/not-a-uuid", "", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetOrder_NotFound(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders/"+uuid.New().String(), "", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetOrder_RepoError(t *testing.T) {
	repo := newFakeOrderRepository()
	repo.findErr = errors.New("db down")
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders/"+uuid.New().String(), "", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetOrder_HistoryError(t *testing.T) {
	repo := newFakeOrderRepository()
	order := entities.NewOrder(uuid.New(), uuid.New(), "brake check")
	repo.orders[order.ID] = *order
	repo.historyErr = errors.New("db down")
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders/"+order.ID.String(), "", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateStatus_Success(t *testing.T) {
	repo := newFakeOrderRepository()
	order := entities.NewOrder(uuid.New(), uuid.New(), "brake check")
	repo.orders[order.ID] = *order
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+order.ID.String()+"/status", `{"status":"BUDGETING"}`, nil)

	require.Equal(t, http.StatusOK, w.Code)
	var resp dto.OrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "BUDGETING", resp.Status)
}

func TestUpdateStatus_InvalidID(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/not-a-uuid/status", `{"status":"BUDGETING"}`, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateStatus_InvalidBody(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+uuid.New().String()+"/status", `not-json`, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateStatus_UnknownStatusValue(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+uuid.New().String()+"/status", `{"status":"BOGUS"}`, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateStatus_NotFound(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+uuid.New().String()+"/status", `{"status":"BUDGETING"}`, nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateStatus_RepoFindError(t *testing.T) {
	repo := newFakeOrderRepository()
	repo.findErr = errors.New("db down")
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+uuid.New().String()+"/status", `{"status":"BUDGETING"}`, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateStatus_InvalidTransition(t *testing.T) {
	repo := newFakeOrderRepository()
	order := entities.NewOrder(uuid.New(), uuid.New(), "brake check")
	repo.orders[order.ID] = *order
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+order.ID.String()+"/status", `{"status":"COMPLETED"}`, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateStatus_SaveError(t *testing.T) {
	repo := newFakeOrderRepository()
	order := entities.NewOrder(uuid.New(), uuid.New(), "brake check")
	repo.orders[order.ID] = *order
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)
	repo.saveErr = errors.New("db down")

	w := doRequest(r, http.MethodPatch, "/api/v1/orders/"+order.ID.String()+"/status", `{"status":"BUDGETING"}`, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestListOrders_Success(t *testing.T) {
	repo := newFakeOrderRepository()
	order := entities.NewOrder(uuid.New(), uuid.New(), "brake check")
	repo.orders[order.ID] = *order
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders", "", nil)

	require.Equal(t, http.StatusOK, w.Code)
	var resp dto.OrderListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Orders, 1)
}

func TestListOrders_FilterByCustomerID(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	customerID := uuid.New()
	w := doRequest(r, http.MethodGet, "/api/v1/orders?customer_id="+customerID.String(), "", nil)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.lastListCall.CustomerID)
	assert.Equal(t, customerID, *repo.lastListCall.CustomerID)
}

func TestListOrders_InvalidCustomerID(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders?customer_id=not-a-uuid", "", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListOrders_FilterByStatus(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders?status=BUDGETING", "", nil)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.lastListCall.Status)
	assert.Equal(t, entities.StatusBudgeting, *repo.lastListCall.Status)
}

func TestListOrders_InvalidStatus(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders?status=BOGUS", "", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListOrders_WithLimit(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders?limit=5", "", nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 5, repo.lastListCall.Limit)
}

func TestListOrders_InvalidLimitIgnored(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders?limit=-1", "", nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, repo.lastListCall.Limit)
}

func TestListOrders_RepoError(t *testing.T) {
	repo := newFakeOrderRepository()
	repo.listErr = errors.New("bad cursor")
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/api/v1/orders", "", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHealthz(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/healthz", "", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

func TestReadyz_DBUp(t *testing.T) {
	repo := newFakeOrderRepository()
	h, _ := newTestHandler(t, repo)
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/readyz", "", nil)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestReadyz_DBDown(t *testing.T) {
	repo := newFakeOrderRepository()
	h, db := newTestHandler(t, repo)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	r := newTestEngine(h)

	w := doRequest(r, http.MethodGet, "/readyz", "", nil)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

var _ repositories.OrderRepository = (*fakeOrderRepository)(nil)
