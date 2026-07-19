package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielAnFran/pos-os-service/internal/application/usecases"
	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/config"
	infradb "github.com/gabrielAnFran/pos-os-service/internal/infrastructure/db"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/handlers"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/middleware"
	"github.com/gin-gonic/gin"
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

	orderRepo := infradb.NewOrderRepository(gormDB)
	idempotencyRepo := infradb.NewIdempotencyRepository(gormDB)
	createOrderUC := usecases.NewCreateOrderUseCase(orderRepo)
	handler := handlers.NewOrderHandler(createOrderUC, orderRepo, gormDB)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Recovery(), middleware.Correlation(), middleware.Logging())

	v1 := r.Group("/api/v1")
	{
		v1.POST("/orders", middleware.Idempotency(idempotencyRepo), handler.CreateOrder)
		v1.GET("/orders", handler.ListOrders)
		v1.GET("/orders/:id", handler.GetOrder)
		v1.PATCH("/orders/:id/status", handler.UpdateStatus)
	}
	r.GET("/healthz", handler.Healthz)
	r.GET("/readyz", handler.Readyz)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()
	slog.Info("server started", "port", cfg.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	slog.Info("server stopped")
}
