package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UsageSyncTarget struct {
	APIKeyID        int64
	TargetCode      string
	CategoryCode    string
	KeyHint         string
	Tag             string
	NewAPIChannelID int64
}

type UsageReading struct {
	APIKeyID        int64
	TargetCode      string
	CategoryCode    string
	KeyHint         string
	NewAPIChannelID int64
	CurrentQuota    int64
}

type RemoteChannelStatusReading struct {
	APIKeyID        int64
	TargetCode      string
	NewAPIChannelID int64
	RemoteStatus    *int
}

type UsageSyncRecord struct {
	APIKeyID        int64   `json:"apiKeyId"`
	TargetCode      string  `json:"targetCode"`
	CategoryCode    string  `json:"categoryCode"`
	KeyHint         string  `json:"keyHint"`
	NewAPIChannelID int64   `json:"newApiChannelId"`
	PreviousQuota   int64   `json:"previousQuota"`
	CurrentQuota    int64   `json:"currentQuota"`
	DeltaQuota      int64   `json:"deltaQuota"`
	DeltaUSD        float64 `json:"deltaUsd"`
	SyncedQuota     int64   `json:"syncedQuota"`
	SyncedUSD       float64 `json:"syncedUsd"`
	Baseline        bool    `json:"baseline"`
	ResetDetected   bool    `json:"resetDetected"`
}

type UsageSummary struct {
	Days       int                    `json:"days"`
	TotalQuota int64                  `json:"totalQuota"`
	TotalUSD   float64                `json:"totalUsd"`
	ByDay      []UsageDaySummary      `json:"byDay"`
	Categories []UsageCategorySummary `json:"categories"`
	Channels   []UsageChannelSummary  `json:"channels"`
}

type UsageDaySummary struct {
	StatDate string  `json:"statDate"`
	Quota    int64   `json:"quota"`
	USD      float64 `json:"usd"`
}

type UsageCategorySummary struct {
	CategoryCode  string  `json:"categoryCode"`
	CategoryLabel string  `json:"categoryLabel"`
	Quota         int64   `json:"quota"`
	USD           float64 `json:"usd"`
}

type UsageChannelSummary struct {
	APIKeyID        int64   `json:"apiKeyId"`
	TargetCode      string  `json:"targetCode"`
	NewAPIChannelID int64   `json:"newApiChannelId"`
	CategoryCode    string  `json:"categoryCode"`
	KeyHint         string  `json:"keyHint"`
	Tag             string  `json:"tag"`
	Quota           int64   `json:"quota"`
	USD             float64 `json:"usd"`
}

const latestUsageSnapshotsSQL = `
SELECT snapshots.*
FROM usage_daily_snapshots snapshots
JOIN (
  SELECT target_code, api_key_id, newapi_channel_id, MAX(stat_date) AS stat_date
  FROM usage_daily_snapshots
  WHERE api_key_id IS NOT NULL
  GROUP BY target_code, api_key_id, newapi_channel_id
) latest ON latest.target_code = snapshots.target_code
  AND latest.api_key_id = snapshots.api_key_id
  AND latest.newapi_channel_id = snapshots.newapi_channel_id
  AND latest.stat_date = snapshots.stat_date`

func ListUsageSyncTargets(ctx context.Context, db *sql.DB) ([]UsageSyncTarget, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  k.id,
  b.target_code,
  k.category_code,
  k.key_hint,
  k.tag,
  b.remote_channel_id
FROM api_key_push_bindings b
JOIN api_keys k ON k.id = b.api_key_id
WHERE b.remote_channel_id > 0 AND b.status = 'active' AND k.status = 'active'
ORDER BY b.target_code ASC, k.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list usage sync targets: %w", err)
	}
	defer rows.Close()

	targets := make([]UsageSyncTarget, 0)
	for rows.Next() {
		var target UsageSyncTarget
		if err := rows.Scan(&target.APIKeyID, &target.TargetCode, &target.CategoryCode, &target.KeyHint, &target.Tag, &target.NewAPIChannelID); err != nil {
			return nil, fmt.Errorf("scan usage sync target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage sync targets: %w", err)
	}
	return targets, nil
}

