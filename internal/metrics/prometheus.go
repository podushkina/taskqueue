package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	HTTPRequestTotal *prometheus.CounterVec
	QueueDepth       *prometheus.GaugeVec
	TaskWaitDuration *prometheus.HistogramVec
	ActiveWorkers    prometheus.Gauge
	TasksProcessed   *prometheus.CounterVec
	TaskDuration     *prometheus.HistogramVec
	TaskRetries      *prometheus.CounterVec
	DeadLetterTasks  *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests processed",
			},
			[]string{"path", "method", "status"},
		),
		QueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "taskqueue_queue_depth",
				Help: "Current depth of the task queue by priority",
			},
			[]string{"priority"},
		),
		TaskWaitDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "taskqueue_task_wait_duration_seconds",
				Help:    "Time task spent waiting in queue before execution",
				Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
			},
			[]string{"task_type"},
		),
		ActiveWorkers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "taskqueue_active_workers",
				Help: "Number of workers currently executing tasks",
			},
		),
		TasksProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "taskqueue_tasks_processed_total",
				Help: "Total number of processed tasks by type and status",
			},
			[]string{"task_type", "status"},
		),
		TaskDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "taskqueue_task_duration_seconds",
				Help:    "Task execution duration in seconds",
				Buckets: []float64{0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
			},
			[]string{"task_type"},
		),
		TaskRetries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "taskqueue_task_retries_total",
				Help: "Total number of task execution retries",
			},
			[]string{"task_type", "retry_reason"},
		),
		DeadLetterTasks: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "taskqueue_dead_letter_tasks_total",
				Help: "Total number of tasks moved to Dead Letter Queue",
			},
			[]string{"task_type"},
		),
	}
}

// nil-safe методы
func (m *Metrics) IncHTTPRequest(pattern, method string, statusCode int) {
	if m != nil {
		m.HTTPRequestTotal.WithLabelValues(pattern, method, strconv.Itoa(statusCode)).Inc()
	}
}

func (m *Metrics) IncActiveWorkers() {
	if m != nil {
		m.ActiveWorkers.Inc()
	}
}

func (m *Metrics) DecActiveWorkers() {
	if m != nil {
		m.ActiveWorkers.Dec()
	}
}

func (m *Metrics) ObserveWaitDuration(taskType string, seconds float64) {
	if m != nil {
		m.TaskWaitDuration.WithLabelValues(taskType).Observe(seconds)
	}
}

func (m *Metrics) ObserveTaskDuration(taskType string, seconds float64) {
	if m != nil {
		m.TaskDuration.WithLabelValues(taskType).Observe(seconds)
	}
}

func (m *Metrics) IncTasksProcessed(taskType, status string) {
	if m != nil {
		m.TasksProcessed.WithLabelValues(taskType, status).Inc()
	}
}

func (m *Metrics) IncTaskRetries(taskType, reason string) {
	if m != nil {
		m.TaskRetries.WithLabelValues(taskType, reason).Inc()
	}
}

func (m *Metrics) IncDeadLetter(taskType string) {
	if m != nil {
		m.DeadLetterTasks.WithLabelValues(taskType).Inc()
	}
}
