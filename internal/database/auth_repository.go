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
	ID                     int64                        `json:"id"`
	Username               string                       `json:"username"`
	DisplayName            string                       `json:"displayName"`
	Role                   string                       `json:"role"`
	CreatedByUserID        *int64                       `json:"createdByUserId,omitempty"`
	CreatedByUsername      string                       `json:"createdByUsername,omitempty"`
	InheritsTargets        bool                         `json:"inheritsAggregationTargets"`
	Disabled               bool                         `json:"disabled"`
	AggregationTargetCodes []string                     `json:"aggregationTargetCodes"`
	AggregationTargets     []AdminUserAggregationTarget `json:"aggregationTargets"`
	UploadedKeyCount       int64                        `json:"uploadedKeyCount"`
	TotalUsageUSD          float64                      `json:"totalUsageUsd"`
	LastLoginAt            *time.Time                   `json:"lastLoginAt,omitempty"`
	CreatedAt              time.Time                    `json:"createdAt"`
}

type LoginAuditTarget struct {
	UserID int64
}

const AdminUserRoleAccountManager = "account_manager"

var (
	ErrAdminUsernameExists    = errors.New("admin username already exists")
	ErrInvalidAdminUserRole   = errors.New("admin user role must be user or account_manager")
	ErrProtectedAdminUserRole = errors.New("root and admin roles cannot be changed")
	ErrNestedAccountManager   = errors.New("a user created by an account manager cannot become an account manager")
	ErrInvalidAccountManager  = errors.New("creator must be an enabled account manager")
	ErrAdminUserNotManaged    = errors.New("admin user is not managed by the current account manager")
)

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
	return createAdminUser(ctx, db, username, password, displayName, nil)
}

func CreateAdminUserForAccountManager(ctx context.Context, db *sql.DB, username string, password string, displayName string, managerUserID int64) (*AdminUser, error) {
	return createAdminUser(ctx, db, username, password, displayName, &managerUserID)
}

func createAdminUser(ctx context.Context, db *sql.DB, username string, password string, displayName string, creatorUserID *int64) (*AdminUser, error) {
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
	if creatorUserID != nil {
		var creatorRole string
		var creatorDisabled bool
		if err := tx.QueryRowContext(ctx, `
SELECT role, disabled
FROM admin_users
WHERE id = ?
FOR UPDATE`, *creatorUserID).Scan(&creatorRole, &creatorDisabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrInvalidAccountManager
			}
			return nil, fmt.Errorf("load account manager: %w", err)
		}
		if creatorRole != AdminUserRoleAccountManager || creatorDisabled {
			return nil, ErrInvalidAccountManager
		}
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO admin_users(username, password_hash, display_name, role, created_by_user_id)
VALUES (?, ?, ?, ?, ?)`, username, hash, displayName, role, creatorUserID)
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

func ListAdminUsers(ctx context.Context, db *sql.DB) ([]AdminUser, error) {
	return ListAdminUsersForAccountManager(ctx, db, nil)
}

func ListAdminUsersForAccountManager(ctx context.Context, db *sql.DB, managerUserID *int64) ([]AdminUser, error) {
	where := ""
	args := []any{}
	if managerUserID != nil {
		where = "WHERE u.created_by_user_id = ?"
		args = append(args, *managerUserID)
	}
	rows, err := db.QueryContext(ctx, `
SELECT
  u.id,
  u.username,
  u.display_name,
  u.role,
  u.created_by_user_id,
  COALESCE(creator.username, ''),
  u.disabled,
  COALESCE(key_totals.uploaded_key_count, 0),
  COALESCE(usage_totals.total_usage_usd, 0),
  u.last_login_at,
  u.created_at
FROM admin_users u
LEFT JOIN admin_users creator ON creator.id = u.created_by_user_id
LEFT JOIN (
  SELECT created_by, COUNT(*) AS uploaded_key_count
  FROM api_keys
  GROUP BY created_by
) key_totals ON key_totals.created_by = u.username
LEFT JOIN (
  SELECT k.created_by, COALESCE(SUM(latest_usage.usd_amount), 0) AS total_usage_usd
  FROM (`+latestUsageSnapshotsSQL+`) latest_usage
  JOIN api_keys k ON k.id = latest_usage.api_key_id
  GROUP BY k.created_by
) usage_totals ON usage_totals.created_by = u.username
`+where+`
ORDER BY u.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin users: %w", err)
	}
	defer rows.Close()

	items := make([]AdminUser, 0)
	for rows.Next() {
		var item AdminUser
		var lastLogin sql.NullTime
		var createdByUserID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.Username,
			&item.DisplayName,
			&item.Role,
			&createdByUserID,
			&item.CreatedByUsername,
			&item.Disabled,
			&item.UploadedKeyCount,
			&item.TotalUsageUSD,
			&lastLogin,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan admin user: %w", err)
		}
		if lastLogin.Valid {
			item.LastLoginAt = &lastLogin.Time
		}
		if createdByUserID.Valid {
			item.CreatedByUserID = &createdByUserID.Int64
			item.InheritsTargets = true
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin users: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close admin users: %w", err)
	}
	targetsByUserID, err := ListAdminUserAggregationTargets(ctx, db)
	if err != nil {
		return nil, err
	}
	for index := range items {
		targetOwnerID := items[index].ID
		if items[index].CreatedByUserID != nil {
			targetOwnerID = *items[index].CreatedByUserID
		}
		items[index].AggregationTargets = append([]AdminUserAggregationTarget{}, targetsByUserID[targetOwnerID]...)
		items[index].AggregationTargetCodes = make([]string, 0, len(items[index].AggregationTargets))
		for _, target := range items[index].AggregationTargets {
			items[index].AggregationTargetCodes = append(items[index].AggregationTargetCodes, target.TargetCode)
		}
	}
	return items, nil
}

