ALTER TABLE usage_daily_snapshots
  ADD COLUMN target_code VARCHAR(64) NOT NULL DEFAULT 'default' AFTER newapi_channel_id;

ALTER TABLE usage_daily_snapshots
  DROP INDEX uk_usage_daily_key;

ALTER TABLE usage_daily_snapshots
  ADD UNIQUE KEY uk_usage_daily_key (stat_date, target_code, api_key_id, newapi_channel_id);

ALTER TABLE usage_daily_snapshots
  ADD INDEX idx_usage_daily_target (target_code);

ALTER TABLE usage_sync_cursors
  ADD COLUMN target_code VARCHAR(64) NOT NULL DEFAULT 'default' AFTER newapi_channel_id;

ALTER TABLE usage_sync_cursors
  DROP PRIMARY KEY;

ALTER TABLE usage_sync_cursors
  ADD PRIMARY KEY (target_code, newapi_channel_id);

ALTER TABLE usage_sync_cursors
  ADD INDEX idx_usage_sync_cursors_target (target_code);
