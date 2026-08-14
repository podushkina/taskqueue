package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/podushkina/taskqueue/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConsumer struct {
	updatedTask *model.Task
	retryCalled bool
	errOnUpdate error
}

func (m *mockConsumer) Pop(ctx context.Context, timeout time.Duration) (*model.Task, error) {
	return nil, nil
}

func (m *mockConsumer) Update(ctx context.Context, t *model.Task) error {
	if m.errOnUpdate != nil {
		return m.errOnUpdate
	}
	m.updatedTask = t
	return nil
}

func (m *mockConsumer) Retry(ctx context.Context, t *model.Task) error {
	m.retryCalled = true
	return nil
}

type mockHistory struct {
	saved         bool
	errOnSave     error
	savedWithTask *model.Task
}

func (m *mockHistory) SaveHistory(ctx context.Context, t *model.Task) error {
	if m.errOnSave != nil {
		return m.errOnSave
	}
	m.saved = true
	m.savedWithTask = t
	return nil
}

type mockMetrics struct{}

func (m *mockMetrics) IncActiveWorkers()                                    {}
func (m *mockMetrics) DecActiveWorkers()                                    {}
func (m *mockMetrics) ObserveWaitDuration(taskType string, seconds float64) {}
func (m *mockMetrics) ObserveTaskDuration(taskType string, seconds float64) {}
func (m *mockMetrics) IncTasksProcessed(taskType, status string)            {}
func (m *mockMetrics) IncTaskRetries(taskType, reason string)               {}
func (m *mockMetrics) IncDeadLetter(taskType string)                        {}

func TestPool_Process_Success(t *testing.T) {
	mc := &mockConsumer{}
	mh := &mockHistory{}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 1)

	pool.Register("success_task", func(ctx context.Context, t *model.Task) (string, error) {
		return "computed data", nil
	})

	tsk := &model.Task{ID: "1", Type: "success_task", Status: model.StatusPending, MaxRetry: 3, CreatedAt: time.Now()}
	pool.process(context.Background(), 1, tsk)

	require.NotNil(t, mc.updatedTask)
	assert.Equal(t, model.StatusCompleted, mc.updatedTask.Status)
	assert.Equal(t, "computed data", mc.updatedTask.Result)
	assert.True(t, mh.saved)
}

func TestPool_Process_UnknownTaskType(t *testing.T) {
	mc := &mockConsumer{}
	mh := &mockHistory{}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 1)

	tsk := &model.Task{ID: "2", Type: "ghost_task", Status: model.StatusPending, CreatedAt: time.Now()}
	pool.process(context.Background(), 1, tsk)

	assert.Equal(t, model.StatusFailed, mc.updatedTask.Status)
	assert.Contains(t, mc.updatedTask.Error, "unknown task type")
	assert.True(t, mh.saved)
}

func TestPool_Process_HandlerError_WithRetry(t *testing.T) {
	mc := &mockConsumer{}
	mh := &mockHistory{}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	pool.Register("retry_task", func(ctx context.Context, t *model.Task) (string, error) {
		return "", errors.New("temporary error")
	})

	tsk := &model.Task{ID: "3", Type: "retry_task", Status: model.StatusPending, Retries: 0, MaxRetry: 3, CreatedAt: time.Now()}
	pool.process(context.Background(), 1, tsk)

	time.Sleep(1050 * time.Millisecond)

	assert.True(t, mc.retryCalled)
}

func TestPool_Process_HandlerError_MaxRetryReached(t *testing.T) {
	mc := &mockConsumer{}
	mh := &mockHistory{}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 1)

	pool.Register("dead_task", func(ctx context.Context, t *model.Task) (string, error) {
		return "", errors.New("fatal error")
	})

	tsk := &model.Task{ID: "4", Type: "dead_task", Status: model.StatusPending, Retries: 3, MaxRetry: 3, CreatedAt: time.Now()}
	pool.process(context.Background(), 1, tsk)

	assert.False(t, mc.retryCalled)
	assert.Equal(t, model.StatusFailed, mc.updatedTask.Status)
	assert.Equal(t, "fatal error", mc.updatedTask.Error)
	assert.True(t, mh.saved)
}

func TestPool_Process_QueueUpdateError(t *testing.T) {
	mc := &mockConsumer{errOnUpdate: errors.New("redis down")}
	mh := &mockHistory{}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 1)

	pool.Register("task", func(ctx context.Context, t *model.Task) (string, error) {
		return "ok", nil
	})

	tsk := &model.Task{ID: "5", Type: "task", Status: model.StatusPending, CreatedAt: time.Now()}
	assert.NotPanics(t, func() {
		pool.process(context.Background(), 1, tsk)
	})
}

func TestPool_Process_HistorySaveError(t *testing.T) {
	mc := &mockConsumer{}
	mh := &mockHistory{errOnSave: errors.New("postgres down")}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 1)

	pool.Register("task", func(ctx context.Context, t *model.Task) (string, error) {
		return "ok", nil
	})

	tsk := &model.Task{ID: "6", Type: "task", Status: model.StatusPending, CreatedAt: time.Now()}
	assert.NotPanics(t, func() {
		pool.process(context.Background(), 1, tsk)
	})
}

func TestPool_Register_ThreadSafety(t *testing.T) {
	mm := &mockMetrics{}
	pool := NewPool(&mockConsumer{}, &mockHistory{}, mm, 1)
	h := func(ctx context.Context, t *model.Task) (string, error) { return "", nil }

	go pool.Register("t1", h)
	go pool.Register("t2", h)

	time.Sleep(50 * time.Millisecond)
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	assert.NotNil(t, pool.handlers["t1"])
	assert.NotNil(t, pool.handlers["t2"])
}

func TestPool_GracefulShutdown(t *testing.T) {
	mc := &mockConsumer{}
	mh := &mockHistory{}
	mm := &mockMetrics{}
	pool := NewPool(mc, mh, mm, 2)

	ctx, cancel := context.WithCancel(context.Background())

	pool.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()

	shutdownDone := make(chan struct{})
	go func() {
		pool.Stop()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Pool.Stop() hung, workers did not stop gracefully")
	}
}
