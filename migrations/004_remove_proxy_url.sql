SET @keyhub_drop_proxy_url := (
  SELECT IF(
    COUNT(*) > 0,
    'ALTER TABLE api_keys DROP COLUMN proxy_url',
    'SELECT 1'
  )
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'api_keys'
    AND column_name = 'proxy_url'
);

PREPARE keyhub_drop_proxy_url_stmt FROM @keyhub_drop_proxy_url;
EXECUTE keyhub_drop_proxy_url_stmt;
DEALLOCATE PREPARE keyhub_drop_proxy_url_stmt;
