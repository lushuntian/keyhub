ALTER TABLE aggregation_targets
  DROP INDEX uk_aggregation_targets_single_default;

ALTER TABLE aggregation_targets
  DROP COLUMN default_unique_slot;