func ListAccountManagerOwnerUsernames(ctx context.Context, db *sql.DB, managerUserID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT username
FROM admin_users
WHERE id = ? OR created_by_user_id = ?
ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, id ASC`, managerUserID, managerUserID, managerUserID)
	if err != nil {
		return nil, fmt.Errorf("list account manager owner usernames: %w", err)
	}
	defer rows.Close()

	usernames := make([]string, 0)
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, fmt.Errorf("scan account manager owner username: %w", err)
		}
		usernames = append(usernames, username)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account manager owner usernames: %w", err)
	}
	return usernames, nil
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

func SetAdminUserDisabled(ctx context.Context, db *sql.DB, userID int64, disabled bool) error {
	return setAdminUserDisabled(ctx, db, userID, nil, disabled)
}

func SetManagedAdminUserDisabled(ctx context.Context, db *sql.DB, userID int64, managerUserID int64, disabled bool) error {
	return setAdminUserDisabled(ctx, db, userID, &managerUserID, disabled)
}

func setAdminUserDisabled(ctx context.Context, db *sql.DB, userID int64, managerUserID *int64, disabled bool) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update admin user status: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int64
	query := "SELECT id FROM admin_users WHERE id = ?"
	args := []any{userID}
	if managerUserID != nil {
		query += " AND created_by_user_id = ?"
		args = append(args, *managerUserID)
	}
	query += " FOR UPDATE"
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&existingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if managerUserID != nil {
				return ErrAdminUserNotManaged
			}
			return ErrAdminUserNotFound
		}
		return fmt.Errorf("load admin user for status update: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE admin_users SET disabled = ? WHERE id = ?", disabled, userID); err != nil {
		return fmt.Errorf("update admin user status: %w", err)
	}
	if disabled {
		if _, err := tx.ExecContext(ctx, "DELETE FROM admin_sessions WHERE user_id = ?", userID); err != nil {
			return fmt.Errorf("delete disabled admin user sessions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit admin user status update: %w", err)
	}
	return nil
}

func ResetAdminUserPassword(ctx context.Context, db *sql.DB, userID int64, password string) error {
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reset admin user password: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM admin_users WHERE id = ? FOR UPDATE", userID).Scan(&existingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAdminUserNotFound
		}
		return fmt.Errorf("load admin user for password reset: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE admin_users SET password_hash = ? WHERE id = ?", hash, userID); err != nil {
		return fmt.Errorf("reset admin user password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM admin_sessions WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("delete password-reset admin user sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit admin user password reset: %w", err)
	}
	return nil
}

func SetAdminUserRole(ctx context.Context, db *sql.DB, userID int64, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "user" && role != AdminUserRoleAccountManager {
		return ErrInvalidAdminUserRole
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update admin user role: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentRole string
	var createdByUserID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
SELECT role, created_by_user_id
FROM admin_users
WHERE id = ?
FOR UPDATE`, userID).Scan(&currentRole, &createdByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAdminUserNotFound
		}
		return fmt.Errorf("load admin user for role update: %w", err)
	}
	if currentRole == "root" || currentRole == "admin" {
		return ErrProtectedAdminUserRole
	}
	if role == AdminUserRoleAccountManager && createdByUserID.Valid {
		return ErrNestedAccountManager
	}
	if _, err := tx.ExecContext(ctx, "UPDATE admin_users SET role = ? WHERE id = ?", role, userID); err != nil {
		return fmt.Errorf("update admin user role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit admin user role update: %w", err)
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
