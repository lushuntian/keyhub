ALTER TABLE admin_users
  MODIFY COLUMN role ENUM('root','admin','account_manager','user') NOT NULL DEFAULT 'user',
  ADD COLUMN created_by_user_id BIGINT UNSIGNED NULL AFTER role,
  ADD INDEX idx_admin_users_created_by (created_by_user_id, id),
  ADD CONSTRAINT fk_admin_users_created_by
    FOREIGN KEY (created_by_user_id) REFERENCES admin_users(id)
    ON DELETE SET NULL;
