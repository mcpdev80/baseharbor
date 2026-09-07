package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidTenantID = errors.New("invalid tenant id")

// WithTenantTx runs fn inside a transaction whose PostgreSQL session context is
// scoped to exactly one tenant. SET LOCAL semantics ensure the tenant context is
// automatically discarded when the transaction ends.
func WithTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	if pool == nil {
		return errors.New("database pool is nil")
	}
	if fn == nil {
		return errors.New("tenant transaction function is nil")
	}

	var parsed pgtype.UUID
	if err := parsed.Scan(tenantID); err != nil || !parsed.Valid {
		return ErrInvalidTenantID
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tenant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"SELECT set_config('baseharbor.tenant_id', $1, true)",
		tenantID,
	); err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant transaction: %w", err)
	}
	return nil
}
