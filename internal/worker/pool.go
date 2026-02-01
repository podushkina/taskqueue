package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/podushkina/taskqueue/internal/queue"
	"github.com/podushkina/taskqueue/internal/task"
)

type Handler func(ctx context.Context, t *task.Task) (string, error)

type Pool struct {
	queue    *queue.Queue
	handlers map[string]Handler
	count    int
	wg       sync.WaitGroup
	mu       sync.RWMutex
	logger   *slog.Logger
}

func NewPool(q *queue.Queue, count int) *Pool {
	// Настраиваем логгер: пишет JSON в консоль
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	return &Pool{
		queue:    q,
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
			// Ждем задачу 1 секунду, если нет — пробуем снова
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

			p.process(ctx, id, t)
		}
	}
}

func (p *Pool) process(ctx context.Context, workerID int, t *task.Task) {
	// Создаем логгер с контекстом задачи (чтобы видеть ID задачи в логах)
	log := p.logger.With("worker_id", workerID, "task_id", t.ID, "type", t.Type)
	log.Info("Processing task")

	t.Status = task.StatusProcessing
	if err := p.queue.Update(ctx, t); err != nil {
		log.Error("Failed to update status", "error", err)
	}

	p.mu.RLock()
	handler, ok := p.handlers[t.Type]
	p.mu.RUnlock()

	// Если не нашли обработчик для такого типа задач
	if !ok {
		t.Status = task.StatusFailed
		t.Error = fmt.Sprintf("unknown task type: %s", t.Type)
		p.queue.Update(ctx, t)
		log.Error("Unknown task type")
		return
	}

	// Выполняем задачу
	result, err := handler(ctx, t)

	if err != nil {
		// RETRY
		if t.Retries < t.MaxRetry {
			// Считаем время ожидания: 2 в степени попыток (1с, 2с, 4с...)
			backoff := time.Duration(math.Pow(2, float64(t.Retries))) * time.Second

			log.Warn("Task failed, retrying", "attempt", t.Retries+1, "backoff", backoff, "error", err)

			// Запускаем таймер в фоне, чтобы не блокировать воркера
			go func() {
				time.Sleep(backoff)
				// Возвращаем в очередь
				if err := p.queue.Retry(context.Background(), t); err != nil {
					p.logger.Error("Failed to retry task", "task_id", t.ID, "error", err)
				}
			}()
		} else {
			// Попытки кончились — фейлим окончательно
			t.Status = task.StatusFailed
			t.Error = err.Error()
			p.queue.Update(ctx, t)
			log.Error("Task failed permanently", "error", err)
		}
	} else {
		// Успех
		t.Status = task.StatusCompleted
		t.Result = result
		p.queue.Update(ctx, t)
		log.Info("Task completed")
	}
}
