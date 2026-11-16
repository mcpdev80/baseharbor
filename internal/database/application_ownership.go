package database

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidApplicationName       = errors.New("invalid application name")
	ErrApplicationOwnershipConflict = errors.New("application is owned by another tenant")
)

// ApplicationOwnershipStore is the authoritative persistence boundary for
// application-to-tenant ownership.
type ApplicationOwnershipStore struct {
	pool *pgxpool.Pool
}

func NewApplicationOwnershipStore(pool *pgxpool.Pool) *ApplicationOwnershipStore {
	return &ApplicationOwnershipStore{pool: pool}
}

func (s *ApplicationOwnershipStore) Claim(ctx context.Context, tenantID, applicationName string) error {
	if s == nil || s.pool == nil {
		return errors.New("database pool is nil")
	}
	applicationName = strings.TrimSpace(applicationName)
	if applicationName == "" {
		return ErrInvalidApplicationName
	}

	var inserted bool
	err := WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `
INSERT INTO application_ownerships (application_name, tenant_id)
VALUES ($1, $2)
ON CONFLICT (application_name) DO NOTHING
`, applicationName, tenantID)
		if err != nil {
			return err
		}
		inserted = command.RowsAffected() == 1
		if inserted {
			return nil
		}

		var owned bool
		if err := tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM application_ownerships WHERE application_name = $1)",
			applicationName,
		).Scan(&owned); err != nil {
			return err
		}
		if !owned {
			return ErrApplicationOwnershipConflict
		}
		return nil
	})
	return err
}

// OwnedByTenant implements the application-secret API ownership resolver.
// The lookup is tenant-scoped and therefore subject to PostgreSQL RLS.
func (s *ApplicationOwnershipStore) OwnedByTenant(ctx context.Context, tenantID, applicationName string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database pool is nil")
	}
	applicationName = strings.TrimSpace(applicationName)
	if applicationName == "" {
		return false, ErrInvalidApplicationName
	}

	var owned bool
	err := WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM application_ownerships WHERE application_name = $1)",
			applicationName,
		).Scan(&owned)
	})
	if err != nil {
		return false, err
	}
	return owned, nil
}
