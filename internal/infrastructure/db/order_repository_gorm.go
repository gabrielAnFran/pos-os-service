package db

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/entities"
	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultIdempotencyTTL = 24 * time.Hour

type orderModel struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	CustomerID  uuid.UUID `gorm:"column:customer_id"`
	VehicleID   uuid.UUID `gorm:"column:vehicle_id"`
	Description string    `gorm:"column:description"`
	Status      string    `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (orderModel) TableName() string { return "orders" }

type statusHistoryModel struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	OrderID    uuid.UUID `gorm:"column:order_id"`
	FromStatus *string   `gorm:"column:from_status"`
	ToStatus   string    `gorm:"column:to_status"`
	Reason     *string   `gorm:"column:reason"`
	ChangedAt  time.Time `gorm:"column:changed_at"`
}

func (statusHistoryModel) TableName() string { return "order_status_history" }

type outboxModel struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	EventID     uuid.UUID  `gorm:"column:event_id"`
	AggregateID uuid.UUID  `gorm:"column:aggregate_id"`
	EventName   string     `gorm:"column:event_name"`
	Payload     []byte     `gorm:"column:payload;type:jsonb"`
	Headers     []byte     `gorm:"column:headers;type:jsonb"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	PublishedAt *time.Time `gorm:"column:published_at"`
}

func (outboxModel) TableName() string { return "outbox" }

type idempotencyModel struct {
	Key          string    `gorm:"column:key;primaryKey"`
	RequestHash  string    `gorm:"column:request_hash"`
	ResponseBody []byte    `gorm:"column:response_body;type:jsonb"`
	StatusCode   int       `gorm:"column:status_code"`
	ExpiresAt    time.Time `gorm:"column:expires_at"`
}

func (idempotencyModel) TableName() string { return "idempotency_keys" }

type processedEventModel struct {
	EventID     uuid.UUID `gorm:"column:event_id;primaryKey"`
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

func (processedEventModel) TableName() string { return "processed_events" }

// OrderRepository is a GORM-backed implementation of the domain repository
// interfaces defined in internal/domain/repositories.
type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func toOrderModel(o *entities.Order) orderModel {
	return orderModel{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		VehicleID:   o.VehicleID,
		Description: o.Description,
		Status:      string(o.Status),
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

func fromOrderModel(m orderModel) entities.Order {
	return entities.Order{
		ID:          m.ID,
		CustomerID:  m.CustomerID,
		VehicleID:   m.VehicleID,
		Description: m.Description,
		Status:      entities.OrderStatus(m.Status),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func (r *OrderRepository) Save(ctx context.Context, order *entities.Order, history *entities.OrderStatusHistory, outboxEvent *repositories.OutboxEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := toOrderModel(order)
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"customer_id", "vehicle_id", "description", "status", "updated_at"}),
		}).Create(&m).Error; err != nil {
			return fmt.Errorf("save order: %w", err)
		}

		if history != nil {
			hm := statusHistoryModel{
				OrderID:   history.OrderID,
				ToStatus:  history.ToStatus,
				ChangedAt: history.ChangedAt,
			}
			if history.FromStatus != "" {
				hm.FromStatus = &history.FromStatus
			}
			if history.Reason != "" {
				hm.Reason = &history.Reason
			}
			if err := tx.Create(&hm).Error; err != nil {
				return fmt.Errorf("save history: %w", err)
			}
		}

		if outboxEvent != nil {
			om := outboxModel{
				EventID:     outboxEvent.EventID,
				AggregateID: outboxEvent.AggregateID,
				EventName:   outboxEvent.EventName,
				Payload:     outboxEvent.Payload,
				Headers:     outboxEvent.Headers,
				CreatedAt:   time.Now().UTC(),
			}
			if err := tx.Create(&om).Error; err != nil {
				return fmt.Errorf("save outbox event: %w", err)
			}
		}
		return nil
	})
}

func (r *OrderRepository) FindByID(ctx context.Context, id uuid.UUID) (*entities.Order, error) {
	var m orderModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, repositories.ErrNotFound
		}
		return nil, err
	}
	o := fromOrderModel(m)
	return &o, nil
}

