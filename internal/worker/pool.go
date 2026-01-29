package worker

import (
	"context"
	"fmt"
	"log"
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
}

func NewPool(q *queue.Queue, count int) *Pool {
	return &Pool{
		queue:    q,
		handlers: make(map[string]Handler),
		count:    count,
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
	log.Printf("Started %d workers", p.count)
}

func (p *Pool) Stop() {
	p.wg.Wait()
	log.Println("All workers stopped")
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	log.Printf("Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down", id)
			return
		default:
			t, err := p.queue.Pop(ctx, 2*time.Second)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: pop error: %v", id, err)
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
	log.Printf("Worker %d processing task %s (type: %s)", workerID, t.ID, t.Type)

	t.Status = task.StatusProcessing
	if err := p.queue.Update(ctx, t); err != nil {
		log.Printf("Worker %d: update status error: %v", workerID, err)
	}

	p.mu.RLock()
	handler, ok := p.handlers[t.Type]
	p.mu.RUnlock()

	if !ok {
		t.Status = task.StatusFailed
		t.Error = fmt.Sprintf("unknown task type: %s", t.Type)
		p.queue.Update(ctx, t)
		return
	}

	result, err := handler(ctx, t)
	if err != nil {
		t.Status = task.StatusFailed
		t.Error = err.Error()
		log.Printf("Worker %d: task %s failed: %v", workerID, t.ID, err)
	} else {
		t.Status = task.StatusCompleted
		t.Result = result
		log.Printf("Worker %d: task %s completed", workerID, t.ID)
	}

	if err := p.queue.Update(ctx, t); err != nil {
		log.Printf("Worker %d: update result error: %v", workerID, err)
	}
}
