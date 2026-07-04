package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"keyhub/internal/keys"
)

var (
	ErrAPIKeyNotFound          = errors.New("api key not found")
	ErrActiveAPIKeyDeleteBlock = errors.New("active api key must be disabled before deletion")
)

type APIKeyRecord struct {
	ID               int64     `json:"id"`
	CategoryCode     string    `json:"categoryCode"`
	KeyHint          string    `json:"keyHint"`
	Region           string    `json:"region"`
	BaseURL          string    `json:"baseUrl"`
	Models           []string  `json:"models"`
	Tag              string    `json:"tag"`
	GroupName        string    `json:"groupName"`
	Note             string    `json:"note"`
	ExpectedTPM      int64     `json:"expectedTpm"`
	UsageQuota30d    int64     `json:"usageQuota30d"`
	UsageUSD30d      float64   `json:"usageUsd30d"`
	Status           string    `json:"status"`
	NewAPIChannelID  *int64    `json:"newApiChannelId,omitempty"`
	LastSyncStatus   string    `json:"lastSyncStatus"`
	LastHealthStatus string    `json:"lastHealthStatus"`
	SuccessCount     int64     `json:"successCount"`
	ErrorCount       int64     `json:"errorCount"`
	LastError        string    `json:"lastError"`
	CreatedAt        time.Time `json:"createdAt"`
}

type DeletedAPIKeyRecord struct {
	ID              int64
	CategoryCode    string
	KeyHint         string
	Status          string
	NewAPIChannelID sql.NullInt64
}

type APIKeyPushBinding struct {
	APIKeyID        int64  `json:"apiKeyId"`
	TargetCode      string `json:"targetCode"`
	RemoteChannelID int64  `json:"remoteChannelId"`
	Status          string `json:"status"`
}

type APIKeyForSync struct {
	ID              int64
	CategoryCode    string
	CategoryLabel   string
	NewAPIType      int
	KeyCiphertext   string
	KeyHint         string
	Region          string
	BaseURL         string
	Models          []string
	Tag             string
	GroupName       string
	Note            string
	Status          string
	NewAPIChannelID sql.NullInt64
}

type APIKeyList struct {
	Items []APIKeyRecord `json:"items"`
	Total int64          `json:"total"`
}

