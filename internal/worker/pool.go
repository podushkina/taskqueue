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

type MetricsRecorder interface {
	IncActiveWorkers()
	DecActiveWorkers()
	ObserveWaitDuration(taskType string, seconds float64)
	ObserveTaskDuration(taskType string, seconds float64)
	IncTasksProcessed(taskType, status string)
	IncTaskRetries(taskType, reason string)
	IncDeadLetter(taskType string)
}

type Handler func(ctx context.Context, t *model.Task) (string, error)

type Pool struct {
	queue    TaskConsumer
	repo     HistoryRepository
	metrics  MetricsRecorder
	handlers map[string]Handler
	count    int
	wg       sync.WaitGroup
	mu       sync.RWMutex
	logger   *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

func NewPool(q TaskConsumer, repo HistoryRepository, m MetricsRecorder, count int) *Pool {
	return &Pool{
		queue:    q,
		repo:     repo,
		metrics:  m,
		handlers: make(map[string]Handler),
		count:    count,
		logger:   slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

func (p *Pool) Register(taskType string, handler Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[taskType] = handler
}

func (p *Pool) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)

	for i := 0; i < p.count; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	p.logger.Info("Started workers", "count", p.count)
}

func (p *Pool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.logger.Info("All workers stopped")
}

func (p *Pool) worker(id int) {
	defer p.wg.Done()
	p.logger.Debug("Worker started", "worker_id", id)

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			t, err := p.queue.Pop(p.ctx, 1*time.Second)
			if err != nil {
				if p.ctx.Err() == nil {
					p.logger.Error("Pop error", "worker_id", id, "error", err)
				}
				continue
			}
			if t == nil {
				continue
			}

			procCtx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
			p.process(procCtx, id, t)
			cancel()
		}
	}
}

func (p *Pool) process(ctx context.Context, workerID int, t *model.Task) {
	log := p.logger.With("worker_id", workerID, "task_id", t.ID, "type", t.Type)
	log.Info("Processing task")

	if p.metrics != nil {
		p.metrics.IncActiveWorkers()
		defer p.metrics.DecActiveWorkers()
		p.metrics.ObserveWaitDuration(t.Type, time.Since(t.CreatedAt).Seconds())
	}

	defer func() {
		if r := recover(); r != nil {
			log.Error("Worker recovered from panic", "panic", r)
			p.fail(ctx, t, fmt.Sprintf("panic: %v", r), "panic")
		}
	}()

	t.Status = model.StatusProcessing
	if err := p.queue.Update(ctx, t); err != nil {
		log.Error("Failed to set processing status", "error", err)
	}

	p.mu.RLock()
	handler, ok := p.handlers[t.Type]
	p.mu.RUnlock()

	if !ok {
		log.Error("Unknown task type")
		p.fail(ctx, t, fmt.Sprintf("unknown task type: %s", t.Type), "failed")
		return
	}

	start := time.Now()
	result, err := handler(ctx, t)
	if p.metrics != nil {
		p.metrics.ObserveTaskDuration(t.Type, time.Since(start).Seconds())
	}

	if err != nil {
		if t.Retries < t.MaxRetry {
			p.scheduleRetry(t, err, log)
		} else {
			if p.metrics != nil {
				p.metrics.IncDeadLetter(t.Type)
			}
			p.fail(ctx, t, err.Error(), "failed")
			log.Error("Task failed permanently, moved to DLQ", "error", err)
		}
		return
	}

	p.complete(ctx, t, result, log)
}

func (p *Pool) complete(ctx context.Context, t *model.Task, result string, log *slog.Logger) {
	if p.metrics != nil {
		p.metrics.IncTasksProcessed(t.Type, "success")
	}
	t.Status = model.StatusCompleted
	t.Result = result

	_ = p.queue.Update(ctx, t)
	if err := p.repo.SaveHistory(ctx, t); err != nil {
		log.Error("Failed to save history", "error", err)
	}
	log.Info("Task completed")
}

func (p *Pool) fail(ctx context.Context, t *model.Task, reason, metricStatus string) {
	if p.metrics != nil {
		p.metrics.IncTasksProcessed(t.Type, metricStatus)
	}
	t.Status = model.StatusFailed
	t.Error = reason

	_ = p.queue.Update(ctx, t)
	if err := p.repo.SaveHistory(ctx, t); err != nil {
		p.logger.Error("Failed to save history", "task_id", t.ID, "error", err)
	}
}

func (p *Pool) scheduleRetry(t *model.Task, err error, log *slog.Logger) {
	if p.metrics != nil {
		p.metrics.IncTaskRetries(t.Type, "handler_error")
	}
	backoff := time.Duration(math.Pow(2, float64(t.Retries))) * time.Second
	log.Warn("Task failed, scheduling retry", "attempt", t.Retries+1, "backoff", backoff, "error", err)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		timer := time.NewTimer(backoff)
		defer timer.Stop()

		select {
		case <-timer.C:
			if err := p.queue.Retry(context.Background(), t); err != nil {
				p.logger.Error("Failed to retry task", "task_id", t.ID, "error", err)
			}
		case <-p.ctx.Done():
			p.logger.Warn("Retry cancelled on shutdown", "task_id", t.ID)
		}
	}()
}
