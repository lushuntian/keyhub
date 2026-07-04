package database

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ApplyMigrations(ctx context.Context, db *sql.DB, dir string) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(128) PRIMARY KEY,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %q: %w", dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)

	for _, file := range files {
		applied, err := migrationApplied(ctx, db, file)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigrationFile(ctx, db, filepath.Join(dir, file), file); err != nil {
			return err
		}
	}

	return nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var existing string
	err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations WHERE version = ?", version).Scan(&existing)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return true, nil
}

func applyMigrationFile(ctx context.Context, db *sql.DB, path string, version string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", version, err)
	}

	statements := splitSQLStatements(string(content))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
	}

	if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", version); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}

	return nil
}

func splitSQLStatements(sqlText string) []string {
	scanner := bufio.NewScanner(strings.NewReader(sqlText))
	statements := make([]string, 0)
	var builder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
		if strings.HasSuffix(trimmed, ";") {
			statement := strings.TrimSpace(builder.String())
			statement = strings.TrimSuffix(statement, ";")
			if statement != "" {
				statements = append(statements, statement)
			}
			builder.Reset()
		}
	}

	if rest := strings.TrimSpace(builder.String()); rest != "" {
		statements = append(statements, rest)
	}

	return statements
}
