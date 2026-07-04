package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type DashboardSummary struct {
	TotalKeys     int64             `json:"totalKeys"`
	ActiveKeys    int64             `json:"activeKeys"`
	InventoryKeys int64             `json:"inventoryKeys"`
	DisabledKeys  int64             `json:"disabledKeys"`
	TotalUsageUSD float64           `json:"totalUsageUsd"`
	Categories    []CategorySummary `json:"categories"`
}

type CategorySummary struct {
	Code          string `json:"code"`
	Label         string `json:"label"`
	NewAPIType    int    `json:"newApiType"`
	TotalKeys     int64  `json:"totalKeys"`
	ActiveKeys    int64  `json:"activeKeys"`
	InventoryKeys int64  `json:"inventoryKeys"`
}

type ChannelGroup struct {
	CategoryCode  string  `json:"categoryCode"`
	Tag           string  `json:"tag"`
	KeyCount      int64   `json:"keyCount"`
	ActiveCount   int64   `json:"activeCount"`
	DisabledCount int64   `json:"disabledCount"`
	UsedUSD       float64 `json:"usedUsd"`
}

func LoadDashboardSummary(ctx context.Context, db *sql.DB, owner string) (DashboardSummary, error) {
	var summary DashboardSummary
	owner = strings.TrimSpace(owner)
	keyWhere := ""
	keyArgs := []any{}
	if owner != "" {
		keyWhere = "WHERE created_by = ?"
		keyArgs = append(keyArgs, owner)
	}

	err := db.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'inventory' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status IN ('disabled', 'error', 'revoked') THEN 1 ELSE 0 END), 0)
FROM api_keys `+keyWhere, keyArgs...).Scan(&summary.TotalKeys, &summary.ActiveKeys, &summary.InventoryKeys, &summary.DisabledKeys)
	if err != nil {
		return summary, fmt.Errorf("load dashboard key totals: %w", err)
	}

	usageSQL := "SELECT COALESCE(SUM(u.usd_amount), 0) FROM usage_daily_snapshots u"
	usageArgs := []any{}
	if owner != "" {
		usageSQL += " JOIN api_keys k ON k.id = u.api_key_id WHERE k.created_by = ?"
		usageArgs = append(usageArgs, owner)
	}
	if err := db.QueryRowContext(ctx, usageSQL, usageArgs...).Scan(&summary.TotalUsageUSD); err != nil {
		return summary, fmt.Errorf("load usage total: %w", err)
	}

	joinOwnerFilter := ""
	categoryArgs := []any{}
	if owner != "" {
		joinOwnerFilter = " AND k.created_by = ?"
		categoryArgs = append(categoryArgs, owner)
	}
	rows, err := db.QueryContext(ctx, `
SELECT
  r.category_code,
  r.category_label,
  r.newapi_type,
  COALESCE(COUNT(k.id), 0) AS total_keys,
  COALESCE(SUM(CASE WHEN k.status = 'active' THEN 1 ELSE 0 END), 0) AS active_keys,
  COALESCE(SUM(CASE WHEN k.status = 'inventory' THEN 1 ELSE 0 END), 0) AS inventory_keys
FROM category_pool_rules r
LEFT JOIN api_keys k ON k.category_code = r.category_code`+joinOwnerFilter+`
GROUP BY r.category_code, r.category_label, r.newapi_type
ORDER BY r.sort_order, r.category_label`, categoryArgs...)
	if err != nil {
		return summary, fmt.Errorf("load category summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item CategorySummary
		if err := rows.Scan(&item.Code, &item.Label, &item.NewAPIType, &item.TotalKeys, &item.ActiveKeys, &item.InventoryKeys); err != nil {
			return summary, fmt.Errorf("scan category summary: %w", err)
		}
		summary.Categories = append(summary.Categories, item)
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("iterate category summary: %w", err)
	}

	return summary, nil
}

func LoadChannelGroups(ctx context.Context, db *sql.DB, owner string) ([]ChannelGroup, error) {
	owner = strings.TrimSpace(owner)
	where := ""
	args := []any{}
	if owner != "" {
		where = "WHERE k.created_by = ?"
		args = append(args, owner)
	}
	rows, err := db.QueryContext(ctx, `
SELECT
  k.category_code,
  COALESCE(NULLIF(k.tag, ''), '未标记') AS tag,
  COUNT(*) AS key_count,
  COALESCE(SUM(CASE WHEN k.status = 'active' THEN 1 ELSE 0 END), 0) AS active_count,
  COALESCE(SUM(CASE WHEN k.status IN ('disabled', 'error', 'revoked') THEN 1 ELSE 0 END), 0) AS disabled_count,
  COALESCE(SUM(u.usd_amount), 0) AS used_usd
FROM api_keys k
LEFT JOIN usage_daily_snapshots u ON u.api_key_id = k.id
`+where+`
GROUP BY k.category_code, COALESCE(NULLIF(k.tag, ''), '未标记')
ORDER BY k.category_code, tag`, args...)
	if err != nil {
		return nil, fmt.Errorf("load channel groups: %w", err)
	}
	defer rows.Close()

	groups := make([]ChannelGroup, 0)
	for rows.Next() {
		var group ChannelGroup
		if err := rows.Scan(&group.CategoryCode, &group.Tag, &group.KeyCount, &group.ActiveCount, &group.DisabledCount, &group.UsedUSD); err != nil {
			return nil, fmt.Errorf("scan channel group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channel groups: %w", err)
	}

	return groups, nil
}
