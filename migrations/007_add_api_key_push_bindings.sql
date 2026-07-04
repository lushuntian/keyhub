CREATE TABLE IF NOT EXISTS api_key_push_bindings (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  api_key_id BIGINT UNSIGNED NOT NULL,
  target_code VARCHAR(64) NOT NULL,
  remote_channel_id BIGINT NOT NULL,
  status ENUM('active','disabled','error') NOT NULL DEFAULT 'active',
  last_sync_status VARCHAR(32) NOT NULL DEFAULT 'success',
  last_error VARCHAR(1024) NOT NULL DEFAULT '',
  synced_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_api_key_push_target (api_key_id, target_code),
  INDEX idx_api_key_push_target (target_code),
  INDEX idx_api_key_push_remote_channel (remote_channel_id),
  CONSTRAINT fk_api_key_push_bindings_api_key
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
