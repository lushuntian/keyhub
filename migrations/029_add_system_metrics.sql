CREATE TABLE IF NOT EXISTS system_metrics (
  metric_key VARCHAR(64) NOT NULL PRIMARY KEY,
  metric_value DECIMAL(20,8) NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO system_metrics(metric_key, metric_value)
VALUES ('estimated_usd_per_minute', 100)
ON DUPLICATE KEY UPDATE metric_value = GREATEST(metric_value, 100);
