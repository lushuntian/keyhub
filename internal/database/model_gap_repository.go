package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrModelGapNotFound = errors.New("model gap not found")

type ModelGapRecord struct {
	ID           int64     `json:"id"`
	ProviderType string    `json:"providerType"`
	ModelName    string    `json:"modelName"`
	RPMGap       int64     `json:"rpmGap"`
	TPMGap       int64     `json:"tpmGap"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type ModelGapInput struct {
	ProviderType string
	ModelName    string
	RPMGap       int64
	TPMGap       int64
}

func ListModelGaps(ctx context.Context, db *sql.DB) ([]ModelGapRecord, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, provider_type, model_name, rpm_gap, tpm_gap, created_at, updated_at
FROM model_gaps
ORDER BY rpm_gap DESC, tpm_gap DESC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list model gaps: %w", err)
	}
	defer rows.Close()

	items := make([]ModelGapRecord, 0)
	for rows.Next() {
		var item ModelGapRecord
		if err := rows.Scan(
			&item.ID,
			&item.ProviderType,
			&item.ModelName,
			&item.RPMGap,
			&item.TPMGap,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan model gap: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model gaps: %w", err)
	}
	return items, nil
}

func CreateModelGap(ctx context.Context, db *sql.DB, input ModelGapInput) (int64, error) {
	result, err := db.ExecContext(ctx, `
INSERT INTO model_gaps(provider_type, model_name, rpm_gap, tpm_gap)
VALUES (?, ?, ?, ?)`, input.ProviderType, input.ModelName, input.RPMGap, input.TPMGap)
	if err != nil {
		return 0, fmt.Errorf("create model gap: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("load created model gap id: %w", err)
	}
	return id, nil
}

func UpdateModelGap(ctx context.Context, db *sql.DB, id int64, input ModelGapInput) error {
	result, err := db.ExecContext(ctx, `
UPDATE model_gaps
SET provider_type = ?, model_name = ?, rpm_gap = ?, tpm_gap = ?
WHERE id = ?`, input.ProviderType, input.ModelName, input.RPMGap, input.TPMGap, id)
	if err != nil {
		return fmt.Errorf("update model gap: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("load updated model gap count: %w", err)
	}
	if affected == 0 {
		return ErrModelGapNotFound
	}
	return nil
}
