package repository

import (
	"context"
	"database/sql"

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
