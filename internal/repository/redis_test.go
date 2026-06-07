package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/podushkina/taskqueue/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestQueue(t *testing.T) (*RedisQueue, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	q, err := NewRedisQueue(mr.Addr(), "", 0)
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
	assert.Equal(t, model.StatusPending, createdTask.Status)

	poppedTask, err := q.Pop(ctx, 1*time.Second)
	require.NoError(t, err)
	require.NotNil(t, poppedTask)

	assert.Equal(t, createdTask.ID, poppedTask.ID)
	assert.Equal(t, "hello payload", poppedTask.Payload)
}

func TestQueue_Update(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	tsk, _ := q.Push(ctx, "echo", "data")
	tsk.Status = model.StatusCompleted
	tsk.Result = "done"

	err := q.Update(ctx, tsk)
	assert.NoError(t, err)

	updated, err := q.Get(ctx, tsk.ID)
	require.NoError(t, err)
	assert.Equal(t, model.StatusCompleted, updated.Status)
	assert.Equal(t, "done", updated.Result)
}

func TestQueue_Retry(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	tsk, err := q.Push(ctx, "fail_task", "data")
	require.NoError(t, err)

	tsk.Status = model.StatusProcessing
	tsk.Retries = 0

	err = q.Retry(ctx, tsk)
	assert.NoError(t, err)

	updatedTask, err := q.Get(ctx, tsk.ID)
	require.NoError(t, err)
	assert.Equal(t, model.StatusPending, updatedTask.Status)
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

func TestQueue_ListAndDelete(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	t1, _ := q.Push(ctx, "echo", "1")
	t2, _ := q.Push(ctx, "echo", "2")

	list, err := q.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	err = q.Delete(ctx, t1.ID)
	assert.NoError(t, err)

	deletedTask, err := q.Get(ctx, t1.ID)
	assert.NoError(t, err)
	assert.Nil(t, deletedTask)

	remainingTask, err := q.Get(ctx, t2.ID)
	assert.NoError(t, err)
	assert.NotNil(t, remainingTask)
	assert.Equal(t, t2.ID, remainingTask.ID)
}