type SyncEventRecord struct {
	ID             int64           `json:"id"`
	APIKeyID       *int64          `json:"apiKeyId,omitempty"`
	Action         string          `json:"action"`
	Status         string          `json:"status"`
	RequestPayload json.RawMessage `json:"requestPayload,omitempty"`
	Response       json.RawMessage `json:"response,omitempty"`
	ErrorMessage   string          `json:"errorMessage"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type ImportAPIKey struct {
	CategoryCode string
	Ciphertext   string
	Fingerprint  string
	KeyHint      string
	Region       string
	BaseURL      string
	Models       []string
	Tag          string
	GroupName    string
	Note         string
	ExpectedTPM  int64
	Status       string
}

type ImportResult struct {
	BatchID    int64 `json:"batchId"`
	Total      int   `json:"total"`
	Imported   int   `json:"imported"`
	Duplicates int   `json:"duplicates"`
	Failed     int   `json:"failed"`
}

func LoadCategories(ctx context.Context, db *sql.DB) ([]keys.Category, error) {
	rows, err := db.QueryContext(ctx, `
SELECT category_code, category_label, newapi_type, default_models_json
FROM category_pool_rules
ORDER BY sort_order, category_label`)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	defer rows.Close()

	categories := make([]keys.Category, 0)
	for rows.Next() {
		var category keys.Category
		var modelsJSON []byte
		if err := rows.Scan(&category.Code, &category.Label, &category.NewAPIType, &modelsJSON); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		if err := json.Unmarshal(modelsJSON, &category.DefaultModels); err != nil {
			return nil, fmt.Errorf("parse default models for %s: %w", category.Code, err)
		}
		category.KeyFormat = keys.KeyFormat(category.Code)
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}

	return categories, nil
}

func LoadCategory(ctx context.Context, db *sql.DB, code string) (keys.Category, error) {
	categories, err := LoadCategories(ctx, db)
	if err != nil {
		return keys.Category{}, err
	}
	for _, category := range categories {
		if category.Code == code {
			return category, nil
		}
	}
	return keys.Category{}, fmt.Errorf("unknown category: %s", code)
}

func ExistingFingerprints(ctx context.Context, db *sql.DB, fingerprints []string) (map[string]bool, error) {
	existing := make(map[string]bool)
	if len(fingerprints) == 0 {
		return existing, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(fingerprints)), ",")
	args := make([]any, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		args = append(args, fingerprint)
	}
	rows, err := db.QueryContext(ctx, "SELECT key_fingerprint FROM api_keys WHERE key_fingerprint IN ("+placeholders+")", args...)
	if err != nil {
		return nil, fmt.Errorf("load existing fingerprints: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fingerprint string
		if err := rows.Scan(&fingerprint); err != nil {
			return nil, fmt.Errorf("scan fingerprint: %w", err)
		}
		existing[fingerprint] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fingerprints: %w", err)
	}
	return existing, nil
}

func ImportKeys(ctx context.Context, db *sql.DB, categoryCode string, tag string, groupName string, note string, actor string, total int, rows []ImportAPIKey, duplicates int, failed int) (ImportResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("begin import: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
INSERT INTO key_import_batches
  (category_code, tag, group_name, total_count, success_count, failed_count, default_to_inventory, note, created_by)
VALUES (?, ?, ?, ?, 0, 0, ?, ?, ?)`,
		categoryCode, tag, groupName, total, true, note, actor)
	if err != nil {
		return ImportResult{}, fmt.Errorf("create import batch: %w", err)
	}
	batchID, err := result.LastInsertId()
	if err != nil {
		return ImportResult{}, fmt.Errorf("read import batch id: %w", err)
	}

	imported := 0
	for _, row := range rows {
		modelsJSON, marshalErr := json.Marshal(row.Models)
		if marshalErr != nil {
			err = fmt.Errorf("marshal models: %w", marshalErr)
			return ImportResult{}, err
		}
		insertResult, execErr := tx.ExecContext(ctx, `
INSERT IGNORE INTO api_keys
  (import_batch_id, category_code, key_ciphertext, key_fingerprint, key_hint, region, base_url, models_json, tag, group_name, note, created_by, expected_tpm, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			batchID, row.CategoryCode, row.Ciphertext, row.Fingerprint, row.KeyHint, row.Region, row.BaseURL, string(modelsJSON), row.Tag, row.GroupName, row.Note, actor, row.ExpectedTPM, row.Status)
		if execErr != nil {
			err = fmt.Errorf("insert api key: %w", execErr)
			return ImportResult{}, err
		}
		affected, affectedErr := insertResult.RowsAffected()
		if affectedErr == nil && affected > 0 {
			imported++
		} else {
			duplicates++
		}
	}

	failed += duplicates
	if _, err = tx.ExecContext(ctx, "UPDATE key_import_batches SET success_count = ?, failed_count = ? WHERE id = ?", imported, failed, batchID); err != nil {
		return ImportResult{}, fmt.Errorf("update import batch counts: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO audit_logs(actor, action, target_type, target_id, detail_json)
VALUES (?, 'keys.import', 'key_import_batch', ?, JSON_OBJECT('total', ?, 'imported', ?, 'duplicates', ?, 'failed', ?))`,
		actor, batchID, total, imported, duplicates, failed); err != nil {
		return ImportResult{}, fmt.Errorf("write audit log: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit import: %w", err)
	}

	return ImportResult{
		BatchID:    batchID,
		Total:      total,
		Imported:   imported,
		Duplicates: duplicates,
		Failed:     failed,
	}, nil
}

