ALTER TABLE api_keys
  DROP INDEX idx_api_keys_key_type,
  DROP COLUMN key_type;

ALTER TABLE aggregation_targets
  DROP COLUMN account_type_restriction;
