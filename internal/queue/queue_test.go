package queue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/podushkina/taskqueue/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestQueue(t *testing.T) (*Queue, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	q, err := New(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	return q, mr
}

func TestQueue_PushAndPop(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	createdTask, err := q.Push(ctx, "echo", "hello payload")
	require.NoError(t, err)
	assert.NotEmpty(t, createdTask.ID)
	assert.Equal(t, task.StatusPending, createdTask.Status)

	poppedTask, err := q.Pop(ctx, 1*time.Second)
	require.NoError(t, err)
	require.NotNil(t, poppedTask)

	assert.Equal(t, createdTask.ID, poppedTask.ID)
	assert.Equal(t, "hello payload", poppedTask.Payload)
}

func TestQueue_Retry(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	tsk, err := q.Push(ctx, "fail_task", "data")
	require.NoError(t, err)

	tsk.Status = task.StatusProcessing
	tsk.Retries = 0

	err = q.Retry(ctx, tsk)
	assert.NoError(t, err)

	updatedTask, err := q.Get(ctx, tsk.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedTask)

	assert.Equal(t, task.StatusPending, updatedTask.Status)
	assert.Equal(t, 1, updatedTask.Retries)
}

func TestQueue_PopEmpty(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	tsk, err := q.Pop(ctx, 100*time.Millisecond)

	assert.NoError(t, err)
	assert.Nil(t, tsk)
}