func ListAPIKeys(ctx context.Context, db *sql.DB, limit int, offset int, owner string) (APIKeyList, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	owner = strings.TrimSpace(owner)
	where := ""
	countArgs := []any{}
	if owner != "" {
		where = "WHERE created_by = ?"
		countArgs = append(countArgs, owner)
	}

	result := APIKeyList{Items: []APIKeyRecord{}}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys "+where, countArgs...).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count api keys: %w", err)
	}

	usageSince := time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	queryArgs := []any{usageSince}
	if owner != "" {
		queryArgs = append(queryArgs, owner)
	}
	queryArgs = append(queryArgs, limit, offset)
	rows, err := db.QueryContext(ctx, `
SELECT
  k.id,
  k.category_code,
  k.key_hint,
  k.region,
  k.base_url,
  k.models_json,
  k.tag,
  k.group_name,
  k.note,
  k.expected_tpm,
  COALESCE(u.usage_quota_30d, 0),
  COALESCE(u.usage_usd_30d, 0),
  k.status,
  k.newapi_channel_id,
  k.last_sync_status,
  k.last_health_status,
  k.success_count,
  k.error_count,
  k.last_error,
  k.created_at
FROM api_keys k
LEFT JOIN (
  SELECT
    api_key_id,
    COALESCE(SUM(quota_amount), 0) AS usage_quota_30d,
    COALESCE(SUM(usd_amount), 0) AS usage_usd_30d
  FROM usage_daily_snapshots
  WHERE stat_date >= ? AND api_key_id IS NOT NULL
  GROUP BY api_key_id
) u ON u.api_key_id = k.id
`+strings.Replace(where, "created_by", "k.created_by", 1)+`
ORDER BY k.id DESC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item APIKeyRecord
		var modelsJSON []byte
		var newAPIChannelID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.CategoryCode,
			&item.KeyHint,
			&item.Region,
			&item.BaseURL,
			&modelsJSON,
			&item.Tag,
			&item.GroupName,
			&item.Note,
			&item.ExpectedTPM,
			&item.UsageQuota30d,
			&item.UsageUSD30d,
			&item.Status,
			&newAPIChannelID,
			&item.LastSyncStatus,
			&item.LastHealthStatus,
			&item.SuccessCount,
			&item.ErrorCount,
			&item.LastError,
			&item.CreatedAt,
		); err != nil {
			return result, fmt.Errorf("scan api key: %w", err)
		}
		if newAPIChannelID.Valid {
			item.NewAPIChannelID = &newAPIChannelID.Int64
		}
		if err := json.Unmarshal(modelsJSON, &item.Models); err != nil {
			return result, fmt.Errorf("parse key models: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate api keys: %w", err)
	}

	return result, nil
}

func LoadAPIKeyForSync(ctx context.Context, db *sql.DB, id int64) (APIKeyForSync, error) {
	var item APIKeyForSync
	var modelsJSON []byte
	err := db.QueryRowContext(ctx, `
SELECT
  k.id,
  k.category_code,
  r.category_label,
  r.newapi_type,
  k.key_ciphertext,
  k.key_hint,
  k.region,
  k.base_url,
  k.models_json,
  k.tag,
  k.group_name,
  k.note,
  k.status,
  k.newapi_channel_id
FROM api_keys k
JOIN category_pool_rules r ON r.category_code = k.category_code
WHERE k.id = ?`, id).Scan(
		&item.ID,
		&item.CategoryCode,
		&item.CategoryLabel,
		&item.NewAPIType,
		&item.KeyCiphertext,
		&item.KeyHint,
		&item.Region,
		&item.BaseURL,
		&modelsJSON,
		&item.Tag,
		&item.GroupName,
		&item.Note,
		&item.Status,
		&item.NewAPIChannelID,
	)
	if err != nil {
		return item, fmt.Errorf("load api key for sync: %w", err)
	}
	if err := json.Unmarshal(modelsJSON, &item.Models); err != nil {
		return item, fmt.Errorf("parse sync models: %w", err)
	}
	return item, nil
}

func MarkKeySyncSuccess(ctx context.Context, db *sql.DB, keyID int64, newAPIChannelID int64, status string) error {
	_, err := db.ExecContext(ctx, `
UPDATE api_keys
SET status = ?, newapi_channel_id = ?, last_sync_status = 'success', last_error = '', synced_at = CURRENT_TIMESTAMP
WHERE id = ?`, status, newAPIChannelID, keyID)
	if err != nil {
		return fmt.Errorf("mark key sync success: %w", err)
	}
	return nil
}

func UpsertAPIKeyPushBinding(ctx context.Context, db *sql.DB, keyID int64, targetCode string, remoteChannelID int64, status string) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO api_key_push_bindings
  (api_key_id, target_code, remote_channel_id, status, last_sync_status, last_error, synced_at)
VALUES (?, ?, ?, ?, 'success', '', CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  remote_channel_id = VALUES(remote_channel_id),
  status = VALUES(status),
  last_sync_status = 'success',
  last_error = '',
  synced_at = CURRENT_TIMESTAMP`,
		keyID, targetCode, remoteChannelID, status)
	if err != nil {
		return fmt.Errorf("upsert api key push binding: %w", err)
	}
	return nil
}

