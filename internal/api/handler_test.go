package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/podushkina/taskqueue/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFullEnqueuer struct {
	tasks      map[string]*model.Task
	errToThrow error
}

func (m *mockFullEnqueuer) Push(ctx context.Context, taskType, payload string) (*model.Task, error) {
	if m.errToThrow != nil {
		return nil, m.errToThrow
	}
	t := &model.Task{ID: "generated-id", Type: taskType, Payload: payload, Status: model.StatusPending}
	m.tasks["generated-id"] = t
	return t, nil
}

func (m *mockFullEnqueuer) Get(ctx context.Context, id string) (*model.Task, error) {
	if m.errToThrow != nil {
		return nil, m.errToThrow
	}
	return m.tasks[id], nil
}

func (m *mockFullEnqueuer) List(ctx context.Context) ([]*model.Task, error) {
	if m.errToThrow != nil {
		return nil, m.errToThrow
	}
	var list []*model.Task
	for _, t := range m.tasks {
		list = append(list, t)
	}
	return list, nil
}

func (m *mockFullEnqueuer) Delete(ctx context.Context, id string) error {
	if m.errToThrow != nil {
		return m.errToThrow
	}
	delete(m.tasks, id)
	return nil
}

func TestCreateTask_Success(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task)}
	h := NewHandler(me)

	body, _ := json.Marshal(map[string]string{"type": "echo", "payload": "test"})
	req, _ := http.NewRequest("POST", "/tasks", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	h.CreateTask(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var res model.Task
	err := json.Unmarshal(rr.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.NotEmpty(t, res.ID)
	assert.Equal(t, "echo", res.Type)
}

func TestCreateTask_MissingType(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task)}
	h := NewHandler(me)

	body, _ := json.Marshal(map[string]string{"payload": "test"})
	req, _ := http.NewRequest("POST", "/tasks", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	h.CreateTask(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCreateTask_InvalidJSON(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task)}
	h := NewHandler(me)

	req, _ := http.NewRequest("POST", "/tasks", bytes.NewBuffer([]byte(`{"type":`)))
	rr := httptest.NewRecorder()

	h.CreateTask(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCreateTask_QueueError(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task), errToThrow: errors.New("redis err")}
	h := NewHandler(me)

	body, _ := json.Marshal(map[string]string{"type": "echo", "payload": "test"})
	req, _ := http.NewRequest("POST", "/tasks", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	h.CreateTask(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetTask_Success(t *testing.T) {
	me := &mockFullEnqueuer{tasks: map[string]*model.Task{
		"111": {ID: "111", Type: "echo", Status: model.StatusPending},
	}}
	h := NewHandler(me)

	req, _ := http.NewRequest("GET", "/tasks/111", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "111")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rr := httptest.NewRecorder()

	h.GetTask(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var res model.Task
	json.Unmarshal(rr.Body.Bytes(), &res)
	assert.Equal(t, "111", res.ID)
}

func TestGetTask_NotFound(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task)}
	h := NewHandler(me)

	req, _ := http.NewRequest("GET", "/tasks/999", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rr := httptest.NewRecorder()

	h.GetTask(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetTask_QueueError(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task), errToThrow: errors.New("err")}
	h := NewHandler(me)

	req, _ := http.NewRequest("GET", "/tasks/111", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "111")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rr := httptest.NewRecorder()

	h.GetTask(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestListTasks_Success(t *testing.T) {
	me := &mockFullEnqueuer{tasks: map[string]*model.Task{
		"1": {ID: "1"},
		"2": {ID: "2"},
	}}
	h := NewHandler(me)

	req, _ := http.NewRequest("GET", "/tasks", nil)
	rr := httptest.NewRecorder()

	h.ListTasks(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var res []model.Task
	json.Unmarshal(rr.Body.Bytes(), &res)
	assert.Len(t, res, 2)
}

func TestDeleteTask_Success(t *testing.T) {
	me := &mockFullEnqueuer{tasks: map[string]*model.Task{
		"del": {ID: "del"},
	}}
	h := NewHandler(me)

	req, _ := http.NewRequest("DELETE", "/tasks/del", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "del")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rr := httptest.NewRecorder()

	h.DeleteTask(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Nil(t, me.tasks["del"])
}

func TestDeleteTask_NotFound(t *testing.T) {
	me := &mockFullEnqueuer{tasks: make(map[string]*model.Task)}
	h := NewHandler(me)

	req, _ := http.NewRequest("DELETE", "/tasks/999", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rr := httptest.NewRecorder()

	h.DeleteTask(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHealthCheck(t *testing.T) {
	h := NewHandler(&mockFullEnqueuer{})
	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	h.HealthCheck(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
}
