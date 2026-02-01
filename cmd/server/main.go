package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/podushkina/taskqueue/internal/api"
	"github.com/podushkina/taskqueue/internal/config"
	"github.com/podushkina/taskqueue/internal/handlers"
	"github.com/podushkina/taskqueue/internal/queue"
	"github.com/podushkina/taskqueue/internal/worker"
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

	q, err := queue.New(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
	if err != nil {
		logger.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := q.Close(); err != nil {
			logger.Error("Error closing Redis", "error", err)
		}
	}()
	logger.Info("Connected to Redis successfully")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := worker.NewPool(q, cfg.WorkerCount)

	pool.Register("echo", handlers.Echo)
	pool.Register("reverse", handlers.Reverse)
	pool.Register("sum", handlers.Sum)
	pool.Register("slow", handlers.Slow)
	pool.Register("flaky", handlers.Flaky)

	pool.Start(ctx)

	handler := api.NewHandler(q)
	router := api.NewRouter(handler)

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

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	}

	pool.Stop()

	logger.Info("Server stopped gracefully")
}
