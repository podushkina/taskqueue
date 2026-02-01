package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/podushkina/taskqueue/internal/queue"
	"github.com/podushkina/taskqueue/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestQueue(t *testing.T) (*queue.Queue, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	q, err := queue.New(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}
	return q, mr
}

func TestCreateTask(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()

	h := NewHandler(q)
	router := NewRouter(h)

	payload := map[string]string{
		"type":    "echo",
		"payload": "hello api",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/tasks", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var response task.Task
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.NotEmpty(t, response.ID)
	assert.Equal(t, "echo", response.Type)
	assert.Equal(t, "hello api", response.Payload)
}

func TestGetTask_NotFound(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()

	h := NewHandler(q)
	router := NewRouter(h)

	req, _ := http.NewRequest("GET", "/tasks/non-existent-id", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetTask_Success(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	tsk, _ := q.Push(ctx, "echo", "test")

	h := NewHandler(q)
	router := NewRouter(h)

	req, _ := http.NewRequest("GET", "/tasks/"+tsk.ID, nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response task.Task
	json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Equal(t, tsk.ID, response.ID)
}

func TestListTasks(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	q.Push(ctx, "t1", "p1")
	q.Push(ctx, "t2", "p2")

	h := NewHandler(q)
	router := NewRouter(h)

	req, _ := http.NewRequest("GET", "/tasks", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var tasks []task.Task
	err := json.Unmarshal(rr.Body.Bytes(), &tasks)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestDeleteTask(t *testing.T) {
	q, mr := setupTestQueue(t)
	defer mr.Close()
	ctx := context.Background()

	tsk, _ := q.Push(ctx, "echo", "del me")

	h := NewHandler(q)
	router := NewRouter(h)

	req, _ := http.NewRequest("DELETE", "/tasks/"+tsk.ID, nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)

	found, _ := q.Get(ctx, tsk.ID)
	assert.Nil(t, found)
}
