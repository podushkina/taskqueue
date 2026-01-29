package main

import (
	"context"
	"log"
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
	cfg := config.Load()

	q, err := queue.New(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer q.Close()
	log.Println("Connected to Redis")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := worker.NewPool(q, cfg.WorkerCount)
	pool.Register("echo", handlers.Echo)
	pool.Register("reverse", handlers.Reverse)
	pool.Register("sum", handlers.Sum)
	pool.Register("slow", handlers.Slow)
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
		log.Printf("Server starting on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown signal received")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	pool.Stop()
	log.Println("Server stopped")
}
