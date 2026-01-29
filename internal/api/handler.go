package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/podushkina/taskqueue/internal/queue"
)

type Handler struct {
	queue *queue.Queue
}

func NewHandler(q *queue.Queue) *Handler {
	return &Handler{queue: q}
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
