//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/application/usecases"
	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestCreateOrder_OutboxDispatch_EndToEnd spins up real Postgres and RabbitMQ
// containers, exercises CreateOrder, confirms an outbox row is written, then
// runs one dispatch cycle and verifies the OSCreated event reaches a queue
// bound to pos.events with routing key OSCreated. Run with:
//
//	go test -tags integration ./tests/integration/...
func TestCreateOrder_OutboxDispatch_EndToEnd(t *testing.T) {
	ctx := context.Background()

	pgContainer, dsn := startPostgres(ctx, t)
	defer func() { _ = pgContainer.Terminate(ctx) }()

	amqpContainer, amqpURL := startRabbitMQ(ctx, t)
	defer func() { _ = amqpContainer.Terminate(ctx) }()

	require.NoError(t, runMigrations(dsn))

	gormDB, err := infradb.Connect(dsn)
	require.NoError(t, err)

	repo := infradb.NewOrderRepository(gormDB)
	createOrderUC := usecases.NewCreateOrderUseCase(repo)

	order, err := createOrderUC.CreateOrder(ctx, uuid.New(), uuid.New(), "integration test order", "corr-it")
	require.NoError(t, err)

	rows, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "OSCreated", rows[0].EventName)

	amqpConn, err := messaging.Dial(amqpURL)
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

func startPostgres(ctx context.Context, t *testing.T) (testcontainers.Container, string) {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "os_service",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	require.NoError(t, err)

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("host=%s user=postgres password=postgres dbname=os_service port=%s sslmode=disable", host, port.Port())
	return c, dsn
}

func startRabbitMQ(ctx context.Context, t *testing.T) (testcontainers.Container, string) {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:3-management-alpine",
		ExposedPorts: []string{"5672/tcp"},
		WaitingFor:   wait.ForListeningPort("5672/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	require.NoError(t, err)

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "5672")
	require.NoError(t, err)

	return c, fmt.Sprintf("amqp://guest:guest@%s:%s/", host, port.Port())
}

func runMigrations(dsn string) error {
	dbConn, err := infradb.Connect(dsn)
	if err != nil {
		return err
	}
	sqlDB, err := dbConn.DB()
	if err != nil {
		return err
	}
	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func mustExtractOSID(t *testing.T, ev messaging.Event) string {
	t.Helper()
	var payload struct {
		OSID string `json:"os_id"`
	}
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	return payload.OSID
}
