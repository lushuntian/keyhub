SET @keyhub_target_columns = (
  SELECT GROUP_CONCAT(operation ORDER BY sort_order, operation SEPARATOR ', ')
  FROM (
    SELECT 0 AS sort_order, CONCAT('DROP CHECK `', REPLACE(CONSTRAINT_NAME, '`', '``'), '`') AS operation
    FROM information_schema.TABLE_CONSTRAINTS
    WHERE CONSTRAINT_SCHEMA = DATABASE()
      AND TABLE_NAME = 'admin_user_aggregation_targets'
      AND CONSTRAINT_TYPE = 'CHECK'
    UNION ALL
    SELECT 1 AS sort_order, CONCAT('DROP COLUMN `', REPLACE(COLUMN_NAME, '`', '``'), '`') AS operation
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'admin_user_aggregation_targets'
      AND COLUMN_NAME NOT IN ('user_id', 'target_code', 'created_by', 'created_at')
  ) operations
);

SET @keyhub_target_sql = IF(
  @keyhub_target_columns IS NULL,
  'DO 0',
  CONCAT('ALTER TABLE admin_user_aggregation_targets ', @keyhub_target_columns)
);

PREPARE keyhub_target_statement FROM @keyhub_target_sql;
EXECUTE keyhub_target_statement;
DEALLOCATE PREPARE keyhub_target_statement;

UPDATE usage_daily_snapshots
SET usd_amount = ROUND(quota_amount / @keyhub_quota_per_usd, 6)
WHERE usd_amount <> ROUND(quota_amount / @keyhub_quota_per_usd, 6);
