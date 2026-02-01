package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/podushkina/taskqueue/internal/task"
	"github.com/redis/go-redis/v9"
)

const (
	queueKey   = "taskqueue:pending"
	taskPrefix = "taskqueue:task:"
)

type Queue struct {
	client *redis.Client
}

func New(addr, password string, db int) (*Queue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &Queue{client: client}, nil
}

func (q *Queue) Close() error {
	return q.client.Close()
}

func (q *Queue) Push(ctx context.Context, taskType, payload string) (*task.Task, error) {
	t := &task.Task{
		ID:        uuid.New().String(),
		Type:      taskType,
		Payload:   payload,
		Status:    task.StatusPending,
		MaxRetry:  task.DefaultMaxRetry, // ← НОВОЕ: максимум 3 попытки
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshal task: %w", err)
	}

	pipe := q.client.Pipeline()
	pipe.Set(ctx, taskPrefix+t.ID, data, 24*time.Hour)
	pipe.RPush(ctx, queueKey, t.ID)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("push task: %w", err)
	}

	return t, nil
}

// Retry кладёт задачу обратно в очередь для повторной обработки.
func (q *Queue) Retry(ctx context.Context, t *task.Task) error {
	t.Retries++
	t.Status = task.StatusPending
	t.UpdatedAt = time.Now()

	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	pipe := q.client.Pipeline()
	pipe.Set(ctx, taskPrefix+t.ID, data, 24*time.Hour)
	pipe.RPush(ctx, queueKey, t.ID)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("retry task: %w", err)
	}

	return nil
}

func (q *Queue) Pop(ctx context.Context, timeout time.Duration) (*task.Task, error) {
	result, err := q.client.BLPop(ctx, timeout, queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("pop task: %w", err)
	}

	taskID := result[1]
	return q.Get(ctx, taskID)
}

func (q *Queue) Get(ctx context.Context, id string) (*task.Task, error) {
	data, err := q.client.Get(ctx, taskPrefix+id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get task: %w", err)
	}

	var t task.Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}

	return &t, nil
}

func (q *Queue) Update(ctx context.Context, t *task.Task) error {
	t.UpdatedAt = time.Now()

	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	if err := q.client.Set(ctx, taskPrefix+t.ID, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	return nil
}

func (q *Queue) List(ctx context.Context) ([]*task.Task, error) {
	keys, err := q.client.Keys(ctx, taskPrefix+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	if len(keys) == 0 {
		return []*task.Task{}, nil
	}

	pipe := q.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("fetch tasks: %w", err)
	}

	tasks := make([]*task.Task, 0, len(keys))
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue
		}

		var t task.Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		tasks = append(tasks, &t)
	}

	return tasks, nil
}

func (q *Queue) Delete(ctx context.Context, id string) error {
	if err := q.client.Del(ctx, taskPrefix+id).Err(); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}
