package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrAPITokenLimit = errors.New("active API token limit reached")

type APIToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func ListAPITokens(ctx context.Context, db *sql.DB, userID int64) ([]APIToken, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, prefix, expires_at, revoked_at, last_used_at, created_at FROM user_api_tokens WHERE user_id = ? ORDER BY (revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP) DESC, id DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []APIToken{}
	for rows.Next() {
		var item APIToken
		if err := rows.Scan(&item.ID, &item.Name, &item.Prefix, &item.ExpiresAt, &item.RevokedAt, &item.LastUsedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func CreateAPIToken(ctx context.Context, db *sql.DB, userID int64, name, hash, prefix string, expires time.Time) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM admin_users WHERE id = ? AND disabled = FALSE FOR UPDATE`, userID).Scan(&id); err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_api_tokens WHERE user_id = ? AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, userID).Scan(&count); err != nil {
		return 0, err
	}
	if count >= 10 {
		return 0, ErrAPITokenLimit
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO user_api_tokens(user_id, name, token_hash, prefix, expires_at) VALUES (?, ?, ?, ?, ?)`, userID, name, hash, prefix, expires)
	if err != nil {
		return 0, err
	}
	id, err = result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor, action, target_type, target_id, detail_json) SELECT username, 'api_token.create', 'user_api_tokens', ?, JSON_OBJECT('name', ?) FROM admin_users WHERE id = ?`, id, name, userID); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func RevokeAPIToken(ctx context.Context, db *sql.DB, userID, tokenID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM user_api_tokens WHERE id = ? AND user_id = ? FOR UPDATE`, tokenID, userID).Scan(&id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_api_tokens SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP) WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor, action, target_type, target_id, detail_json) SELECT username, 'api_token.revoke', 'user_api_tokens', ?, '{}' FROM admin_users WHERE id = ?`, id, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func AuthenticateAPIToken(ctx context.Context, db *sql.DB, hash string) (*AdminUser, int64, error) {
	var user AdminUser
	var tokenID int64
	err := db.QueryRowContext(ctx, `SELECT u.id, u.username, u.display_name, u.role, t.id
 FROM user_api_tokens t JOIN admin_users u ON u.id = t.user_id
 WHERE t.token_hash = ? AND t.revoked_at IS NULL AND t.expires_at > CURRENT_TIMESTAMP AND u.disabled = FALSE`, hash).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &tokenID)
	if err != nil {
		return nil, 0, err
	}
	_, _ = db.ExecContext(ctx, `UPDATE user_api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ? AND (last_used_at IS NULL OR last_used_at < CURRENT_TIMESTAMP - INTERVAL 1 MINUTE)`, tokenID)
	return &user, tokenID, nil
}

// The database clock and row lock make the limit shared by all tokens and instances.
func AllowAPIRequest(ctx context.Context, db *sql.DB, userID int64, bucket string, limit int) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO user_api_rate_limits(user_id, bucket, window_start, request_count)
 VALUES (?, ?, FLOOR(UNIX_TIMESTAMP() / 60), 1)
 ON DUPLICATE KEY UPDATE request_count = IF(window_start = FLOOR(UNIX_TIMESTAMP() / 60), request_count + 1, 1), window_start = FLOOR(UNIX_TIMESTAMP() / 60)`, userID, bucket)
	if err != nil {
		return false, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT request_count FROM user_api_rate_limits WHERE user_id = ? AND bucket = ?`, userID, bucket).Scan(&count); err != nil {
		return false, err
	}
	return count <= limit, tx.Commit()
}

// Keep the connection pinned: MySQL advisory locks belong to a connection.
func AcquireAPILock(ctx context.Context, db *sql.DB, key string, waitSeconds int) (func(), bool, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var databaseName string
	if err := conn.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&databaseName); err != nil {
		conn.Close()
		return nil, false, err
	}
	digest := sha256.Sum256([]byte(databaseName + "\x00" + key))
	name := fmt.Sprintf("kh:%x", digest[:24])
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, name, waitSeconds).Scan(&locked); err != nil {
		conn.Close()
		return nil, false, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		conn.Close()
		return nil, false, nil
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(releaseCtx, `DO RELEASE_LOCK(?)`, name)
		_ = conn.Close()
	}
	return release, true, nil
}
