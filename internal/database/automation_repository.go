package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type HealthCheckTarget struct {
	ID              int64
	CategoryCode    string
	KeyHint         string
	Status          string
	NewAPIChannelID int64
}

type HealthCheckRecord struct {
	ID           int64     `json:"id"`
	APIKeyID     int64     `json:"apiKeyId"`
	CategoryCode string    `json:"categoryCode,omitempty"`
	KeyHint      string    `json:"keyHint,omitempty"`
	Status       string    `json:"status"`
	LatencyMS    int       `json:"latencyMs"`
	ErrorCode    string    `json:"errorCode"`
	ErrorMessage string    `json:"errorMessage"`
	CheckedAt    time.Time `json:"checkedAt"`
}

func ListHealthCheckTargets(ctx context.Context, db *sql.DB, limit int) ([]HealthCheckTarget, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, category_code, key_hint, status, newapi_channel_id
FROM api_keys
WHERE status = 'active' AND newapi_channel_id IS NOT NULL
ORDER BY COALESCE(last_checked_at, '1970-01-01') ASC, id ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list health check targets: %w", err)
	}
	defer rows.Close()

	targets := make([]HealthCheckTarget, 0)
	for rows.Next() {
		var target HealthCheckTarget
		if err := rows.Scan(&target.ID, &target.CategoryCode, &target.KeyHint, &target.Status, &target.NewAPIChannelID); err != nil {
			return nil, fmt.Errorf("scan health target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate health targets: %w", err)
	}
	return targets, nil
}

func InsertHealthCheck(ctx context.Context, db *sql.DB, keyID int64, status string, latencyMS int, errorCode string, errorMessage string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin health check: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
INSERT INTO health_checks(api_key_id, status, latency_ms, error_code, error_message)
VALUES (?, ?, ?, ?, ?)`, keyID, status, latencyMS, errorCode, errorMessage); err != nil {
		return fmt.Errorf("insert health check: %w", err)
	}

	if status == "success" {
		_, err = tx.ExecContext(ctx, `
UPDATE api_keys
SET last_health_status = 'success',
    success_count = success_count + 1,
    last_error = '',
    last_checked_at = CURRENT_TIMESTAMP
WHERE id = ?`, keyID)
	} else {
		_, err = tx.ExecContext(ctx, `
UPDATE api_keys
SET last_health_status = 'failed',
    error_count = error_count + 1,
    last_error = ?,
    last_checked_at = CURRENT_TIMESTAMP
WHERE id = ?`, errorMessage, keyID)
	}
	if err != nil {
		return fmt.Errorf("update key health status: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit health check: %w", err)
	}
	return nil
}

func ListHealthChecks(ctx context.Context, db *sql.DB, limit int, owner string) ([]HealthCheckRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	owner = strings.TrimSpace(owner)
	where := ""
	args := []any{}
	if owner != "" {
		where = "WHERE k.created_by = ?"
		args = append(args, owner)
	}
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, `
SELECT h.id, h.api_key_id, k.category_code, k.key_hint, h.status, h.latency_ms, h.error_code, h.error_message, h.checked_at
FROM health_checks h
JOIN api_keys k ON k.id = h.api_key_id
`+where+`
ORDER BY h.id DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list health checks: %w", err)
	}
	defer rows.Close()

	records := make([]HealthCheckRecord, 0)
	for rows.Next() {
		var record HealthCheckRecord
		if err := rows.Scan(&record.ID, &record.APIKeyID, &record.CategoryCode, &record.KeyHint, &record.Status, &record.LatencyMS, &record.ErrorCode, &record.ErrorMessage, &record.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan health check: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate health checks: %w", err)
	}
	return records, nil
}

func InsertAuditLog(ctx context.Context, db *sql.DB, actor string, action string, targetType string, targetID *int64, detail any) error {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO audit_logs(actor, action, target_type, target_id, detail_json)
VALUES (?, ?, ?, ?, ?)`, actor, action, targetType, targetID, string(detailJSON))
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}
