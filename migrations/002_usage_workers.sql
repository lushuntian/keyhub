CREATE TABLE IF NOT EXISTS usage_sync_cursors (
  newapi_channel_id BIGINT NOT NULL PRIMARY KEY,
  api_key_id BIGINT UNSIGNED NULL,
  last_quota_amount BIGINT NOT NULL DEFAULT 0,
  last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_usage_sync_cursors_api_key (api_key_id),
  CONSTRAINT fk_usage_sync_cursors_api_key
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS worker_runs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  worker_name VARCHAR(64) NOT NULL,
  status ENUM('success','failed') NOT NULL,
  started_at TIMESTAMP NOT NULL,
  finished_at TIMESTAMP NOT NULL,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  detail_json JSON NULL,
  error_message VARCHAR(1024) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_worker_runs_name_created (worker_name, created_at),
  INDEX idx_worker_runs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
