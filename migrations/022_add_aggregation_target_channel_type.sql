ALTER TABLE aggregation_targets
  ADD COLUMN channel_type VARCHAR(64) NOT NULL DEFAULT 'anthropic' AFTER connection_mode;
