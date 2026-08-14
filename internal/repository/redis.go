package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/podushkina/taskqueue/internal/metrics"
	"github.com/podushkina/taskqueue/internal/model"
	"github.com/redis/go-redis/v9"
)

const (
	queueKey   = "taskqueue:pending"
	taskPrefix = "taskqueue:task:"
)

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(addr, password string, db int) (*RedisQueue, error) {
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

	return &RedisQueue{client: client}, nil
}

func (q *RedisQueue) Close() error {
	return q.client.Close()
}

func (q *RedisQueue) Push(ctx context.Context, taskType, payload string) (*model.Task, error) {
	t := &model.Task{
		ID:        uuid.New().String(),
		Type:      taskType,
		Payload:   payload,
		Status:    model.StatusPending,
		MaxRetry:  model.DefaultMaxRetry,
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

func (q *RedisQueue) Retry(ctx context.Context, t *model.Task) error {
	t.Retries++
	t.Status = model.StatusPending
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

func (q *RedisQueue) Pop(ctx context.Context, timeout time.Duration) (*model.Task, error) {
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

func (q *RedisQueue) Get(ctx context.Context, id string) (*model.Task, error) {
	data, err := q.client.Get(ctx, taskPrefix+id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get task: %w", err)
	}

	var t model.Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}

	return &t, nil
}

func (q *RedisQueue) Update(ctx context.Context, t *model.Task) error {
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

func (q *RedisQueue) List(ctx context.Context) ([]*model.Task, error) {
	keys, err := q.client.Keys(ctx, taskPrefix+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	if len(keys) == 0 {
		return []*model.Task{}, nil
	}

	pipe := q.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("fetch tasks: %w", err)
	}

	tasks := make([]*model.Task, 0, len(keys))
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue
		}

		var t model.Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		tasks = append(tasks, &t)
	}

	return tasks, nil
}

func (q *RedisQueue) Delete(ctx context.Context, id string) error {
	if err := q.client.Del(ctx, taskPrefix+id).Err(); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

func (q *RedisQueue) StartQueueDepthCollector(ctx context.Context, m *metrics.Metrics, interval time.Duration) {
	if m == nil {
		return
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				length, err := q.client.LLen(ctx, queueKey).Result()
				if err == nil {
					m.QueueDepth.WithLabelValues("default").Set(float64(length))
				}
			}
		}
	}()
}
