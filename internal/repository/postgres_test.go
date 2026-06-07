package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/podushkina/taskqueue/internal/model"
	"github.com/podushkina/taskqueue/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_PostgresRepository_SaveHistory(t *testing.T) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN env var is not set, skipping integration test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed connection: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("postgres is configured but not responding: %v", err)
	}

	ctx := context.Background()

	err = migrations.Run(db)
	require.NoError(t, err)

	repo := NewPostgresRepository(db)

	task := &model.Task{
		ID:        "test-uuid-99999",
		Type:      "sum",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now().Truncate(time.Millisecond),
		UpdatedAt: time.Now().Truncate(time.Millisecond),
	}

	_, _ = db.ExecContext(ctx, "DELETE FROM task_history WHERE id = $1", task.ID)

	err = repo.SaveHistory(ctx, task)
	require.NoError(t, err)

	var dbStatus, dbType string
	err = db.QueryRowContext(ctx, "SELECT status, type FROM task_history WHERE id = $1", task.ID).Scan(&dbStatus, &dbType)
	require.NoError(t, err)
	assert.Equal(t, string(model.StatusProcessing), dbStatus)
	assert.Equal(t, task.Type, dbType)

	task.Status = model.StatusCompleted
	task.Result = "100.0"
	task.UpdatedAt = time.Now().Truncate(time.Millisecond)

	err = repo.SaveHistory(ctx, task)
	require.NoError(t, err)

	var dbNewStatus, dbResult string
	err = db.QueryRowContext(ctx, "SELECT status, result FROM task_history WHERE id = $1", task.ID).Scan(&dbNewStatus, &dbResult)
	require.NoError(t, err)
	assert.Equal(t, string(model.StatusCompleted), dbNewStatus)
	assert.Equal(t, "100.0", dbResult)

	_, _ = db.ExecContext(ctx, "DELETE FROM task_history WHERE id = $1", task.ID)
}
