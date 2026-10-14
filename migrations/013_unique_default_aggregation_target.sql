UPDATE aggregation_targets
SET is_default = FALSE
WHERE enabled = FALSE;

UPDATE aggregation_targets
SET is_default = FALSE
WHERE is_default = TRUE
  AND code <> (
    SELECT keep_code
    FROM (
      SELECT code AS keep_code
      FROM aggregation_targets
      WHERE enabled = TRUE AND is_default = TRUE
      ORDER BY sort_order ASC, name ASC, code ASC
      LIMIT 1
    ) AS keep_default
  );

ALTER TABLE aggregation_targets
  ADD COLUMN default_unique_slot TINYINT
    GENERATED ALWAYS AS (CASE WHEN is_default THEN 1 ELSE NULL END) STORED;

ALTER TABLE aggregation_targets
  ADD UNIQUE KEY uk_aggregation_targets_single_default (default_unique_slot);
