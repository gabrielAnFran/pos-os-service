//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/application/usecases"
	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestCreateOrder_OutboxDispatch_EndToEnd exercises CreateOrder against the
// shared Postgres and RabbitMQ containers, confirms an outbox row is
// written, then runs one dispatch cycle and verifies the OSCreated event
// reaches a queue bound to pos.events with routing key OSCreated. Run with:
//
//	go test -tags integration ./tests/integration/...
func TestCreateOrder_OutboxDispatch_EndToEnd(t *testing.T) {
	ctx := context.Background()
	gormDB := newTestDB(t)
	truncateAll(t, gormDB)

	repo := infradb.NewOrderRepository(gormDB)
	createOrderUC := usecases.NewCreateOrderUseCase(repo)

	order, err := createOrderUC.CreateOrder(ctx, uuid.New(), uuid.New(), "integration test order", "corr-it")
	require.NoError(t, err)

	rows, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "OSCreated", rows[0].EventName)

	amqpConn, err := messaging.Dial(testAMQPURL)
	require.NoError(t, err)
	defer amqpConn.Close()

	ch := amqpConn.Channel()
	q, err := ch.QueueDeclare("it-test-q", false, true, true, false, nil)
	require.NoError(t, err)
	require.NoError(t, ch.QueueBind(q.Name, "OSCreated", messaging.EventsExchange, false, nil))

	var ev messaging.Event
	require.NoError(t, json.Unmarshal(rows[0].Payload, &ev))
	require.NoError(t, amqpConn.Publish(ctx, ev))
	require.NoError(t, repo.MarkPublished(ctx, []int64{rows[0].ID}))

	msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	require.NoError(t, err)

	select {
	case d := <-msgs:
		var received messaging.Event
		require.NoError(t, json.Unmarshal(d.Body, &received))
		require.Equal(t, "OSCreated", received.EventName)
		require.Equal(t, order.ID.String(), mustExtractOSID(t, received))
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for OSCreated event")
	}
}

func mustExtractOSID(t *testing.T, ev messaging.Event) string {
	t.Helper()
	var payload struct {
		OSID string `json:"os_id"`
	}
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	return payload.OSID
}
