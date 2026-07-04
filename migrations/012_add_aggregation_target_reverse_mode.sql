ALTER TABLE aggregation_targets
  ADD COLUMN connection_mode VARCHAR(32) NOT NULL DEFAULT 'api' AFTER base_url;

ALTER TABLE aggregation_targets
  ADD COLUMN reverse_username VARCHAR(255) NOT NULL DEFAULT '' AFTER token_ciphertext;

ALTER TABLE aggregation_targets
  ADD COLUMN reverse_password_ciphertext TEXT NULL AFTER reverse_username;
