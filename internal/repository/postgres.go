package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/podushkina/taskqueue/internal/model"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) SaveHistory(ctx context.Context, t *model.Task) error {
	query := `
		INSERT INTO task_history (id, type, status, result, error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE 
		SET status = EXCLUDED.status, 
			result = EXCLUDED.result, 
			error = EXCLUDED.error, 
			updated_at = EXCLUDED.updated_at;`

	_, err := r.db.ExecContext(ctx, query, t.ID, t.Type, t.Status, t.Result, t.Error, t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *PostgresRepository) GetAnalytics(ctx context.Context, from, to time.Time) (*model.AnalyticsSummary, error) {
	query := `
		SELECT 
			COUNT(*) AS total_tasks,
			COUNT(*) FILTER (WHERE status = 'completed') AS completed_count,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
			COUNT(*) FILTER (WHERE status = 'processing') AS processing_count,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
			COALESCE(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) FILTER (WHERE status = 'completed'), 0) AS avg_duration
		FROM task_history
		WHERE created_at >= $1 AND created_at <= $2;`

	var (
		total, completed, failed, processing, pending int64
		avgDuration                                   float64
	)

	err := r.db.QueryRowContext(ctx, query, from, to).Scan(
		&total,
		&completed,
		&failed,
		&processing,
		&pending,
		&avgDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute analytics query: %w", err)
	}

	return &model.AnalyticsSummary{
		TotalTasks: total,
		StatusCounts: map[string]int64{
			"completed":  completed,
			"failed":     failed,
			"processing": processing,
			"pending":    pending,
		},
		AvgDurationSecs: avgDuration,
	}, nil
}
