ALTER TABLE aggregation_targets
  ADD COLUMN reverse_group_name VARCHAR(64) NOT NULL DEFAULT 'default' AFTER reverse_username;
