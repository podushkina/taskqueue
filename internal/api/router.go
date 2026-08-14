package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *Handler, m HTTPMetricsRecorder) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(MetricsMiddleware(m))

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.HealthCheck)
	r.Get("/analytics", h.GetAnalytics)

	r.Route("/tasks", func(r chi.Router) {
		r.Post("/", h.CreateTask)
		r.Get("/", h.ListTasks)
		r.Get("/{id}", h.GetTask)
		r.Delete("/{id}", h.DeleteTask)
	})

	return r
}
