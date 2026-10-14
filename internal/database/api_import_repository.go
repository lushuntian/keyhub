package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

type APIImportRow struct {
	InputIndex int
	Key        ImportAPIKey
}

type APIImportTarget struct {
	Code        string
	ChannelType string
	Signature   string
}

type APIImportDelivery struct {
	ChannelType string `json:"channelType"`
	Status      string `json:"status"`
	ErrorCode   string `json:"errorCode,omitempty"`
}

type APIImportItem struct {
	ID           int64               `json:"-"`
	InputIndex   int                 `json:"inputIndex"`
	BaseURL      string              `json:"baseUrl,omitempty"`
	KeyID        *int64              `json:"keyId,omitempty"`
	ImportStatus string              `json:"importStatus"`
	Deliveries   []APIImportDelivery `json:"deliveries"`
}

type APIImportJob struct {
	ID           string          `json:"id"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"createdAt"`
	Total        int             `json:"total"`
	Imported     int             `json:"imported"`
	Duplicates   int             `json:"duplicates"`
	PublishTotal int             `json:"publishTotal"`
	Succeeded    int             `json:"succeeded"`
	Failed       int             `json:"failed"`
	NeedsReview  int             `json:"needsReview"`
	Pending      int             `json:"pending"`
	Items        []APIImportItem `json:"items"`
}

func FindAPIImportRequest(ctx context.Context, db *sql.DB, userID int64, idempotencyKey string) (string, string, error) {
	var id, hash string
	err := db.QueryRowContext(ctx, `SELECT id, request_hash FROM api_import_jobs WHERE user_id = ? AND idempotency_key = ?`, userID, idempotencyKey).Scan(&id, &hash)
	return id, hash, err
}

// Local keys, item outcomes, and the durable publishing queue commit together.
func EnqueueAPIImport(ctx context.Context, db *sql.DB, jobID string, user *AdminUser, tokenID int64, idempotencyKey, requestHash string, rows []APIImportRow, targets []APIImportTarget) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var enabledID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM admin_users WHERE id = ? AND disabled = FALSE FOR UPDATE`, user.ID).Scan(&enabledID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO key_import_batches(category_code, tag, group_name, total_count, success_count, failed_count, default_to_inventory, note, created_by) VALUES ('', '', 'default', ?, 0, 0, TRUE, '', ?)`, len(rows), user.Username)
	if err != nil {
		return err
	}
	batchID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO api_import_jobs(id, user_id, token_id, idempotency_key, request_hash, batch_id) VALUES (?, ?, ?, ?, ?, ?)`, jobID, user.ID, tokenID, idempotencyKey, requestHash, batchID); err != nil {
		return err
	}
	imported, duplicates := 0, 0
	for _, row := range rows {
		keyID, duplicate, err := insertImportedAPIKey(ctx, tx, batchID, user.Username, row.Key)
		if err != nil {
			return err
		}
		outcome := "imported"
		var nullableKeyID any = keyID
		if duplicate {
			outcome = "duplicate"
			nullableKeyID = nil
			duplicates++
		} else {
			imported++
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO api_import_items(job_id, input_index, base_url, api_key_id, outcome) VALUES (?, ?, ?, ?, ?)`, jobID, row.InputIndex, row.Key.BaseURL, nullableKeyID, outcome)
		if err != nil {
			return err
		}
		itemID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if duplicate {
			continue
		}
		for _, target := range targets {
			if _, err := tx.ExecContext(ctx, `INSERT INTO api_import_tasks(item_id, target_code, channel_type, target_signature) VALUES (?, ?, ?, ?)`, itemID, target.Code, target.ChannelType, target.Signature); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE key_import_batches SET success_count = ?, failed_count = ? WHERE id = ?`, imported, duplicates, batchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor, action, target_type, target_id, detail_json) VALUES (?, 'api.keys.import', 'key_import_batch', ?, JSON_OBJECT('jobId', ?, 'tokenId', ?, 'imported', ?, 'duplicates', ?))`, user.Username, batchID, jobID, tokenID, imported, duplicates); err != nil {
		return err
	}
	return tx.Commit()
}

func insertImportedAPIKey(ctx context.Context, tx *sql.Tx, batchID int64, actor string, row ImportAPIKey) (int64, bool, error) {
	models, err := json.Marshal(row.Models)
	if err != nil {
		return 0, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO api_keys
 (import_batch_id, category_code, aws_credential_type, key_ciphertext, key_fingerprint, key_hint, region, base_url, models_json, tag, group_name, note, created_by, expected_tpm, status)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, batchID, row.CategoryCode, row.AWSCredentialType, row.Ciphertext, row.Fingerprint, row.KeyHint, row.Region, row.BaseURL, string(models), row.Tag, row.GroupName, row.Note, actor, row.ExpectedTPM, row.Status)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return 0, true, nil
		}
		return 0, false, err
	}
	id, err := result.LastInsertId()
	return id, false, err
}