func (r *OrderRepository) History(ctx context.Context, orderID uuid.UUID) ([]entities.OrderStatusHistory, error) {
	var rows []statusHistoryModel
	if err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("changed_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]entities.OrderStatusHistory, 0, len(rows))
	for _, row := range rows {
		h := entities.OrderStatusHistory{
			ID:        row.ID,
			OrderID:   row.OrderID,
			ToStatus:  row.ToStatus,
			ChangedAt: row.ChangedAt,
		}
		if row.FromStatus != nil {
			h.FromStatus = *row.FromStatus
		}
		if row.Reason != nil {
			h.Reason = *row.Reason
		}
		out = append(out, h)
	}
	return out, nil
}

type orderCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s|%s", createdAt.UTC().Format(time.RFC3339Nano), id.String())
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (*orderCursor, error) {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	parts := splitOnce(string(raw), '|')
	if parts == nil {
		return nil, fmt.Errorf("malformed cursor")
	}
	createdAtStr, idStr := parts[0], parts[1]
	t, err := time.Parse(time.RFC3339Nano, createdAtStr)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &orderCursor{CreatedAt: t, ID: id}, nil
}

func splitOnce(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}

func (r *OrderRepository) List(ctx context.Context, filter repositories.OrderFilter) ([]entities.Order, string, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	q := r.db.WithContext(ctx).Model(&orderModel{})
	if filter.CustomerID != nil {
		q = q.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}
	if filter.Cursor != "" {
		c, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []orderModel
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, "", err
	}

	out := make([]entities.Order, 0, len(rows))
	for _, m := range rows {
		out = append(out, fromOrderModel(m))
	}

	nextCursor := ""
	if len(rows) == limit {
		last := rows[len(rows)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return out, nextCursor, nil
}

// OutboxRepository implementation.

func (r *OrderRepository) FetchUnpublished(ctx context.Context, batch int) ([]repositories.OutboxRow, error) {
	var rows []outboxModel
	if err := r.db.WithContext(ctx).Where("published_at IS NULL").Order("created_at ASC").Limit(batch).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]repositories.OutboxRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, repositories.OutboxRow{
			ID:        m.ID,
			EventID:   m.EventID,
			EventName: m.EventName,
			Payload:   m.Payload,
			Headers:   m.Headers,
		})
	}
	return out, nil
}

func (r *OrderRepository) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&outboxModel{}).Where("id IN ?", ids).Update("published_at", now).Error
}

// ProcessedEventRepository implementation.

func (r *OrderRepository) IsProcessed(ctx context.Context, eventID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&processedEventModel{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *OrderRepository) MarkProcessed(ctx context.Context, eventID uuid.UUID) error {
	m := processedEventModel{EventID: eventID, ProcessedAt: time.Now().UTC()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error
}

// IdempotencyRepository is a separate GORM-backed type (distinct receiver from
// OrderRepository) since both interfaces define a differently-shaped Save method.
type IdempotencyRepository struct {
	db *gorm.DB
}

func NewIdempotencyRepository(db *gorm.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) Get(ctx context.Context, key string) (*repositories.IdempotencyRecord, error) {
	var m idempotencyModel
	if err := r.db.WithContext(ctx).First(&m, "key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &repositories.IdempotencyRecord{
		Key:          m.Key,
		RequestHash:  m.RequestHash,
		ResponseBody: m.ResponseBody,
		StatusCode:   m.StatusCode,
	}, nil
}

func (r *IdempotencyRepository) Save(ctx context.Context, rec *repositories.IdempotencyRecord) error {
	m := idempotencyModel{
		Key:          rec.Key,
		RequestHash:  rec.RequestHash,
		ResponseBody: rec.ResponseBody,
		StatusCode:   rec.StatusCode,
		ExpiresAt:    time.Now().UTC().Add(defaultIdempotencyTTL),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_hash", "response_body", "status_code", "expires_at"}),
	}).Create(&m).Error
}
