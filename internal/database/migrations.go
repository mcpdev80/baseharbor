package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const migrationTable = `
CREATE TABLE IF NOT EXISTS baseharbor_schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

var ErrNoAppliedMigration = errors.New("no applied migration to roll back")

// Migrate applies all embedded forward migrations exactly once in lexical order.
// Each migration runs in its own transaction. The migration table is authoritative.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	if _, err := pool.Exec(ctx, migrationTable); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	names, err := forwardMigrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		applied, err := migrationApplied(ctx, pool, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(ctx, pool, name, string(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

// RollbackLast reverses exactly the most recently applied BaseHarbor migration.
// A rollback file is mandatory; rollback never guesses or destroys earlier schema.
func RollbackLast(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	if _, err := pool.Exec(ctx, migrationTable); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	var version string
	err := pool.QueryRow(ctx, `
SELECT version
FROM baseharbor_schema_migrations
ORDER BY applied_at DESC, version DESC
LIMIT 1
`).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoAppliedMigration
	}
	if err != nil {
		return fmt.Errorf("find last migration: %w", err)
	}

	downName := strings.TrimSuffix(version, ".sql") + ".down.sql"
	sqlBytes, err := migrationFS.ReadFile("migrations/" + downName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("rollback %s: missing %s", version, downName)
		}
		return fmt.Errorf("read rollback %s: %w", downName, err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin rollback %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("rollback migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM baseharbor_schema_migrations WHERE version = $1", version); err != nil {
		return fmt.Errorf("remove migration record %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollback %s: %w", version, err)
	}
	return nil
}

func forwardMigrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") || strings.HasSuffix(entry.Name(), ".down.sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func migrationApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM baseharbor_schema_migrations WHERE version = $1)",
		version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return exists, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, version, sqlText string) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sqlText); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO baseharbor_schema_migrations (version) VALUES ($1)",
		version,
	); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}
