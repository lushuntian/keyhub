ALTER TABLE aggregation_targets
  ADD COLUMN account_type_restriction VARCHAR(32) NOT NULL DEFAULT 'none' AFTER is_default;
