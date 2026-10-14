package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrAggregationTargetNotFound = errors.New("aggregation target not found")

type AggregationTargetRecord struct {
	Code                      string    `json:"code"`
	Name                      string    `json:"name"`
	BaseURL                   string    `json:"baseUrl"`
	ConnectionMode            string    `json:"connectionMode"`
	ChannelType               string    `json:"channelType"`
	Models                    []string  `json:"models"`
	Enabled                   bool      `json:"enabled"`
	IsSystemDefault           bool      `json:"isSystemDefault"`
	Source                    string    `json:"source"`
	HasToken                  bool      `json:"hasToken"`
	ReverseUsername           string    `json:"reverseUsername"`
	ReverseGroupName          string    `json:"reverseGroupName"`
	HasReversePassword        bool      `json:"hasReversePassword"`
	CreatedAt                 time.Time `json:"createdAt"`
	UpdatedAt                 time.Time `json:"updatedAt"`
	TokenCiphertext           string    `json:"-"`
	ReversePasswordCiphertext string    `json:"-"`
}

type AggregationTargetUpsert struct {
	Code                      string
	Name                      string
	BaseURL                   string
	ConnectionMode            string
	ChannelType               string
	Models                    []string
	TokenCiphertext           string
	ReverseUsername           string
	ReverseGroupName          string
	ReversePasswordCiphertext string
	Enabled                   bool
	IsSystemDefault           bool
	SortOrder                 int
}

