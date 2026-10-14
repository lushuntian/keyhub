ALTER TABLE api_key_push_bindings
  ADD COLUMN remote_status SMALLINT NULL AFTER status,
  ADD COLUMN remote_status_checked_at TIMESTAMP NULL AFTER remote_status;
