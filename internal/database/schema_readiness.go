package database

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSchemaNotReady       = errors.New("database schema is not ready")
	ErrUnexpectedMigration  = errors.New("database has unexpected migration")
)

// VerifySchemaReady verifies the authoritative migration state without mutating it.
// Runtime processes must not apply migrations implicitly during startup.
func VerifySchemaReady(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database pool is nil")
	}

	expected, err := forwardMigrationNames()
	if err != nil {
		return err
	}

	rows, err := pool.Query(ctx, `
SELECT version
FROM baseharbor_schema_migrations
ORDER BY version
`)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSchemaNotReady
	}
	if err != nil {
		return fmt.Errorf("%w: read migration state: %v", ErrSchemaNotReady, err)
	}
	defer rows.Close()

	applied := make([]string, 0, len(expected))
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("%w: scan migration state: %v", ErrSchemaNotReady, err)
		}
		applied = append(applied, version)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: read migration state: %v", ErrSchemaNotReady, err)
	}

	sort.Strings(expected)
	if len(applied) != len(expected) {
		return fmt.Errorf("%w: applied=%d expected=%d", ErrSchemaNotReady, len(applied), len(expected))
	}
	for i := range expected {
		if applied[i] != expected[i] {
			if !containsMigration(expected, applied[i]) {
				return fmt.Errorf("%w: %s", ErrUnexpectedMigration, applied[i])
			}
			return fmt.Errorf("%w: expected %s", ErrSchemaNotReady, expected[i])
		}
	}
	return nil
}

func containsMigration(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}
