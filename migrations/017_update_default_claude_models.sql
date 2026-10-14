UPDATE category_pool_rules
SET default_models_json = JSON_ARRAY(
  'claude-opus-4-8',
  'claude-opus-4-7',
  'claude-sonnet-4-6',
  'claude-opus-4-6',
  'claude-fable-5',
  'claude-sonnet-5'
)
WHERE category_code IN ('aws_bedrock', 'anthropic');
