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
	ID                int64               `json:"id"`
	CategoryCode      string              `json:"categoryCode"`
	AWSCredentialType string              `json:"awsCredentialType,omitempty"`
	KeyHint           string              `json:"keyHint"`
	Region            string              `json:"region"`
	BaseURL           string              `json:"baseUrl"`
	Models            []string            `json:"models"`
	Tag               string              `json:"tag"`
	GroupName         string              `json:"groupName"`
	Note              string              `json:"note"`
	CreatedBy         string              `json:"createdBy"`
	ExpectedTPM       int64               `json:"expectedTpm"`
	UsageQuota30d     int64               `json:"usageQuota30d"`
	UsageUSD30d       float64             `json:"usageUsd30d"`
	Status            string              `json:"status"`
	NewAPIChannelID   *int64              `json:"newApiChannelId,omitempty"`
	PushBindings      []APIKeyPushBinding `json:"pushBindings,omitempty"`
	LastSyncStatus    string              `json:"lastSyncStatus"`
	LastHealthStatus  string              `json:"lastHealthStatus"`
	SuccessCount      int64               `json:"successCount"`
	ErrorCount        int64               `json:"errorCount"`
	LastError         string              `json:"lastError"`
	FirstUsedAt       *time.Time          `json:"firstUsedAt,omitempty"`
	LastUsedAt        *time.Time          `json:"lastUsedAt,omitempty"`
	CreatedAt         time.Time           `json:"createdAt"`
}

type DeletedAPIKeyRecord struct {
	ID              int64
	CategoryCode    string
	KeyHint         string
	Status          string
	NewAPIChannelID sql.NullInt64
}

type APIKeyPushBinding struct {
	APIKeyID              int64      `json:"apiKeyId"`
	TargetCode            string     `json:"targetCode"`
	RemoteChannelID       int64      `json:"remoteChannelId"`
	Status                string     `json:"status"`
	RemoteStatus          *int       `json:"remoteStatus,omitempty"`
	RemoteStatusCheckedAt *time.Time `json:"remoteStatusCheckedAt,omitempty"`
}

type APIKeyForSync struct {
	ID                int64
	CategoryCode      string
	AWSCredentialType string
	CreatedBy         string
	CategoryLabel     string
	NewAPIType        int
	KeyCiphertext     string
	KeyHint           string
	Region            string
	BaseURL           string
	Models            []string
	Tag               string
	GroupName         string
	Note              string
	Status            string
	NewAPIChannelID   sql.NullInt64
}

type APIKeySecret struct {
	ID            int64
	CategoryCode  string
	KeyCiphertext string
	KeyHint       string
}

type APIKeyList struct {
	Items         []APIKeyRecord `json:"items"`
	Total         int64          `json:"total"`
	TotalUsageUSD float64        `json:"totalUsageUsd"`
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
	CategoryCode      string
	AWSCredentialType string
	Ciphertext        string
	Fingerprint       string
	KeyHint           string
	Region            string
	BaseURL           string
	Models            []string
	Tag               string
	GroupName         string
	Note              string
	ExpectedTPM       int64
	Status            string
}

type ImportResult struct {
	BatchID        int64   `json:"batchId"`
	Total          int     `json:"total"`
	Imported       int     `json:"imported"`
	Duplicates     int     `json:"duplicates"`
	Failed         int     `json:"failed"`
	ImportedKeyIDs []int64 `json:"importedKeyIds"`
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
	defer tx.Rollback()

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
	importedKeyIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		keyID, duplicate, insertErr := insertImportedAPIKey(ctx, tx, batchID, actor, row)
		if insertErr != nil {
			return ImportResult{}, fmt.Errorf("insert api key: %w", insertErr)
		}
		if !duplicate {
			imported++
			importedKeyIDs = append(importedKeyIDs, keyID)
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
		BatchID:        batchID,
		Total:          total,
		Imported:       imported,
		Duplicates:     duplicates,
		Failed:         failed,
		ImportedKeyIDs: importedKeyIDs,
	}, nil
}

func ListAPIKeys(ctx context.Context, db *sql.DB, limit int, offset int, owner string, includePushBindings bool) (APIKeyList, error) {
	owners := []string{}
	if strings.TrimSpace(owner) != "" {
		owners = append(owners, owner)
	}
	return ListAPIKeysForOwners(ctx, db, limit, offset, owners, includePushBindings)
}

