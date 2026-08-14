package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/podushkina/taskqueue/internal/model"
)

type TaskEnqueuer interface {
	Push(ctx context.Context, taskType, payload string) (*model.Task, error)
	Get(ctx context.Context, id string) (*model.Task, error)
	List(ctx context.Context) ([]*model.Task, error)
	Delete(ctx context.Context, id string) error
}

type AnalyticsProvider interface {
	GetAnalytics(ctx context.Context, from, to time.Time) (*model.AnalyticsSummary, error)
}

type Handler struct {
	queue     TaskEnqueuer
	analytics AnalyticsProvider
}

func NewHandler(q TaskEnqueuer, a AnalyticsProvider) *Handler {
	return &Handler{
		queue:     q,
		analytics: a,
	}
}

type CreateTaskRequest struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Type == "" {
		respondError(w, http.StatusBadRequest, "type is required")
		return
	}

	task, err := h.queue.Push(r.Context(), req.Type, req.Payload)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, task)
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	task, err := h.queue.Get(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if task == nil {
		respondError(w, http.StatusNotFound, "task not found")
		return
	}

	respondJSON(w, http.StatusOK, task)
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.queue.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, tasks)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	task, err := h.queue.Get(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if task == nil {
		respondError(w, http.StatusNotFound, "task not found")
		return
	}

	if err := h.queue.Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	if h.analytics == nil {
		respondError(w, http.StatusNotImplemented, "analytics provider is not configured")
		return
	}

	to := time.Now()
	from := to.Add(-24 * time.Hour)

	if fromParam := r.URL.Query().Get("from"); fromParam != "" {
		if t, err := time.Parse(time.RFC3339, fromParam); err == nil {
			from = t
		}
	}
	if toParam := r.URL.Query().Get("to"); toParam != "" {
		if t, err := time.Parse(time.RFC3339, toParam); err == nil {
			to = t
		}
	}

	summary, err := h.analytics.GetAnalytics(r.Context(), from, to)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, summary)
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, ErrorResponse{Error: message})
}