func RecordRemoteChannelStatuses(
	ctx context.Context,
	db *sql.DB,
	checkedAt time.Time,
	readings []RemoteChannelStatusReading,
) (int, error) {
	if len(readings) == 0 {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin remote channel status sync: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	updated := 0
	for _, reading := range readings {
		result, err := tx.ExecContext(ctx, `
UPDATE api_key_push_bindings
SET remote_status = ?, remote_status_checked_at = ?
WHERE api_key_id = ?
  AND target_code = ?
  AND remote_channel_id = ?
  AND status = 'active'`,
			reading.RemoteStatus,
			checkedAt,
			reading.APIKeyID,
			reading.TargetCode,
			reading.NewAPIChannelID,
		)
		if err != nil {
			return 0, fmt.Errorf("update remote channel status for key %d: %w", reading.APIKeyID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count remote channel status updates for key %d: %w", reading.APIKeyID, err)
		}
		updated += int(affected)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit remote channel status sync: %w", err)
	}
	committed = true
	return updated, nil
}

func RecordUsageReadings(ctx context.Context, db *sql.DB, statDate time.Time, quotaPerUSD float64, readings []UsageReading) ([]UsageSyncRecord, error) {
	if quotaPerUSD <= 0 {
		quotaPerUSD = 500000
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin usage sync: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	records := make([]UsageSyncRecord, 0, len(readings))
	for _, reading := range readings {
		record, err := recordUsageReading(ctx, tx, statDate, quotaPerUSD, reading)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit usage sync: %w", err)
	}
	return records, nil
}

func recordUsageReading(ctx context.Context, tx *sql.Tx, statDate time.Time, quotaPerUSD float64, reading UsageReading) (UsageSyncRecord, error) {
	var previous sql.NullInt64
	err := tx.QueryRowContext(ctx, `
SELECT last_quota_amount
FROM usage_sync_cursors
WHERE target_code = ? AND newapi_channel_id = ?
FOR UPDATE`, reading.TargetCode, reading.NewAPIChannelID).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return UsageSyncRecord{}, fmt.Errorf("load usage cursor for channel %d: %w", reading.NewAPIChannelID, err)
	}

	record := UsageSyncRecord{
		APIKeyID:        reading.APIKeyID,
		TargetCode:      reading.TargetCode,
		CategoryCode:    reading.CategoryCode,
		KeyHint:         reading.KeyHint,
		NewAPIChannelID: reading.NewAPIChannelID,
		CurrentQuota:    reading.CurrentQuota,
		SyncedQuota:     reading.CurrentQuota,
		SyncedUSD:       usageUSD(reading.CurrentQuota, quotaPerUSD),
	}
	if !previous.Valid {
		record.Baseline = true
	} else {
		record.PreviousQuota = previous.Int64
		if reading.CurrentQuota < previous.Int64 {
			record.ResetDetected = true
		}
	}
	record.DeltaQuota = incrementalUsageQuota(reading.CurrentQuota, previous)
	record.DeltaUSD = usageUSD(record.DeltaQuota, quotaPerUSD)

	hasPositiveUsage := reading.CurrentQuota > 0
	usageIncreased := hasPositiveUsage && (!previous.Valid || reading.CurrentQuota > previous.Int64)
	if hasPositiveUsage {
		if err := updateAPIKeyUsageTimes(ctx, tx, reading.APIKeyID, statDate, usageIncreased); err != nil {
			return UsageSyncRecord{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO usage_daily_snapshots(stat_date, api_key_id, target_code, newapi_channel_id, quota_amount, usd_amount, request_count)
VALUES (?, ?, ?, ?, ?, ?, 0)
ON DUPLICATE KEY UPDATE
  quota_amount = VALUES(quota_amount),
  usd_amount = VALUES(usd_amount),
  request_count = VALUES(request_count)`, statDate.Format("2006-01-02"), reading.APIKeyID, reading.TargetCode, reading.NewAPIChannelID, record.SyncedQuota, record.SyncedUSD); err != nil {
		return UsageSyncRecord{}, fmt.Errorf("insert usage snapshot for channel %d: %w", reading.NewAPIChannelID, err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO usage_sync_cursors(newapi_channel_id, target_code, api_key_id, last_quota_amount)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  api_key_id = VALUES(api_key_id),
  last_quota_amount = VALUES(last_quota_amount),
  last_seen_at = CURRENT_TIMESTAMP`, reading.NewAPIChannelID, reading.TargetCode, reading.APIKeyID, reading.CurrentQuota); err != nil {
		return UsageSyncRecord{}, fmt.Errorf("upsert usage cursor for channel %d: %w", reading.NewAPIChannelID, err)
	}
	return record, nil
}

func usageUSD(quota int64, quotaPerUSD float64) float64 {
	if quotaPerUSD <= 0 {
		quotaPerUSD = 500000
	}
	return float64(quota) / quotaPerUSD
}

func incrementalUsageQuota(current int64, previous sql.NullInt64) int64 {
	if !previous.Valid || current <= previous.Int64 {
		return 0
	}
	return current - previous.Int64
}

func updateAPIKeyUsageTimes(ctx context.Context, tx *sql.Tx, apiKeyID int64, usedAt time.Time, usageIncreased bool) error {
	if usageIncreased {
		if _, err := tx.ExecContext(ctx, `
UPDATE api_keys
SET first_used_at = COALESCE(first_used_at, ?),
    last_used_at = ?
WHERE id = ?`, usedAt, usedAt, apiKeyID); err != nil {
			return fmt.Errorf("update api key usage times for key %d: %w", apiKeyID, err)
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE api_keys
SET first_used_at = COALESCE(first_used_at, ?)
WHERE id = ?`, usedAt, apiKeyID); err != nil {
		return fmt.Errorf("update api key first usage time for key %d: %w", apiKeyID, err)
	}
	return nil
}

func LoadUsageSummary(ctx context.Context, db *sql.DB, days int, owner string) (UsageSummary, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	summary := UsageSummary{Days: days}
	since := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")
	owner = strings.TrimSpace(owner)

	totalSQL := `
SELECT COALESCE(SUM(quota_amount), 0), COALESCE(SUM(usd_amount), 0)
FROM (` + latestUsageSnapshotsSQL + `) u
WHERE stat_date >= ?`
	totalArgs := []any{since}
	if owner != "" {
		totalSQL = `
SELECT COALESCE(SUM(u.quota_amount), 0), COALESCE(SUM(u.usd_amount), 0)
FROM (` + latestUsageSnapshotsSQL + `) u
JOIN api_keys k ON k.id = u.api_key_id
WHERE u.stat_date >= ? AND k.created_by = ?`
		totalArgs = append(totalArgs, owner)
	}
	if err := db.QueryRowContext(ctx, totalSQL, totalArgs...).Scan(&summary.TotalQuota, &summary.TotalUSD); err != nil {
		return summary, fmt.Errorf("load usage summary total: %w", err)
	}

	byDay, err := loadUsageByDay(ctx, db, since, owner)
	if err != nil {
		return summary, err
	}
	summary.ByDay = byDay

	categories, err := loadUsageByCategory(ctx, db, since, owner)
	if err != nil {
		return summary, err
	}
	summary.Categories = categories

	channels, err := loadUsageByChannel(ctx, db, since, owner)
	if err != nil {
		return summary, err
	}
	summary.Channels = channels
	return summary, nil
}

func loadUsageByDay(ctx context.Context, db *sql.DB, since string, owner string) ([]UsageDaySummary, error) {
	query := `
SELECT stat_date, COALESCE(SUM(quota_amount), 0), COALESCE(SUM(usd_amount), 0)
FROM (` + latestUsageSnapshotsSQL + `) u
WHERE stat_date >= ?
GROUP BY stat_date
ORDER BY stat_date ASC`
	args := []any{since}
	if owner != "" {
		query = `
SELECT u.stat_date, COALESCE(SUM(u.quota_amount), 0), COALESCE(SUM(u.usd_amount), 0)
FROM (` + latestUsageSnapshotsSQL + `) u
JOIN api_keys k ON k.id = u.api_key_id
WHERE u.stat_date >= ? AND k.created_by = ?
GROUP BY u.stat_date
ORDER BY u.stat_date ASC`
		args = append(args, owner)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load usage by day: %w", err)
	}
	defer rows.Close()

	items := make([]UsageDaySummary, 0)
	for rows.Next() {
		var day time.Time
		var item UsageDaySummary
		if err := rows.Scan(&day, &item.Quota, &item.USD); err != nil {
			return nil, fmt.Errorf("scan usage by day: %w", err)
		}
		item.StatDate = day.Format("2006-01-02")
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage by day: %w", err)
	}
	return items, nil
}

func loadUsageByCategory(ctx context.Context, db *sql.DB, since string, owner string) ([]UsageCategorySummary, error) {
	ownerFilter := ""
	args := []any{since}
	if owner != "" {
		ownerFilter = " AND k.created_by = ?"
		args = append(args, owner)
	}
	rows, err := db.QueryContext(ctx, `
SELECT
  k.category_code,
  COALESCE(r.category_label, k.category_code),
  COALESCE(SUM(u.quota_amount), 0),
  COALESCE(SUM(u.usd_amount), 0)
FROM (`+latestUsageSnapshotsSQL+`) u
JOIN api_keys k ON k.id = u.api_key_id
LEFT JOIN category_pool_rules r ON r.category_code = k.category_code
WHERE u.stat_date >= ?`+ownerFilter+`
GROUP BY k.category_code, COALESCE(r.category_label, k.category_code)
ORDER BY COALESCE(SUM(u.usd_amount), 0) DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("load usage by category: %w", err)
	}
	defer rows.Close()

	items := make([]UsageCategorySummary, 0)
	for rows.Next() {
		var item UsageCategorySummary
		if err := rows.Scan(&item.CategoryCode, &item.CategoryLabel, &item.Quota, &item.USD); err != nil {
			return nil, fmt.Errorf("scan usage by category: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage by category: %w", err)
	}
	return items, nil
}

func loadUsageByChannel(ctx context.Context, db *sql.DB, since string, owner string) ([]UsageChannelSummary, error) {
	ownerFilter := ""
	args := []any{since}
	if owner != "" {
		ownerFilter = " AND k.created_by = ?"
		args = append(args, owner)
	}
	rows, err := db.QueryContext(ctx, `
SELECT
  k.id,
  u.target_code,
  u.newapi_channel_id,
  k.category_code,
  k.key_hint,
  k.tag,
  COALESCE(SUM(u.quota_amount), 0),
  COALESCE(SUM(u.usd_amount), 0)
FROM (`+latestUsageSnapshotsSQL+`) u
JOIN api_keys k ON k.id = u.api_key_id
WHERE u.stat_date >= ?`+ownerFilter+`
GROUP BY k.id, u.target_code, u.newapi_channel_id, k.category_code, k.key_hint, k.tag
ORDER BY COALESCE(SUM(u.usd_amount), 0) DESC
LIMIT 100`, args...)
	if err != nil {
		return nil, fmt.Errorf("load usage by channel: %w", err)
	}
	defer rows.Close()

	items := make([]UsageChannelSummary, 0)
	for rows.Next() {
		var item UsageChannelSummary
		if err := rows.Scan(&item.APIKeyID, &item.TargetCode, &item.NewAPIChannelID, &item.CategoryCode, &item.KeyHint, &item.Tag, &item.Quota, &item.USD); err != nil {
			return nil, fmt.Errorf("scan usage by channel: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage by channel: %w", err)
	}
	return items, nil
}