func ListAPIKeysForOwners(ctx context.Context, db *sql.DB, limit int, offset int, owners []string, includePushBindings bool) (APIKeyList, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	cleanedOwners := make([]string, 0, len(owners))
	seenOwners := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		owner = strings.TrimSpace(owner)
		if owner == "" {
			continue
		}
		if _, exists := seenOwners[owner]; exists {
			continue
		}
		seenOwners[owner] = struct{}{}
		cleanedOwners = append(cleanedOwners, owner)
	}
	where := ""
	countArgs := []any{}
	if len(cleanedOwners) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cleanedOwners)), ",")
		where = "WHERE created_by IN (" + placeholders + ")"
		for _, owner := range cleanedOwners {
			countArgs = append(countArgs, owner)
		}
	}

	result := APIKeyList{Items: []APIKeyRecord{}}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys "+where, countArgs...).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count api keys: %w", err)
	}
	usageWhere := strings.Replace(where, "created_by", "k.created_by", 1)
	if err := db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(latest_usage.usd_amount), 0)
FROM (`+latestUsageSnapshotsSQL+`) latest_usage
JOIN api_keys k ON k.id = latest_usage.api_key_id
`+usageWhere, countArgs...).Scan(&result.TotalUsageUSD); err != nil {
		return result, fmt.Errorf("sum api key usage: %w", err)
	}

	queryArgs := append([]any{}, countArgs...)
	queryArgs = append(queryArgs, limit, offset)
	rows, err := db.QueryContext(ctx, `
SELECT
  k.id,
  k.category_code,
  k.aws_credential_type,
  k.key_hint,
  k.region,
  k.base_url,
  k.models_json,
  k.tag,
  k.group_name,
  k.note,
  k.created_by,
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
  k.first_used_at,
  k.last_used_at,
  k.created_at
