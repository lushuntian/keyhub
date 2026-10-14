ALTER TABLE api_keys
  ADD COLUMN key_type VARCHAR(32) NOT NULL DEFAULT 'long_term' AFTER category_code,
  ADD INDEX idx_api_keys_key_type (key_type);
