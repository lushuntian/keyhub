ALTER TABLE api_keys
  ADD COLUMN aws_credential_type VARCHAR(16) NOT NULL DEFAULT '' AFTER category_code;
