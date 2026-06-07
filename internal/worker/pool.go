package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/podushkina/taskqueue/internal/model"
)

type TaskConsumer interface {
	Pop(ctx context.Context, timeout time.Duration) (*model.Task, error)
	Update(ctx context.Context, t *model.Task) error
	Retry(ctx context.Context, t *model.Task) error
}

type HistoryRepository interface {
	SaveHistory(ctx context.Context, t *model.Task) error
}

type Handler func(ctx context.Context, t *model.Task) (string, error)

type Pool struct {
	queue    TaskConsumer
	repo     HistoryRepository
	handlers map[string]Handler
	count    int
	wg       sync.WaitGroup
	mu       sync.RWMutex
	logger   *slog.Logger
}

func NewPool(q TaskConsumer, repo HistoryRepository, count int) *Pool {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	return &Pool{
		queue:    q,
		repo:     repo,
		handlers: make(map[string]Handler),
		count:    count,
		logger:   logger,
	}
}

func (p *Pool) Register(taskType string, handler Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[taskType] = handler
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.count; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
	p.logger.Info("Started workers", "count", p.count)
}

func (p *Pool) Stop() {
	p.wg.Wait()
	p.logger.Info("All workers stopped")
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	p.logger.Debug("Worker started", "worker_id", id)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			t, err := p.queue.Pop(ctx, 1*time.Second)
			if err != nil {
				if ctx.Err() == nil {
					p.logger.Error("Pop error", "worker_id", id, "error", err)
				}
				continue
			}

			if t == nil {
				continue
			}

			processCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			p.process(processCtx, id, t)
			cancel()
		}
	}
}

func (p *Pool) process(ctx context.Context, workerID int, t *model.Task) {
	log := p.logger.With("worker_id", workerID, "task_id", t.ID, "type", t.Type)
	log.Info("Processing task")

	t.Status = model.StatusProcessing
	if err := p.queue.Update(ctx, t); err != nil {
		log.Error("Failed to update status", "error", err)
	}

	p.mu.RLock()
	handler, ok := p.handlers[t.Type]
	p.mu.RUnlock()

	if !ok {
		t.Status = model.StatusFailed
		t.Error = fmt.Sprintf("unknown task type: %s", t.Type)
		_ = p.queue.Update(ctx, t)
		if err := p.repo.SaveHistory(ctx, t); err != nil {
			log.Error("Failed to save history", "error", err)
		}
		log.Error("Unknown task type")
		return
	}

	result, err := handler(ctx, t)

	if err != nil {
		if t.Retries < t.MaxRetry {
			backoff := time.Duration(math.Pow(2, float64(t.Retries))) * time.Second

			log.Warn("Task failed, retrying", "attempt", t.Retries+1, "backoff", backoff, "error", err)

			go func() {
				time.Sleep(backoff)
				if err := p.queue.Retry(context.Background(), t); err != nil {
					p.logger.Error("Failed to retry task", "task_id", t.ID, "error", err)
				}
			}()
		} else {
			t.Status = model.StatusFailed
			t.Error = err.Error()
			_ = p.queue.Update(ctx, t)
			if err := p.repo.SaveHistory(ctx, t); err != nil {
				log.Error("Failed to save history", "error", err)
			}
			log.Error("Task failed permanently", "error", err)
		}
	} else {
		t.Status = model.StatusCompleted
		t.Result = result
		_ = p.queue.Update(ctx, t)
		if err := p.repo.SaveHistory(ctx, t); err != nil {
			log.Error("Failed to save history", "error", err)
		}
		log.Info("Task completed")
	}
}
