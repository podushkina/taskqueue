package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", h.HealthCheck)

	r.Route("/tasks", func(r chi.Router) {
		r.Post("/", h.CreateTask)
		r.Get("/", h.ListTasks)
		r.Get("/{id}", h.GetTask)
		r.Delete("/{id}", h.DeleteTask)
	})

	return r
}
