package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAdminUserNotFound           = errors.New("admin user not found")
	ErrInheritedAggregationTargets = errors.New("inherited aggregation targets must be configured on the account manager")
)

type AdminUserAggregationTarget struct {
	TargetCode string `json:"targetCode"`
}

func ListUserAggregationTargets(ctx context.Context, db *sql.DB, username string) ([]AggregationTargetRecord, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  t.code,
  t.name,
  t.base_url,
  COALESCE(NULLIF(t.connection_mode, ''), 'api'),
  COALESCE(NULLIF(t.channel_type, ''), 'anthropic'),
  t.models_json,
  t.enabled,
  t.token_ciphertext <> '',
  t.reverse_username,
  COALESCE(NULLIF(t.reverse_group_name, ''), 'default'),
  COALESCE(t.reverse_password_ciphertext, '') <> '',
  t.created_at,
  t.updated_at
FROM admin_users u
JOIN admin_user_aggregation_targets ut ON ut.user_id = COALESCE(u.created_by_user_id, u.id)
JOIN aggregation_targets t ON t.code = ut.target_code
WHERE u.username = ? AND u.disabled = FALSE AND t.enabled = TRUE
ORDER BY t.sort_order ASC, t.name ASC, t.code ASC`, strings.TrimSpace(username))
	if err != nil {
		return nil, fmt.Errorf("list user aggregation targets: %w", err)
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
			&hasToken,
			&item.ReverseUsername,
			&item.ReverseGroupName,
			&hasReversePassword,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user aggregation target: %w", err)
		}
		item.HasToken = hasToken > 0
		item.HasReversePassword = hasReversePassword > 0
		item.Source = "database"
		if err := decodeAggregationTargetModels(modelsJSON, &item); err != nil {
			return nil, fmt.Errorf("decode user aggregation target %s models: %w", item.Code, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user aggregation targets: %w", err)
	}
	return items, nil
}

func UserCanUseAggregationTarget(ctx context.Context, db *sql.DB, username string, targetCode string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM admin_users u
JOIN admin_user_aggregation_targets ut ON ut.user_id = COALESCE(u.created_by_user_id, u.id)
JOIN aggregation_targets t ON t.code = ut.target_code
WHERE u.username = ? AND ut.target_code = ? AND u.disabled = FALSE AND t.enabled = TRUE`,
		strings.TrimSpace(username), strings.TrimSpace(targetCode),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check user aggregation target: %w", err)
	}
	return count > 0, nil
}

func ListAdminUserAggregationTargets(ctx context.Context, db *sql.DB) (map[int64][]AdminUserAggregationTarget, error) {
	rows, err := db.QueryContext(ctx, `
SELECT user_id, target_code
FROM admin_user_aggregation_targets
ORDER BY user_id ASC, target_code ASC`)
	if err != nil {
		return nil, fmt.Errorf("list admin user aggregation targets: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]AdminUserAggregationTarget)
	for rows.Next() {
		var userID int64
		var target AdminUserAggregationTarget
		if err := rows.Scan(&userID, &target.TargetCode); err != nil {
			return nil, fmt.Errorf("scan admin user aggregation target: %w", err)
		}
		result[userID] = append(result[userID], target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin user aggregation targets: %w", err)
	}
	return result, nil
}

func ReplaceAdminUserAggregationTargets(ctx context.Context, db *sql.DB, userID int64, targets []AdminUserAggregationTarget, actor string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace admin user aggregation targets: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var createdByUserID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT created_by_user_id FROM admin_users WHERE id = ?`, userID).Scan(&createdByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAdminUserNotFound
		}
		return fmt.Errorf("check admin user: %w", err)
	}
	if createdByUserID.Valid {
		return ErrInheritedAggregationTargets
	}

	cleanedTargets := make([]AdminUserAggregationTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		target.TargetCode = strings.TrimSpace(target.TargetCode)
		if target.TargetCode == "" {
			continue
		}
		if _, exists := seen[target.TargetCode]; exists {
			continue
		}
		seen[target.TargetCode] = struct{}{}
		cleanedTargets = append(cleanedTargets, target)
	}

	for _, target := range cleanedTargets {
		var targetExists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM aggregation_targets WHERE code = ?`, target.TargetCode).Scan(&targetExists); err != nil {
			return fmt.Errorf("check aggregation target %s: %w", target.TargetCode, err)
		}
		if targetExists == 0 {
			return fmt.Errorf("%w: %s", ErrAggregationTargetNotFound, target.TargetCode)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_user_aggregation_targets WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear admin user aggregation targets: %w", err)
	}
	for _, target := range cleanedTargets {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO admin_user_aggregation_targets(user_id, target_code, created_by)
VALUES (?, ?, ?)`, userID, target.TargetCode, strings.TrimSpace(actor)); err != nil {
			return fmt.Errorf("add admin user aggregation target %s: %w", target.TargetCode, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit admin user aggregation targets: %w", err)
	}
	return nil
}