func ListAggregationTargets(ctx context.Context, db *sql.DB, enabledOnly bool) ([]AggregationTargetRecord, error) {
	query := `
SELECT
  code,
  name,
  base_url,
  COALESCE(NULLIF(connection_mode, ''), 'api'),
  COALESCE(NULLIF(channel_type, ''), 'anthropic'),
  models_json,
  enabled,
  is_system_default,
  token_ciphertext <> '',
  reverse_username,
  COALESCE(NULLIF(reverse_group_name, ''), 'default'),
  COALESCE(reverse_password_ciphertext, '') <> '',
  created_at,
  updated_at
FROM aggregation_targets`
	if enabledOnly {
		query += `
WHERE enabled = TRUE`
	}
	query += `
ORDER BY sort_order ASC, name ASC, code ASC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list aggregation targets: %w", err)
	}
	defer rows.Close()

	items := make([]AggregationTargetRecord, 0)
	for rows.Next() {
		var item AggregationTargetRecord
		var modelsJSON []byte
		var hasToken int
		var hasReversePassword int
		if err := rows.Scan(
			&item.Code,
			&item.Name,
			&item.BaseURL,
			&item.ConnectionMode,
			&item.ChannelType,
			&modelsJSON,
			&item.Enabled,
			&item.IsSystemDefault,
			&hasToken,
			&item.ReverseUsername,
			&item.ReverseGroupName,
			&hasReversePassword,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan aggregation target: %w", err)
		}
		item.HasToken = hasToken > 0
		item.HasReversePassword = hasReversePassword > 0
		item.Source = "database"
		if err := decodeAggregationTargetModels(modelsJSON, &item); err != nil {
			return nil, fmt.Errorf("decode aggregation target %s models: %w", item.Code, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregation targets: %w", err)
	}
	return items, nil
}

func ListAggregationTargetSecrets(ctx context.Context, db *sql.DB, enabledOnly bool) ([]AggregationTargetRecord, error) {
	query := `
SELECT
  code,
  name,
  base_url,
  COALESCE(NULLIF(connection_mode, ''), 'api'),
  COALESCE(NULLIF(channel_type, ''), 'anthropic'),
  models_json,
  enabled,
  is_system_default,
  token_ciphertext <> '',
  reverse_username,
  COALESCE(NULLIF(reverse_group_name, ''), 'default'),
  COALESCE(reverse_password_ciphertext, '') <> '',
  created_at,
  updated_at,
  token_ciphertext,
  COALESCE(reverse_password_ciphertext, '')
FROM aggregation_targets`
	if enabledOnly {
		query += `
WHERE enabled = TRUE`
	}
	query += `
ORDER BY sort_order ASC, name ASC, code ASC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list aggregation target secrets: %w", err)
	}
	defer rows.Close()

	items := make([]AggregationTargetRecord, 0)
	for rows.Next() {
		var item AggregationTargetRecord
		var modelsJSON []byte
		var hasToken int
		var hasReversePassword int
		if err := rows.Scan(
			&item.Code,
			&item.Name,
			&item.BaseURL,
			&item.ConnectionMode,
			&item.ChannelType,
			&modelsJSON,
			&item.Enabled,
			&item.IsSystemDefault,
			&hasToken,
			&item.ReverseUsername,
			&item.ReverseGroupName,
			&hasReversePassword,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.TokenCiphertext,
			&item.ReversePasswordCiphertext,
		); err != nil {
			return nil, fmt.Errorf("scan aggregation target secret: %w", err)
		}
		item.HasToken = hasToken > 0
		item.HasReversePassword = hasReversePassword > 0
		item.Source = "database"
		if err := decodeAggregationTargetModels(modelsJSON, &item); err != nil {
			return nil, fmt.Errorf("decode aggregation target %s models: %w", item.Code, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregation target secrets: %w", err)
	}
	return items, nil
}

func CountAggregationTargets(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM aggregation_targets`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count aggregation targets: %w", err)
	}
	return count, nil
}

func GetAggregationTarget(ctx context.Context, db *sql.DB, code string) (AggregationTargetRecord, error) {
	var item AggregationTargetRecord
	var modelsJSON []byte
	var hasToken int
	var hasReversePassword int
	err := db.QueryRowContext(ctx, `
SELECT
  code,
  name,
  base_url,
  COALESCE(NULLIF(connection_mode, ''), 'api'),
  COALESCE(NULLIF(channel_type, ''), 'anthropic'),
  models_json,
  enabled,
  is_system_default,
  token_ciphertext <> '',
  reverse_username,
  COALESCE(NULLIF(reverse_group_name, ''), 'default'),
  COALESCE(reverse_password_ciphertext, '') <> '',
  created_at,
  updated_at,
  token_ciphertext,
  COALESCE(reverse_password_ciphertext, '')
FROM aggregation_targets
WHERE code = ?`, code).Scan(
		&item.Code,
		&item.Name,
		&item.BaseURL,
		&item.ConnectionMode,
		&item.ChannelType,
		&modelsJSON,
		&item.Enabled,
		&item.IsSystemDefault,
		&hasToken,
		&item.ReverseUsername,
		&item.ReverseGroupName,
		&hasReversePassword,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.TokenCiphertext,
		&item.ReversePasswordCiphertext,
	)
	if err == sql.ErrNoRows {
		return item, ErrAggregationTargetNotFound
	}
	if err != nil {
		return item, fmt.Errorf("get aggregation target: %w", err)
	}
	item.HasToken = hasToken > 0
	item.HasReversePassword = hasReversePassword > 0
	item.Source = "database"
	if err := decodeAggregationTargetModels(modelsJSON, &item); err != nil {
		return item, fmt.Errorf("decode aggregation target %s models: %w", item.Code, err)
	}
	return item, nil
}

func SaveAggregationTarget(ctx context.Context, db *sql.DB, target AggregationTargetUpsert) error {
	modelsJSON, err := json.Marshal(target.Models)
	if err != nil {
		return fmt.Errorf("encode aggregation target models: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save aggregation target: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if target.IsSystemDefault {
		if _, err = tx.ExecContext(ctx, `
UPDATE aggregation_targets
SET is_system_default = FALSE
WHERE code <> ? AND is_system_default = TRUE`, target.Code); err != nil {
			return fmt.Errorf("clear previous system default aggregation target: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO aggregation_targets
  (code, name, base_url, connection_mode, channel_type, models_json, token_ciphertext, reverse_username, reverse_group_name, reverse_password_ciphertext, enabled, is_system_default, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  name = VALUES(name),
  base_url = VALUES(base_url),
  connection_mode = VALUES(connection_mode),
  channel_type = VALUES(channel_type),
  models_json = VALUES(models_json),
  token_ciphertext = VALUES(token_ciphertext),
  reverse_username = VALUES(reverse_username),
  reverse_group_name = VALUES(reverse_group_name),
  reverse_password_ciphertext = VALUES(reverse_password_ciphertext),
  enabled = VALUES(enabled),
  is_system_default = VALUES(is_system_default),
  sort_order = VALUES(sort_order)`,
		target.Code,
		target.Name,
		target.BaseURL,
		target.ConnectionMode,
		target.ChannelType,
		string(modelsJSON),
		target.TokenCiphertext,
		target.ReverseUsername,
		target.ReverseGroupName,
		target.ReversePasswordCiphertext,
		target.Enabled,
		target.IsSystemDefault,
		target.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("save aggregation target: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit save aggregation target: %w", err)
	}
	return nil
}

func decodeAggregationTargetModels(raw []byte, target *AggregationTargetRecord) error {
	if len(raw) == 0 {
		target.Models = []string{}
		return nil
	}
	if err := json.Unmarshal(raw, &target.Models); err != nil {
		return err
	}
	if target.Models == nil {
		target.Models = []string{}
	}
	return nil
}

func DeleteAggregationTarget(ctx context.Context, db *sql.DB, code string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete aggregation target: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `DELETE FROM aggregation_targets WHERE code = ?`, code)
	if err != nil {
		return fmt.Errorf("delete aggregation target: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted aggregation target: %w", err)
	}
	if affected == 0 {
		return ErrAggregationTargetNotFound
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit delete aggregation target: %w", err)
	}
	return nil
}
