package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	utcDSN, err := normalizeDSNToUTC(dsn)
	if err != nil {
		return nil, fmt.Errorf("configure mysql: %w", err)
	}

	db, err := sql.Open("mysql", utcDSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}

// normalizeDSNToUTC keeps MySQL's TIMESTAMP conversion and Go's time parsing in
// the same timezone. API timestamps are then serialized as UTC and converted to
// the user's timezone only by the frontend.
func normalizeDSNToUTC(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("parse DSN: %w", err)
	}

	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	cfg.Params["time_zone"] = "'+00:00'"

	return cfg.FormatDSN(), nil
}
