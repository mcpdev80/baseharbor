package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSchemaNotReady      = errors.New("database schema is not ready")
	ErrUnexpectedMigration = errors.New("database has unexpected migration")
)

// VerifySchemaReady verifies the authoritative migration state without mutating it.
// Runtime processes must not apply migrations implicitly during startup.
func VerifySchemaReady(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("database pool is nil")
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

	expectedSet := make(map[string]struct{}, len(expected))
	for _, version := range expected {
		expectedSet[version] = struct{}{}
	}
	for _, version := range applied {
		if _, ok := expectedSet[version]; !ok {
			return fmt.Errorf("%w: %s", ErrUnexpectedMigration, version)
		}
	}
	if len(applied) != len(expected) {
		return fmt.Errorf("%w: applied=%d expected=%d", ErrSchemaNotReady, len(applied), len(expected))
	}
	for i := range expected {
		if applied[i] != expected[i] {
			return fmt.Errorf("%w: expected %s", ErrSchemaNotReady, expected[i])
		}
	}
	return nil
}
