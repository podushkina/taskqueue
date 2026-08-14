-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_task_history_status_created_at 
ON task_history (status, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_task_history_status_created_at;
-- +goose StatementEnd