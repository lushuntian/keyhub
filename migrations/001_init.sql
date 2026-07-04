CREATE TABLE IF NOT EXISTS category_pool_rules (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  category_code VARCHAR(64) NOT NULL UNIQUE,
  category_label VARCHAR(128) NOT NULL,
  newapi_type INT NOT NULL,
  default_models_json JSON NOT NULL,
  keep_alive_target INT NOT NULL DEFAULT 0,
  refill_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_category_pool_rules_newapi_type (newapi_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS key_import_batches (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  category_code VARCHAR(64) NOT NULL,
  tag VARCHAR(128) NOT NULL DEFAULT '',
  group_name VARCHAR(64) NOT NULL DEFAULT 'default',
  total_count INT NOT NULL DEFAULT 0,
  success_count INT NOT NULL DEFAULT 0,
  failed_count INT NOT NULL DEFAULT 0,
  default_to_inventory BOOLEAN NOT NULL DEFAULT TRUE,
  note VARCHAR(512) NOT NULL DEFAULT '',
  created_by VARCHAR(128) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_key_import_batches_category (category_code),
  INDEX idx_key_import_batches_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS api_keys (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  import_batch_id BIGINT UNSIGNED NULL,
  category_code VARCHAR(64) NOT NULL,
  key_ciphertext TEXT NOT NULL,
  key_fingerprint CHAR(64) NOT NULL UNIQUE,
  key_hint VARCHAR(64) NOT NULL DEFAULT '',
  region VARCHAR(64) NOT NULL DEFAULT '',
  base_url VARCHAR(512) NOT NULL DEFAULT '',
  models_json JSON NOT NULL,
  tag VARCHAR(128) NOT NULL DEFAULT '',
  group_name VARCHAR(64) NOT NULL DEFAULT 'default',
  note VARCHAR(512) NOT NULL DEFAULT '',
  status ENUM('inventory','active','disabled','error','revoked') NOT NULL DEFAULT 'inventory',
  priority BIGINT NOT NULL DEFAULT 0,
  weight INT UNSIGNED NOT NULL DEFAULT 0,
  newapi_channel_id BIGINT NULL,
  last_sync_status VARCHAR(32) NOT NULL DEFAULT 'pending',
  last_health_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  success_count BIGINT NOT NULL DEFAULT 0,
  error_count BIGINT NOT NULL DEFAULT 0,
  last_error VARCHAR(1024) NOT NULL DEFAULT '',
  last_checked_at TIMESTAMP NULL,
  synced_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_api_keys_category_status (category_code, status),
  INDEX idx_api_keys_tag (tag),
  INDEX idx_api_keys_newapi_channel_id (newapi_channel_id),
  CONSTRAINT fk_api_keys_import_batch
    FOREIGN KEY (import_batch_id) REFERENCES key_import_batches(id)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS newapi_sync_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  api_key_id BIGINT UNSIGNED NULL,
  action VARCHAR(64) NOT NULL,
  status ENUM('pending','success','failed') NOT NULL DEFAULT 'pending',
  request_payload_json JSON NULL,
  response_json JSON NULL,
  error_message VARCHAR(1024) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_newapi_sync_events_key_created (api_key_id, created_at),
  CONSTRAINT fk_newapi_sync_events_api_key
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS health_checks (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  api_key_id BIGINT UNSIGNED NOT NULL,
  status ENUM('success','failed','skipped') NOT NULL,
  latency_ms INT NOT NULL DEFAULT 0,
  error_code VARCHAR(128) NOT NULL DEFAULT '',
  error_message VARCHAR(1024) NOT NULL DEFAULT '',
  checked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_health_checks_key_checked (api_key_id, checked_at),
  CONSTRAINT fk_health_checks_api_key
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS usage_daily_snapshots (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  stat_date DATE NOT NULL,
  api_key_id BIGINT UNSIGNED NULL,
  newapi_channel_id BIGINT NULL,
  quota_amount BIGINT NOT NULL DEFAULT 0,
  usd_amount DECIMAL(18,6) NOT NULL DEFAULT 0,
  request_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usage_daily_key (stat_date, api_key_id, newapi_channel_id),
  INDEX idx_usage_daily_stat_date (stat_date),
  CONSTRAINT fk_usage_daily_api_key
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  actor VARCHAR(128) NOT NULL DEFAULT '',
  action VARCHAR(128) NOT NULL,
  target_type VARCHAR(64) NOT NULL DEFAULT '',
  target_id BIGINT UNSIGNED NULL,
  detail_json JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_audit_logs_target (target_type, target_id),
  INDEX idx_audit_logs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO category_pool_rules
  (category_code, category_label, newapi_type, default_models_json, keep_alive_target, refill_enabled, sort_order)
VALUES
  ('aws_bedrock', 'AWS Bedrock', 33, JSON_ARRAY('claude-opus-4-6', 'claude-opus-4-7', 'claude-opus-4-8'), 0, FALSE, 10),
  ('anthropic', 'Anthropic 官方', 14, JSON_ARRAY('claude-opus-4-6', 'claude-opus-4-7', 'claude-opus-4-8'), 0, FALSE, 30),
  ('openai', 'OpenAI', 1, JSON_ARRAY('gpt-4.1', 'gpt-4o', 'o3'), 0, FALSE, 40),
  ('azure_openai', 'Azure OpenAI', 3, JSON_ARRAY('gpt-4.1', 'gpt-4o'), 0, FALSE, 50),
  ('google_ai_studio', 'Google AI Studio', 24, JSON_ARRAY('gemini-2.5-pro', 'gemini-2.5-flash'), 0, FALSE, 60)
ON DUPLICATE KEY UPDATE
  category_label = VALUES(category_label),
  newapi_type = VALUES(newapi_type),
  default_models_json = VALUES(default_models_json),
  sort_order = VALUES(sort_order);
