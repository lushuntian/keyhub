package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type UsageLogChannelMapping struct {
	LocalChannelID  int64
	RemoteChannelID int64
}

func ListUsageLogChannelMappings(
	ctx context.Context,
	db *sql.DB,
	targetCode string,
	owners []string,
) ([]UsageLogChannelMapping, error) {
	targetCode = strings.TrimSpace(targetCode)
	query := `
SELECT k.id, b.remote_channel_id
FROM api_key_push_bindings b
JOIN api_keys k ON k.id = b.api_key_id
WHERE b.target_code = ? AND b.remote_channel_id > 0`
	args := []any{targetCode}
	if owners != nil {
		if len(owners) == 0 {
			return []UsageLogChannelMapping{}, nil
		}
		placeholders := make([]string, 0, len(owners))
		for _, owner := range owners {
			placeholders = append(placeholders, "?")
			args = append(args, strings.TrimSpace(owner))
		}
		query += `
  AND k.created_by IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += `
ORDER BY k.id ASC`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list usage log channel mappings: %w", err)
	}
	defer rows.Close()

	items := make([]UsageLogChannelMapping, 0)
	localByRemote := make(map[int64]int64)
	for rows.Next() {
		var item UsageLogChannelMapping
		if err := rows.Scan(&item.LocalChannelID, &item.RemoteChannelID); err != nil {
			return nil, fmt.Errorf("scan usage log channel mapping: %w", err)
		}
		if existing, ok := localByRemote[item.RemoteChannelID]; ok && existing != item.LocalChannelID {
			return nil, fmt.Errorf(
				"ambiguous usage log channel mapping for target %s remote channel %d",
				targetCode,
				item.RemoteChannelID,
			)
		}
		localByRemote[item.RemoteChannelID] = item.LocalChannelID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage log channel mappings: %w", err)
	}
	return items, nil
}
