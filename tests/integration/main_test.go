//go:build integration

package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// testDSN and testAMQPURL point at the Postgres and RabbitMQ containers
// started once in TestMain and shared across every test in this package.
var (
	testDSN     string
	testAMQPURL string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, dsn, err := startPostgres(ctx)
	if err != nil {
		log.Fatalf("start postgres: %v", err)
	}
	testDSN = dsn

	amqpContainer, amqpURL, err := startRabbitMQ(ctx)
	if err != nil {
		log.Fatalf("start rabbitmq: %v", err)
	}
	testAMQPURL = amqpURL

	if err := runMigrations(testDSN); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	code := m.Run()

	_ = pgContainer.Terminate(ctx)
	_ = amqpContainer.Terminate(ctx)
	os.Exit(code)
}

func startPostgres(ctx context.Context) (testcontainers.Container, string, error) {
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
	if err != nil {
		return nil, "", err
	}

	host, err := c.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		return nil, "", err
	}

	dsn := fmt.Sprintf("host=%s user=postgres password=postgres dbname=os_service port=%s sslmode=disable", host, port.Port())
	return c, dsn, nil
}

func startRabbitMQ(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:3-management-alpine",
		ExposedPorts: []string{"5672/tcp"},
		WaitingFor:   wait.ForListeningPort("5672/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return nil, "", err
	}

	host, err := c.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	port, err := c.MappedPort(ctx, "5672")
	if err != nil {
		return nil, "", err
	}

	return c, fmt.Sprintf("amqp://guest:guest@%s:%s/", host, port.Port()), nil
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

// newTestDB opens a fresh GORM connection to the shared test Postgres container.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := infradb.Connect(testDSN)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	return db
}

// truncateAll clears every table so each test starts from a clean slate
// despite sharing one Postgres container across the whole package.
func truncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, tbl := range []string{"processed_events", "idempotency_keys", "outbox", "order_status_history", "orders"} {
		if err := db.Exec("TRUNCATE TABLE " + tbl + " CASCADE").Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}
