package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/podushkina/taskqueue/internal/queue"
	"github.com/podushkina/taskqueue/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) (*Pool, *queue.Queue, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	q, err := queue.New(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	pool := NewPool(q, 1)
	return pool, q, mr
}

func TestPool_ProcessSuccess(t *testing.T) {
	pool, q, mr := setupTest(t)
	defer mr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Register("success_task", func(ctx context.Context, t *task.Task) (string, error) {
		return "ok result", nil
	})

	tsk, err := q.Push(ctx, "success_task", "payload")
	require.NoError(t, err)

	pool.Start(ctx)

	time.Sleep(200 * time.Millisecond)

	updatedTask, err := q.Get(ctx, tsk.ID)
	require.NoError(t, err)

	assert.Equal(t, task.StatusCompleted, updatedTask.Status)
	assert.Equal(t, "ok result", updatedTask.Result)
}

func TestPool_RetryLogic(t *testing.T) {
	pool, q, mr := setupTest(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())

	pool.Register("fail_task", func(ctx context.Context, t *task.Task) (string, error) {
		return "", errors.New("something went wrong")
	})

	tsk, err := q.Push(ctx, "fail_task", "payload")
	require.NoError(t, err)

	pool.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	cancel()
	pool.Stop()

	time.Sleep(1100 * time.Millisecond)

	updatedTask, err := q.Get(context.Background(), tsk.ID)
	require.NoError(t, err)

	assert.Equal(t, task.StatusPending, updatedTask.Status)
	assert.Equal(t, 1, updatedTask.Retries)
}
