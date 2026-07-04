package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type TableStat struct {
	TableName string  `json:"tableName"`
	Rows      int64   `json:"rows"`
	DataMB    float64 `json:"dataMb"`
	IndexMB   float64 `json:"indexMb"`
}

type WorkerStat struct {
	WorkerName     string     `json:"workerName"`
	TotalRuns      int64      `json:"totalRuns"`
	SuccessRuns    int64      `json:"successRuns"`
	FailedRuns     int64      `json:"failedRuns"`
	SuccessRate    float64    `json:"successRate"`
	LastStatus     string     `json:"lastStatus"`
	LastFinishedAt *time.Time `json:"lastFinishedAt,omitempty"`
}

type RecentError struct {
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

type KeyExportRecord struct {
	ID               int64     `json:"id"`
	CategoryCode     string    `json:"categoryCode"`
	CategoryLabel    string    `json:"categoryLabel"`
	KeyHint          string    `json:"keyHint"`
	Region           string    `json:"region"`
	BaseURL          string    `json:"baseUrl"`
	Models           []string  `json:"models"`
	Tag              string    `json:"tag"`
	ExpectedTPM      int64     `json:"expectedTpm"`
	Status           string    `json:"status"`
	NewAPIChannelID  *int64    `json:"newApiChannelId,omitempty"`
	LastSyncStatus   string    `json:"lastSyncStatus"`
	LastHealthStatus string    `json:"lastHealthStatus"`
	SuccessCount     int64     `json:"successCount"`
	ErrorCount       int64     `json:"errorCount"`
	LastError        string    `json:"lastError"`
	CreatedAt        time.Time `json:"createdAt"`
}

func LoadTableStats(ctx context.Context, db *sql.DB) ([]TableStat, error) {
	rows, err := db.QueryContext(ctx, `
SELECT table_name,
       COALESCE(table_rows, 0),
       COALESCE(data_length, 0) / 1024 / 1024,
       COALESCE(index_length, 0) / 1024 / 1024
FROM information_schema.tables
WHERE table_schema = DATABASE()
ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("load table stats: %w", err)
	}
	defer rows.Close()

	items := make([]TableStat, 0)
	for rows.Next() {
		var item TableStat
		if err := rows.Scan(&item.TableName, &item.Rows, &item.DataMB, &item.IndexMB); err != nil {
			return nil, fmt.Errorf("scan table stat: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table stats: %w", err)
	}
	return items, nil
}

func LoadWorkerStats(ctx context.Context, db *sql.DB, days int) ([]WorkerStat, error) {
	if days <= 0 || days > 365 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	rows, err := db.QueryContext(ctx, `
SELECT
  worker_name,
  COUNT(*) AS total_runs,
  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
  COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
  MAX(finished_at) AS last_finished_at,
  SUBSTRING_INDEX(GROUP_CONCAT(status ORDER BY finished_at DESC, id DESC), ',', 1) AS last_status
FROM worker_runs
WHERE created_at >= ?
GROUP BY worker_name
ORDER BY worker_name`, since)
	if err != nil {
		return nil, fmt.Errorf("load worker stats: %w", err)
	}
	defer rows.Close()

	items := make([]WorkerStat, 0)
	for rows.Next() {
		var item WorkerStat
		var lastFinished sql.NullTime
		if err := rows.Scan(&item.WorkerName, &item.TotalRuns, &item.SuccessRuns, &item.FailedRuns, &lastFinished, &item.LastStatus); err != nil {
			return nil, fmt.Errorf("scan worker stat: %w", err)
		}
		if item.TotalRuns > 0 {
			item.SuccessRate = float64(item.SuccessRuns) / float64(item.TotalRuns)
		}
		if lastFinished.Valid {
			item.LastFinishedAt = &lastFinished.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate worker stats: %w", err)
	}
	return items, nil
}

func LoadRecentErrors(ctx context.Context, db *sql.DB, limit int) ([]RecentError, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `
SELECT source, message, created_at
FROM (
  SELECT 'sync' AS source, error_message AS message, created_at
  FROM newapi_sync_events
  WHERE status = 'failed' AND error_message <> ''
  UNION ALL
  SELECT 'health' AS source, error_message AS message, checked_at AS created_at
  FROM health_checks
  WHERE status = 'failed' AND error_message <> ''
  UNION ALL
  SELECT 'worker' AS source, error_message AS message, created_at
  FROM worker_runs
  WHERE status = 'failed' AND error_message <> ''
  UNION ALL
  SELECT 'key' AS source, last_error AS message, updated_at AS created_at
  FROM api_keys
  WHERE last_error <> ''
) recent_errors
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load recent errors: %w", err)
	}
	defer rows.Close()

	items := make([]RecentError, 0)
	for rows.Next() {
		var item RecentError
		if err := rows.Scan(&item.Source, &item.Message, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan recent error: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent errors: %w", err)
	}
	return items, nil
}

func ExportKeyInventory(ctx context.Context, db *sql.DB, limit int) ([]KeyExportRecord, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := db.QueryContext(ctx, `
SELECT
  k.id,
  k.category_code,
  r.category_label,
  k.key_hint,
  k.region,
  k.base_url,
  k.models_json,
  k.tag,
  k.expected_tpm,
  k.status,
  k.newapi_channel_id,
  k.last_sync_status,
  k.last_health_status,
  k.success_count,
  k.error_count,
  k.last_error,
  k.created_at
FROM api_keys k
JOIN category_pool_rules r ON r.category_code = k.category_code
ORDER BY k.id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("export key inventory: %w", err)
	}
	defer rows.Close()

	items := make([]KeyExportRecord, 0)
	for rows.Next() {
		var item KeyExportRecord
		var modelsJSON []byte
		var newAPIChannelID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.CategoryCode,
			&item.CategoryLabel,
			&item.KeyHint,
			&item.Region,
			&item.BaseURL,
			&modelsJSON,
			&item.Tag,
			&item.ExpectedTPM,
			&item.Status,
			&newAPIChannelID,
			&item.LastSyncStatus,
			&item.LastHealthStatus,
			&item.SuccessCount,
			&item.ErrorCount,
			&item.LastError,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan key export: %w", err)
		}
		if newAPIChannelID.Valid {
			item.NewAPIChannelID = &newAPIChannelID.Int64
		}
		if err := json.Unmarshal(modelsJSON, &item.Models); err != nil {
			return nil, fmt.Errorf("parse key export models: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate key export: %w", err)
	}
	return items, nil
}
