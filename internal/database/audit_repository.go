package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AuditLogRecord struct {
	ID         int64     `json:"id"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   *int64    `json:"targetId,omitempty"`
	DetailJSON string    `json:"detailJson"`
	CreatedAt  time.Time `json:"createdAt"`
}

func ListAuditLogs(ctx context.Context, db *sql.DB, limit int) ([]AuditLogRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, actor, action, target_type, target_id, COALESCE(CAST(detail_json AS CHAR), ''), created_at
FROM audit_logs
ORDER BY id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	items := make([]AuditLogRecord, 0)
	for rows.Next() {
		var item AuditLogRecord
		var targetID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Actor, &item.Action, &item.TargetType, &targetID, &item.DetailJSON, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		if targetID.Valid {
			item.TargetID = &targetID.Int64
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit logs: %w", err)
	}
	return items, nil
}
