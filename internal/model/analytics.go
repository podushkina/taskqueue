package model

type AnalyticsSummary struct {
	TotalTasks      int64            `json:"total_tasks"`
	StatusCounts    map[string]int64 `json:"status_counts"`
	AvgDurationSecs float64          `json:"avg_duration_seconds"`
}