FROM api_keys k
LEFT JOIN (
  SELECT
    api_key_id,
    COALESCE(SUM(quota_amount), 0) AS usage_quota_30d,
    COALESCE(SUM(usd_amount), 0) AS usage_usd_30d
  FROM (`+latestUsageSnapshotsSQL+`) latest_usage
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
		var firstUsedAt sql.NullTime
		var lastUsedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.CategoryCode,
			&item.AWSCredentialType,
			&item.KeyHint,
			&item.Region,
			&item.BaseURL,
			&modelsJSON,
			&item.Tag,
			&item.GroupName,
			&item.Note,
			&item.CreatedBy,
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
			&firstUsedAt,
			&lastUsedAt,
			&item.CreatedAt,
		); err != nil {
			return result, fmt.Errorf("scan api key: %w", err)
		}
		if newAPIChannelID.Valid {
			item.NewAPIChannelID = &newAPIChannelID.Int64
		}
		if firstUsedAt.Valid {
			item.FirstUsedAt = &firstUsedAt.Time
		}
		if lastUsedAt.Valid {
			item.LastUsedAt = &lastUsedAt.Time
		}
		if err := json.Unmarshal(modelsJSON, &item.Models); err != nil {
			return result, fmt.Errorf("parse key models: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate api keys: %w", err)
	}
	if includePushBindings {
		if err := loadAPIKeyPushBindingsForList(ctx, db, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

func loadAPIKeyPushBindingsForList(ctx context.Context, db *sql.DB, result *APIKeyList) error {
	if result == nil || len(result.Items) == 0 {
		return nil
	}
	indexByID := make(map[int64]int, len(result.Items))
	placeholders := make([]string, 0, len(result.Items))
	args := make([]any, 0, len(result.Items))
	for index, item := range result.Items {
		indexByID[item.ID] = index
		placeholders = append(placeholders, "?")
		args = append(args, item.ID)
	}
	rows, err := db.QueryContext(ctx, `
SELECT api_key_id, target_code, remote_channel_id, status, remote_status, remote_status_checked_at
FROM api_key_push_bindings
WHERE api_key_id IN (`+strings.Join(placeholders, ",")+`) AND remote_channel_id > 0 AND status <> 'disabled'
ORDER BY api_key_id ASC, target_code ASC`, args...)
	if err != nil {
		return fmt.Errorf("list api key push bindings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var binding APIKeyPushBinding
		var remoteStatus sql.NullInt64
		var remoteStatusCheckedAt sql.NullTime
		if err := rows.Scan(
			&binding.APIKeyID,
			&binding.TargetCode,
			&binding.RemoteChannelID,
			&binding.Status,
			&remoteStatus,
			&remoteStatusCheckedAt,
		); err != nil {
			return fmt.Errorf("scan api key push binding: %w", err)
		}
		if remoteStatus.Valid {
			value := int(remoteStatus.Int64)
			binding.RemoteStatus = &value
		}
		if remoteStatusCheckedAt.Valid {
			value := remoteStatusCheckedAt.Time
			binding.RemoteStatusCheckedAt = &value
		}
		if index, ok := indexByID[binding.APIKeyID]; ok {
			result.Items[index].PushBindings = append(result.Items[index].PushBindings, binding)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate api key push bindings: %w", err)
	}
	return nil
}

func LoadAPIKeyForSync(ctx context.Context, db *sql.DB, id int64) (APIKeyForSync, error) {
	var item APIKeyForSync
	var modelsJSON []byte
	err := db.QueryRowContext(ctx, `
SELECT
  k.id,
  k.category_code,
  k.aws_credential_type,
  k.created_by,
  '',
  0,
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
WHERE k.id = ?`, id).Scan(
		&item.ID,
		&item.CategoryCode,
		&item.AWSCredentialType,
		&item.CreatedBy,
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

func LoadAPIKeySecret(ctx context.Context, db *sql.DB, id int64) (APIKeySecret, error) {
	var item APIKeySecret
	err := db.QueryRowContext(ctx, `
SELECT id, category_code, key_ciphertext, key_hint
FROM api_keys
WHERE id = ?`, id).Scan(
		&item.ID,
		&item.CategoryCode,
		&item.KeyCiphertext,
		&item.KeyHint,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrAPIKeyNotFound
	}
	if err != nil {
		return item, fmt.Errorf("load api key secret: %w", err)
	}
	return item, nil
}

func APIKeyBelongsToOwner(ctx context.Context, db *sql.DB, id int64, owner string) (bool, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return true, nil
	}
	var count int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM api_keys
WHERE id = ? AND created_by = ?`, id, owner).Scan(&count); err != nil {
		return false, fmt.Errorf("check api key owner: %w", err)
	}
	return count > 0, nil
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
  remote_status = NULL,
  remote_status_checked_at = NULL,
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

func GetActiveAPIKeyPushBinding(ctx context.Context, db *sql.DB, keyID int64, targetCode string) (APIKeyPushBinding, bool, error) {
	var item APIKeyPushBinding
	err := db.QueryRowContext(ctx, `
SELECT api_key_id, target_code, remote_channel_id, status
FROM api_key_push_bindings
WHERE api_key_id = ? AND target_code = ? AND remote_channel_id > 0 AND status = 'active'
LIMIT 1`, keyID, targetCode).Scan(&item.APIKeyID, &item.TargetCode, &item.RemoteChannelID, &item.Status)
	if err == nil {
		return item, true, nil
	}
	if err == sql.ErrNoRows {
		return item, false, nil
	}
	return item, false, fmt.Errorf("get api key push binding: %w", err)
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

func ListFailedActivationTargetCodes(ctx context.Context, db *sql.DB, keyID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT request_payload_json
FROM newapi_sync_events
WHERE api_key_id = ? AND action = 'activate' AND status = 'failed'
ORDER BY id DESC
LIMIT 200`, keyID)
	if err != nil {
		return nil, fmt.Errorf("list failed activation targets: %w", err)
	}
	defer rows.Close()

	targetCodes := make([]string, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var requestJSON []byte
		if err := rows.Scan(&requestJSON); err != nil {
			return nil, fmt.Errorf("scan failed activation target: %w", err)
		}
		var payload struct {
			TargetCode string `json:"targetCode"`
		}
		if len(requestJSON) == 0 || json.Unmarshal(requestJSON, &payload) != nil {
			continue
		}
		targetCode := strings.TrimSpace(payload.TargetCode)
		if targetCode == "" {
			continue
		}
		if _, exists := seen[targetCode]; exists {
			continue
		}
		seen[targetCode] = struct{}{}
		targetCodes = append(targetCodes, targetCode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate failed activation targets: %w", err)
	}
	return targetCodes, nil
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
