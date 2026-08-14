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

	_ "github.com/lib/pq"
	"github.com/podushkina/taskqueue/internal/api"
	"github.com/podushkina/taskqueue/internal/config"
	"github.com/podushkina/taskqueue/internal/metrics"
	"github.com/podushkina/taskqueue/internal/repository"
	"github.com/podushkina/taskqueue/internal/worker"
	"github.com/podushkina/taskqueue/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	logger.Info("Starting application",
		"port", cfg.ServerPort,
		"worker_count", cfg.WorkerCount,
		"redis_addr", cfg.RedisAddr,
	)

	db, err := sql.Open("postgres", cfg.DBDsn)
	if err != nil {
		logger.Error("Failed to open Postgres", "error", err)
		os.Exit(1)
	}

	if err := db.Ping(); err != nil {
		logger.Error("Failed to ping Postgres", "error", err)
		_ = db.Close()
		os.Exit(1)
	}
	logger.Info("Connected to PostgreSQL successfully")

	if err := migrations.Run(db); err != nil {
		logger.Error("Failed to run migrations", "error", err)
		_ = db.Close()
		os.Exit(1)
	}
	logger.Info("Migrations applied successfully")

	postgresRepo := repository.NewPostgresRepository(db)

	redisQueue, err := repository.NewRedisQueue(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
	if err != nil {
		logger.Error("Failed to connect to Redis", "error", err)
		_ = db.Close()
		os.Exit(1)
	}
	logger.Info("Connected to Redis successfully")

	m := metrics.NewMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redisQueue.StartQueueDepthCollector(ctx, m, 2*time.Second)

	pool := worker.NewPool(redisQueue, postgresRepo, m, cfg.WorkerCount)

	pool.Register("echo", worker.Echo)
	pool.Register("reverse", worker.Reverse)
	pool.Register("sum", worker.Sum)
	pool.Register("slow", worker.Slow)
	pool.Register("flaky", worker.Flaky)

	pool.Start(ctx)

	handler := api.NewHandler(redisQueue)
	router := api.NewRouter(handler, m)

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Server starting", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server startup failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	logger.Info("Shutdown signal received", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	}
	shutdownCancel()
	logger.Info("HTTP server stopped")

	cancel()
	pool.Stop()

	logger.Info("Closing storage connections...")
	if err := redisQueue.Close(); err != nil {
		logger.Error("Error closing Redis cleanly", "error", err)
	} else {
		logger.Info("Redis connection closed successfully")
	}

	if err := db.Close(); err != nil {
		logger.Error("Error closing Postgres cleanly", "error", err)
	} else {
		logger.Info("PostgreSQL connection closed successfully")
	}

	logger.Info("Server stopped gracefully")
}
