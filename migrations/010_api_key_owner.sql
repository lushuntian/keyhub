ALTER TABLE api_keys
  ADD COLUMN created_by VARCHAR(128) NOT NULL DEFAULT '' AFTER note,
  ADD INDEX idx_api_keys_created_by (created_by);

UPDATE api_keys k
JOIN key_import_batches b ON b.id = k.import_batch_id
SET k.created_by = b.created_by
WHERE k.created_by = '';
