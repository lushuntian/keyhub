ALTER TABLE api_keys
  ADD COLUMN first_used_at TIMESTAMP NULL AFTER synced_at,
  ADD COLUMN last_used_at TIMESTAMP NULL AFTER first_used_at,
  ADD INDEX idx_api_keys_first_used_at (first_used_at),
  ADD INDEX idx_api_keys_last_used_at (last_used_at);

UPDATE api_keys k
JOIN (
  SELECT api_key_id, MIN(created_at) AS first_used_at
  FROM usage_daily_snapshots
  WHERE api_key_id IS NOT NULL
    AND quota_amount > 0
  GROUP BY api_key_id
) usage_times ON usage_times.api_key_id = k.id
SET k.first_used_at = usage_times.first_used_at
WHERE k.first_used_at IS NULL;

UPDATE api_keys k
JOIN (
  SELECT api_key_id, MAX(observed_at) AS last_used_at
  FROM (
    SELECT
      api_key_id,
      quota_amount,
      created_at AS observed_at,
      COALESCE(
        LAG(quota_amount) OVER (
          PARTITION BY target_code, api_key_id, newapi_channel_id
          ORDER BY stat_date, id
        ),
        0
      ) AS previous_quota
    FROM usage_daily_snapshots
    WHERE api_key_id IS NOT NULL
  ) history
  WHERE quota_amount > previous_quota
  GROUP BY api_key_id
) usage_times ON usage_times.api_key_id = k.id
SET k.last_used_at = usage_times.last_used_at
WHERE k.last_used_at IS NULL;
