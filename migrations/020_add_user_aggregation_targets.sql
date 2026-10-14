CREATE TABLE IF NOT EXISTS admin_user_aggregation_targets (
  user_id BIGINT UNSIGNED NOT NULL,
  target_code VARCHAR(64) NOT NULL,
  created_by VARCHAR(128) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, target_code),
  INDEX idx_admin_user_targets_target (target_code, user_id),
  CONSTRAINT fk_admin_user_targets_user
    FOREIGN KEY (user_id) REFERENCES admin_users(id)
    ON DELETE CASCADE,
  CONSTRAINT fk_admin_user_targets_target
    FOREIGN KEY (target_code) REFERENCES aggregation_targets(code)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE aggregation_targets
  DROP INDEX idx_aggregation_targets_enabled_sort,
  DROP COLUMN is_default,
  ADD INDEX idx_aggregation_targets_enabled_sort (enabled, sort_order, name, code);
