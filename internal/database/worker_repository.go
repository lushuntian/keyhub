package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type WorkerRunRecord struct {
	ID           int64     `json:"id"`
	WorkerName   string    `json:"workerName"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
	DurationMS   int64     `json:"durationMs"`
	ErrorMessage string    `json:"errorMessage"`
}

func InsertWorkerRun(ctx context.Context, db *sql.DB, workerName string, status string, startedAt time.Time, finishedAt time.Time, detail any, errorMessage string) error {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal worker detail: %w", err)
	}
	durationMS := finishedAt.Sub(startedAt).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO worker_runs(worker_name, status, started_at, finished_at, duration_ms, detail_json, error_message)
VALUES (?, ?, ?, ?, ?, ?, ?)`, workerName, status, startedAt, finishedAt, durationMS, string(detailJSON), errorMessage); err != nil {
		return fmt.Errorf("insert worker run: %w", err)
	}
	return nil
}

func ListWorkerRuns(ctx context.Context, db *sql.DB, limit int) ([]WorkerRunRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, worker_name, status, started_at, finished_at, duration_ms, error_message
FROM worker_runs
ORDER BY id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list worker runs: %w", err)
	}
	defer rows.Close()

	items := make([]WorkerRunRecord, 0)
	for rows.Next() {
		var item WorkerRunRecord
		if err := rows.Scan(&item.ID, &item.WorkerName, &item.Status, &item.StartedAt, &item.FinishedAt, &item.DurationMS, &item.ErrorMessage); err != nil {
			return nil, fmt.Errorf("scan worker run: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate worker runs: %w", err)
	}
	return items, nil
}