func ListAPIKeyPushBindings(ctx context.Context, db *sql.DB, keyID int64) ([]APIKeyPushBinding, error) {
	rows, err := db.QueryContext(ctx, `
SELECT api_key_id, target_code, remote_channel_id, status
FROM api_key_push_bindings
WHERE api_key_id = ? AND remote_channel_id > 0 AND status <> 'disabled'
ORDER BY target_code ASC`, keyID)
	if err != nil {
		return nil, fmt.Errorf("list api key push bindings: %w", err)
	}
	defer rows.Close()

	items := make([]APIKeyPushBinding, 0)
	for rows.Next() {
		var item APIKeyPushBinding
		if err := rows.Scan(&item.APIKeyID, &item.TargetCode, &item.RemoteChannelID, &item.Status); err != nil {
			return nil, fmt.Errorf("scan api key push binding: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key push bindings: %w", err)
	}
	return items, nil
}

func MarkAPIKeyPushBindingDisabled(ctx context.Context, db *sql.DB, keyID int64, targetCode string) error {
	_, err := db.ExecContext(ctx, `
UPDATE api_key_push_bindings
SET status = 'disabled', last_sync_status = 'success', last_error = '', synced_at = CURRENT_TIMESTAMP
WHERE api_key_id = ? AND target_code = ?`, keyID, targetCode)
	if err != nil {
		return fmt.Errorf("mark api key push binding disabled: %w", err)
	}
	return nil
}

func MarkAPIKeyPushBindingFailure(ctx context.Context, db *sql.DB, keyID int64, targetCode string, message string) error {
	_, err := db.ExecContext(ctx, `
UPDATE api_key_push_bindings
SET status = 'error', last_sync_status = 'failed', last_error = ?
WHERE api_key_id = ? AND target_code = ?`, message, keyID, targetCode)
	if err != nil {
		return fmt.Errorf("mark api key push binding failure: %w", err)
	}
	return nil
}

func MarkKeySyncFailure(ctx context.Context, db *sql.DB, keyID int64, message string) error {
	_, err := db.ExecContext(ctx, `
UPDATE api_keys
SET last_sync_status = 'failed', last_error = ?
WHERE id = ?`, message, keyID)
	if err != nil {
		return fmt.Errorf("mark key sync failure: %w", err)
	}
	return nil
}

func MarkKeyDisabled(ctx context.Context, db *sql.DB, keyID int64) error {
	_, err := db.ExecContext(ctx, `
UPDATE api_keys
SET status = 'disabled', last_sync_status = 'success', synced_at = CURRENT_TIMESTAMP
WHERE id = ?`, keyID)
	if err != nil {
		return fmt.Errorf("mark key disabled: %w", err)
	}
	return nil
}

func DeleteAPIKey(ctx context.Context, db *sql.DB, keyID int64) (DeletedAPIKeyRecord, error) {
	var record DeletedAPIKeyRecord
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return record, fmt.Errorf("begin delete api key: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	err = tx.QueryRowContext(ctx, `
SELECT id, category_code, key_hint, status, newapi_channel_id
FROM api_keys
WHERE id = ?
FOR UPDATE`, keyID).Scan(
		&record.ID,
		&record.CategoryCode,
		&record.KeyHint,
		&record.Status,
		&record.NewAPIChannelID,
	)
	if err == sql.ErrNoRows {
		return record, ErrAPIKeyNotFound
	}
	if err != nil {
		return record, fmt.Errorf("load api key for delete: %w", err)
	}
	if record.Status == "active" {
		return record, ErrActiveAPIKeyDeleteBlock
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, keyID); err != nil {
		return record, fmt.Errorf("delete api key: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return record, fmt.Errorf("commit delete api key: %w", err)
	}
	committed = true
	return record, nil
}

func InsertSyncEvent(ctx context.Context, db *sql.DB, keyID *int64, action string, status string, requestPayload any, response any, errorMessage string) error {
	requestJSON, err := nullableJSON(requestPayload)
	if err != nil {
		return err
	}
	responseJSON, err := nullableJSON(response)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO newapi_sync_events(api_key_id, action, status, request_payload_json, response_json, error_message)
VALUES (?, ?, ?, ?, ?, ?)`, keyID, action, status, requestJSON, responseJSON, errorMessage)
	if err != nil {
		return fmt.Errorf("insert sync event: %w", err)
	}
	return nil
}

func ListSyncEvents(ctx context.Context, db *sql.DB, limit int, owner string) ([]SyncEventRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	owner = strings.TrimSpace(owner)
	fromClause := "FROM newapi_sync_events e"
	where := ""
	args := []any{}
	if owner != "" {
		fromClause += " JOIN api_keys k ON k.id = e.api_key_id"
		where = "WHERE k.created_by = ?"
		args = append(args, owner)
	}
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, `
SELECT e.id, e.api_key_id, e.action, e.status, e.request_payload_json, e.response_json, e.error_message, e.created_at
`+fromClause+`
`+where+`
ORDER BY e.id DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list sync events: %w", err)
	}
	defer rows.Close()

	events := make([]SyncEventRecord, 0)
	for rows.Next() {
		var event SyncEventRecord
		var keyID sql.NullInt64
		var requestJSON []byte
		var responseJSON []byte
		if err := rows.Scan(&event.ID, &keyID, &event.Action, &event.Status, &requestJSON, &responseJSON, &event.ErrorMessage, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sync event: %w", err)
		}
		if keyID.Valid {
			event.APIKeyID = &keyID.Int64
		}
		if len(requestJSON) > 0 {
			event.RequestPayload = json.RawMessage(requestJSON)
		}
		if len(responseJSON) > 0 {
			event.Response = json.RawMessage(responseJSON)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync events: %w", err)
	}
	return events, nil
}

func nullableJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal sync event json: %w", err)
	}
	return string(payload), nil
}
