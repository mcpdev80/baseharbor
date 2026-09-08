package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVerifySchemaReadyTracksAuthoritativeMigrationState(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := resetTestDatabase(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := VerifySchemaReady(ctx, pool); !errors.Is(err, ErrSchemaNotReady) {
		t.Fatalf("schema before migration error = %v, want ErrSchemaNotReady", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := VerifySchemaReady(ctx, pool); err != nil {
		t.Fatalf("schema after migration: %v", err)
	}

	names, err := forwardMigrationNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("expected embedded migrations")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM baseharbor_schema_migrations WHERE version = $1", names[len(names)-1]); err != nil {
		t.Fatal(err)
	}
	if err := VerifySchemaReady(ctx, pool); !errors.Is(err, ErrSchemaNotReady) {
		t.Fatalf("missing migration error = %v, want ErrSchemaNotReady", err)
	}
}
