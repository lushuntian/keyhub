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

type UsageDelta struct {
	APIKeyID        int64   `json:"apiKeyId"`
	TargetCode      string  `json:"targetCode"`
	CategoryCode    string  `json:"categoryCode"`
	KeyHint         string  `json:"keyHint"`
	NewAPIChannelID int64   `json:"newApiChannelId"`
	PreviousQuota   int64   `json:"previousQuota"`
	CurrentQuota    int64   `json:"currentQuota"`
	DeltaQuota      int64   `json:"deltaQuota"`
	DeltaUSD        float64 `json:"deltaUsd"`
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

func ListUsageSyncTargets(ctx context.Context, db *sql.DB) ([]UsageSyncTarget, error) {
	rows, err := db.QueryContext(ctx, `
SELECT k.id, b.target_code, k.category_code, k.key_hint, k.tag, b.remote_channel_id
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

func RecordUsageReadings(ctx context.Context, db *sql.DB, statDate time.Time, quotaPerUSD float64, readings []UsageReading) ([]UsageDelta, error) {
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

	deltas := make([]UsageDelta, 0, len(readings))
	for _, reading := range readings {
		delta, err := recordUsageReading(ctx, tx, statDate, quotaPerUSD, reading)
		if err != nil {
			return nil, err
		}
		deltas = append(deltas, delta)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit usage sync: %w", err)
	}
	return deltas, nil
}

func recordUsageReading(ctx context.Context, tx *sql.Tx, statDate time.Time, quotaPerUSD float64, reading UsageReading) (UsageDelta, error) {
	var previous sql.NullInt64
	err := tx.QueryRowContext(ctx, `
SELECT last_quota_amount
FROM usage_sync_cursors
WHERE target_code = ? AND newapi_channel_id = ?
FOR UPDATE`, reading.TargetCode, reading.NewAPIChannelID).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return UsageDelta{}, fmt.Errorf("load usage cursor for channel %d: %w", reading.NewAPIChannelID, err)
	}

	delta := UsageDelta{
		APIKeyID:        reading.APIKeyID,
		TargetCode:      reading.TargetCode,
		CategoryCode:    reading.CategoryCode,
		KeyHint:         reading.KeyHint,
		NewAPIChannelID: reading.NewAPIChannelID,
		CurrentQuota:    reading.CurrentQuota,
	}
	if !previous.Valid {
		delta.Baseline = true
	} else {
		delta.PreviousQuota = previous.Int64
		delta.DeltaQuota = reading.CurrentQuota - previous.Int64
		if delta.DeltaQuota < 0 {
			delta.ResetDetected = true
			delta.DeltaQuota = reading.CurrentQuota
		}
	}
	delta.DeltaUSD = float64(delta.DeltaQuota) / quotaPerUSD

	if delta.DeltaQuota > 0 {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO usage_daily_snapshots(stat_date, api_key_id, target_code, newapi_channel_id, quota_amount, usd_amount, request_count)
VALUES (?, ?, ?, ?, ?, ?, 0)
ON DUPLICATE KEY UPDATE
  quota_amount = quota_amount + VALUES(quota_amount),
  usd_amount = usd_amount + VALUES(usd_amount)`, statDate.Format("2006-01-02"), reading.APIKeyID, reading.TargetCode, reading.NewAPIChannelID, delta.DeltaQuota, delta.DeltaUSD); err != nil {
			return UsageDelta{}, fmt.Errorf("insert usage snapshot for channel %d: %w", reading.NewAPIChannelID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO usage_sync_cursors(newapi_channel_id, target_code, api_key_id, last_quota_amount)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  api_key_id = VALUES(api_key_id),
  last_quota_amount = VALUES(last_quota_amount),
  last_seen_at = CURRENT_TIMESTAMP`, reading.NewAPIChannelID, reading.TargetCode, reading.APIKeyID, reading.CurrentQuota); err != nil {
		return UsageDelta{}, fmt.Errorf("upsert usage cursor for channel %d: %w", reading.NewAPIChannelID, err)
	}
	return delta, nil
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
FROM usage_daily_snapshots
WHERE stat_date >= ?`
	totalArgs := []any{since}
	if owner != "" {
		totalSQL = `
SELECT COALESCE(SUM(u.quota_amount), 0), COALESCE(SUM(u.usd_amount), 0)
FROM usage_daily_snapshots u
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
FROM usage_daily_snapshots
WHERE stat_date >= ?
GROUP BY stat_date
ORDER BY stat_date ASC`
	args := []any{since}
	if owner != "" {
		query = `
SELECT u.stat_date, COALESCE(SUM(u.quota_amount), 0), COALESCE(SUM(u.usd_amount), 0)
FROM usage_daily_snapshots u
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
FROM usage_daily_snapshots u
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
FROM usage_daily_snapshots u
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
