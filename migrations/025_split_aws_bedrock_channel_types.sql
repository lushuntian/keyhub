UPDATE category_pool_rules
SET category_label = '旧版 AWS Bedrock',
    newapi_type = 33
WHERE category_code = 'aws_bedrock';

INSERT INTO category_pool_rules
  (category_code, category_label, newapi_type, default_models_json, keep_alive_target, refill_enabled, sort_order)
VALUES
  (
    'aws_bedrock_new',
    '新版 AWS Bedrock',
    14,
    JSON_ARRAY(
      'claude-opus-4-8',
      'claude-opus-4-7',
      'claude-sonnet-4-6',
      'claude-opus-4-6',
      'claude-fable-5',
      'claude-sonnet-5'
    ),
    0,
    FALSE,
    20
  )
ON DUPLICATE KEY UPDATE
  category_label = VALUES(category_label),
  newapi_type = VALUES(newapi_type),
  default_models_json = VALUES(default_models_json),
  sort_order = VALUES(sort_order);
