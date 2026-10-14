ALTER TABLE aggregation_targets
  ADD COLUMN models_json JSON NULL AFTER channel_type;

UPDATE aggregation_targets t
JOIN category_pool_rules r ON r.category_code = t.channel_type
SET t.models_json = r.default_models_json
WHERE t.models_json IS NULL;

UPDATE aggregation_targets
SET models_json = JSON_ARRAY()
WHERE models_json IS NULL;

ALTER TABLE aggregation_targets
  MODIFY COLUMN models_json JSON NOT NULL;
