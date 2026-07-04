ALTER TABLE admin_users
  MODIFY COLUMN role ENUM('root','admin','user') NOT NULL DEFAULT 'user';

UPDATE admin_users
SET role = 'user'
WHERE role = 'admin'
  AND username <> 'admin';
