ALTER TABLE aggregation_targets
  ADD COLUMN is_system_default BOOLEAN NOT NULL DEFAULT FALSE AFTER enabled,
  ADD COLUMN system_default_unique_slot TINYINT
    GENERATED ALWAYS AS (CASE WHEN is_system_default THEN 1 ELSE NULL END) STORED,
  ADD UNIQUE INDEX uk_aggregation_targets_single_system_default (system_default_unique_slot);