func LoadAPIImportJob(ctx context.Context, db *sql.DB, userID int64, id string) (APIImportJob, error) {
	job := APIImportJob{Items: []APIImportItem{}}
	if err := db.QueryRowContext(ctx, `SELECT id, created_at FROM api_import_jobs WHERE id = ? AND user_id = ?`, id, userID).Scan(&job.ID, &job.CreatedAt); err != nil {
		return job, err
	}
	rows, err := db.QueryContext(ctx, `SELECT i.id, i.input_index, i.base_url, i.api_key_id, i.outcome, t.channel_type, t.status, t.error_code
 FROM api_import_items i LEFT JOIN api_import_tasks t ON t.item_id = i.id WHERE i.job_id = ? ORDER BY i.id, t.id`, id)
	if err != nil {
		return job, err
	}
	defer rows.Close()
	for rows.Next() {
		var item APIImportItem
		var channelType, status, errorCode sql.NullString
		if err := rows.Scan(&item.ID, &item.InputIndex, &item.BaseURL, &item.KeyID, &item.ImportStatus, &channelType, &status, &errorCode); err != nil {
			return job, err
		}
		if len(job.Items) == 0 || job.Items[len(job.Items)-1].ID != item.ID {
			item.Deliveries = []APIImportDelivery{}
			job.Items = append(job.Items, item)
		}
		if status.Valid {
			last := &job.Items[len(job.Items)-1]
			last.Deliveries = append(last.Deliveries, APIImportDelivery{channelType.String, status.String, errorCode.String})
		}
	}
	if err := rows.Err(); err != nil {
		return job, err
	}
	summarizeAPIImport(&job)
	return job, nil
}

func summarizeAPIImport(job *APIImportJob) {
	running := false
	for _, item := range job.Items {
		job.Total++
		if item.ImportStatus == "duplicate" {
			job.Duplicates++
		} else {
			job.Imported++
		}
		for _, delivery := range item.Deliveries {
			job.PublishTotal++
			switch delivery.Status {
			case "succeeded":
				job.Succeeded++
			case "failed":
				job.Failed++
			case "needs_review":
				job.NeedsReview++
			default:
				job.Pending++
				running = running || delivery.Status == "running"
			}
		}
	}
	switch {
	case running:
		job.Status = "running"
	case job.Pending > 0:
		job.Status = "queued"
	case job.NeedsReview > 0:
		job.Status = "needs_review"
	case job.Failed > 0 && job.Succeeded > 0:
		job.Status = "partial_failed"
	case job.Failed > 0:
		job.Status = "failed"
	default:
		job.Status = "succeeded"
	}
}

func RetryAPIImport(ctx context.Context, db *sql.DB, userID, tokenID int64, id string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var batchID int64
	if err := tx.QueryRowContext(ctx, `SELECT batch_id FROM api_import_jobs WHERE id = ? AND user_id = ? FOR UPDATE`, id, userID).Scan(&batchID); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE api_import_tasks t JOIN api_import_items i ON i.id = t.item_id
 SET t.status = IF(t.status = 'needs_review', 'reconcile_pending', 'pending'), t.error_code = ''
 WHERE i.job_id = ? AND t.status IN ('failed', 'needs_review')`, id)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor, action, target_type, target_id, detail_json) SELECT username, 'api.keys.retry', 'key_import_batch', ?, JSON_OBJECT('jobId', ?, 'tokenId', ?, 'tasks', ?) FROM admin_users WHERE id = ?`, batchID, id, tokenID, count, userID); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}

type APIImportTask struct {
	ID              int64
	JobID           string
	UserID          int64
	KeyID           int64
	TargetCode      string
	ChannelType     string
	TargetSignature string
	ReconcileOnly   bool
}

// Caller holds the shared worker advisory lock. An interrupted outbound call is
// reconciled without issuing another creation request.
func ClaimAPIImportTask(ctx context.Context, db *sql.DB) (*APIImportTask, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE api_import_tasks SET status = 'reconcile_pending', error_code = 'INTERRUPTED' WHERE status = 'running'`); err != nil {
		return nil, err
	}
	var task APIImportTask
	var status string
	err = tx.QueryRowContext(ctx, `SELECT t.id, i.job_id, j.user_id, COALESCE(i.api_key_id, 0), t.target_code, t.channel_type, t.target_signature, t.status
 FROM api_import_tasks t JOIN api_import_items i ON i.id = t.item_id JOIN api_import_jobs j ON j.id = i.job_id
 WHERE t.status IN ('pending', 'reconcile_pending') ORDER BY t.id LIMIT 1 FOR UPDATE`).Scan(&task.ID, &task.JobID, &task.UserID, &task.KeyID, &task.TargetCode, &task.ChannelType, &task.TargetSignature, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	task.ReconcileOnly = status == "reconcile_pending"
	if _, err := tx.ExecContext(ctx, `UPDATE api_import_tasks SET status = 'running', attempts = attempts + 1 WHERE id = ?`, task.ID); err != nil {
		return nil, err
	}
	return &task, tx.Commit()
}

func FinishAPIImportTask(ctx context.Context, db *sql.DB, id int64, status, code string) error {
	switch status {
	case "succeeded", "failed", "needs_review":
	default:
		return fmt.Errorf("invalid task status")
	}
	_, err := db.ExecContext(ctx, `UPDATE api_import_tasks SET status = ?, error_code = ? WHERE id = ? AND status = 'running'`, status, code, id)
	return err
}

func APIImportOwner(ctx context.Context, db *sql.DB, id int64) (string, error) {
	var username string
	err := db.QueryRowContext(ctx, `SELECT username FROM admin_users WHERE id = ? AND disabled = FALSE`, id).Scan(&username)
	return username, err
}
