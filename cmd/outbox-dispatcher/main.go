package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/config"
	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func runMigrations(sqlDB *sql.DB) error {
	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

const maxBackoff = 60 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	gormDB, err := infradb.Connect(cfg.DBDSN)
	if err != nil {
		slog.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		slog.Error("failed to get sql.DB", "error", err)
		os.Exit(1)
	}
	if err := runMigrations(sqlDB); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	amqpConn, err := messaging.Dial(cfg.AMQPURL)
	if err != nil {
		slog.Error("failed to connect to amqp", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	repo := infradb.NewOrderRepository(gormDB)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(cfg.DispatchIntervalMS) * time.Millisecond
	backoff := time.Second

	slog.Info("outbox dispatcher started", "interval_ms", cfg.DispatchIntervalMS)

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox dispatcher stopped")
			return
		case <-time.After(interval):
		}

		published, err := dispatchBatch(ctx, repo, amqpConn)
		if err != nil {
			slog.Error("dispatch batch failed, backing off", "error", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		if published > 0 {
			backoff = time.Second
		}
	}
}

func dispatchBatch(ctx context.Context, repo *infradb.OrderRepository, conn *messaging.Conn) (int, error) {
	rows, err := repo.FetchUnpublished(ctx, 100)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	published := make([]int64, 0, len(rows))
	for _, row := range rows {
		var ev messaging.Event
		if err := json.Unmarshal(row.Payload, &ev); err != nil {
			slog.Error("skipping malformed outbox row", "id", row.ID, "error", err)
			continue
		}
		if err := conn.Publish(ctx, ev); err != nil {
			return 0, err
		}
		published = append(published, row.ID)
	}

	if err := repo.MarkPublished(ctx, published); err != nil {
		return 0, err
	}
	return len(published), nil
}
