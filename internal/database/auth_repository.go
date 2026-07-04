package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"keyhub/internal/security"
)

type AdminUser struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"`
	Disabled    bool       `json:"disabled"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type LoginAuditTarget struct {
	UserID int64
}

var ErrAdminUsernameExists = errors.New("admin username already exists")

func CountAdminUsers(ctx context.Context, db *sql.DB) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		return 0, fmt.Errorf("count admin users: %w", err)
	}
	return count, nil
}

func EnsureBootstrapAdmin(ctx context.Context, db *sql.DB, username string, password string) (bool, error) {
	count, err := CountAdminUsers(ctx, db)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	if strings.TrimSpace(password) == "" {
		return false, fmt.Errorf("no admin users exist; set KEYHUB_BOOTSTRAP_ADMIN_PASSWORD to create the first admin")
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return false, err
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO admin_users(username, password_hash, display_name, role)
VALUES (?, ?, ?, 'root')`, username, hash, username); err != nil {
		return false, fmt.Errorf("create bootstrap admin: %w", err)
	}
	return true, nil
}

func CreateAdminUser(ctx context.Context, db *sql.DB, username string, password string, displayName string) (*AdminUser, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create admin user: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var count int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		return nil, fmt.Errorf("count admin users: %w", err)
	}
	role := "user"
	if count == 0 {
		role = "root"
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO admin_users(username, password_hash, display_name, role)
VALUES (?, ?, ?, ?)`, username, hash, displayName, role)
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrAdminUsernameExists
		}
		return nil, fmt.Errorf("create admin user: %w", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("load created admin id: %w", err)
	}

	user, err := loadAdminUserInTx(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create admin user: %w", err)
	}
	return user, nil
}

func AuthenticateAdmin(ctx context.Context, db *sql.DB, username string, password string) (*AdminUser, error) {
	var user AdminUser
	var passwordHash string
	var lastLogin sql.NullTime
	err := db.QueryRowContext(ctx, `
SELECT id, username, password_hash, display_name, role, disabled, last_login_at, created_at
FROM admin_users
WHERE username = ?`, strings.TrimSpace(username)).Scan(
		&user.ID,
		&user.Username,
		&passwordHash,
		&user.DisplayName,
		&user.Role,
		&user.Disabled,
		&lastLogin,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid username or password")
	}
	if err != nil {
		return nil, fmt.Errorf("load admin user: %w", err)
	}
	if user.Disabled {
		return nil, fmt.Errorf("admin user is disabled")
	}
	if !security.VerifyPassword(password, passwordHash) {
		return nil, fmt.Errorf("invalid username or password")
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func MarkAdminLogin(ctx context.Context, db *sql.DB, userID int64) error {
	if _, err := db.ExecContext(ctx, `
UPDATE admin_users
SET last_login_at = CURRENT_TIMESTAMP
WHERE id = ?`, userID); err != nil {
		return fmt.Errorf("mark admin login: %w", err)
	}
	return nil
}

func CreateAdminSession(ctx context.Context, db *sql.DB, tokenHash string, userID int64, expiresAt time.Time, userAgent string, clientIP string) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE expires_at < CURRENT_TIMESTAMP"); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO admin_sessions(token_hash, user_id, expires_at, user_agent, client_ip)
VALUES (?, ?, ?, ?, ?)`, tokenHash, userID, expiresAt, truncate(userAgent, 255), truncate(clientIP, 64)); err != nil {
		return fmt.Errorf("create admin session: %w", err)
	}
	return nil
}

func LoadAdminSession(ctx context.Context, db *sql.DB, tokenHash string) (*AdminUser, error) {
	var user AdminUser
	var lastLogin sql.NullTime
	err := db.QueryRowContext(ctx, `
SELECT u.id, u.username, u.display_name, u.role, u.disabled, u.last_login_at, u.created_at
FROM admin_sessions s
JOIN admin_users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > CURRENT_TIMESTAMP AND u.disabled = FALSE`, tokenHash).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.Role,
		&user.Disabled,
		&lastLogin,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load admin session: %w", err)
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	_, _ = db.ExecContext(ctx, "UPDATE admin_sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE token_hash = ?", tokenHash)
	return &user, nil
}

func DeleteAdminSession(ctx context.Context, db *sql.DB, tokenHash string) error {
	if tokenHash == "" {
		return nil
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE token_hash = ?", tokenHash); err != nil {
		return fmt.Errorf("delete admin session: %w", err)
	}
	return nil
}

func loadAdminUserInTx(ctx context.Context, tx *sql.Tx, userID int64) (*AdminUser, error) {
	var user AdminUser
	var lastLogin sql.NullTime
	err := tx.QueryRowContext(ctx, `
SELECT id, username, display_name, role, disabled, last_login_at, created_at
FROM admin_users
WHERE id = ?`, userID).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.Role,
		&user.Disabled,
		&lastLogin,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load created admin user: %w", err)
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func isDuplicateKeyError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "Duplicate entry") || strings.Contains(message, "Error 1062")
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}
