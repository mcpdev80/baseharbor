package database

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestOpenRejectsMissingDSN(t *testing.T) {
	pool, err := Open(context.Background(), Config{})
	if pool != nil {
		pool.Close()
		t.Fatal("expected no pool")
	}
	if !errors.Is(err, ErrMissingDSN) {
		t.Fatalf("expected ErrMissingDSN, got %v", err)
	}
}

func TestCoreMigrationIsEmbedded(t *testing.T) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one embedded migration")
	}

	raw, err := migrationFS.ReadFile("migrations/0001_core_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{"CREATE TABLE tenants", "CREATE TABLE external_identities", "CREATE TABLE memberships"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
