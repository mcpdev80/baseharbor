package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestSharedPostgresIdentityIsDeterministicAndCollisionSafe(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	a := WithSQLInstances(New("payments-super-long-application-name-that-would-overflow-postgresql-identifiers", "dev", true, false, false), "primary", "analytics")
	b := WithSQLInstances(New("payments-super-long-application-name-that-would-overflow-postgresql-identifiers", "test", true, false, false), "primary", "analytics")

	seen := map[string]struct{}{}
	for _, m := range []Manifest{a, b} {
		for _, instance := range SQLInstanceNames(m) {
			db := sharedPostgresDatabaseName(m, instance)
			role := sharedPostgresRoleName(m, instance)
			for kind, value := range map[string]string{"database": db, "role": role} {
				if len(value) > 63 {
					t.Fatalf("%s %q exceeds PostgreSQL identifier limit", kind, value)
				}
				if _, exists := seen[value]; exists {
					t.Fatalf("%s identity %q collided across app/env/instance resources", kind, value)
				}
				seen[value] = struct{}{}
			}
			if db != sharedPostgresDatabaseName(m, instance) || role != sharedPostgresRoleName(m, instance) {
				t.Fatalf("shared PostgreSQL identity is not deterministic for %s/%s/%s", m.Name, m.Environment, instance)
			}
		}
	}
}

func TestVerifySharedPostgresStateOwnershipFailsClosedOnAmbiguity(t *testing.T) {
	state := sharedBackendState{
		Version: sharedBackendStateVersion,
		Applications: map[string]sharedBackendAppState{
			"app-a/dev": {
				Application: "app-a", Environment: "dev",
				SQL: map[string]sharedPostgresResource{"default": {
					Database: "db_a", Username: "role_a", CredentialReference: "credentials/postgres/a.password",
				}},
			},
			"app-b/dev": {
				Application: "app-b", Environment: "dev",
				SQL: map[string]sharedPostgresResource{"default": {
					Database: "db_a", Username: "role_b", CredentialReference: "credentials/postgres/b.password",
				}},
			},
		},
	}
	if err := verifySharedPostgresStateOwnership(state, "app-a/dev"); err == nil || !strings.Contains(err.Error(), "ambiguous owners") {
		t.Fatalf("verifySharedPostgresStateOwnership() error = %v, want ambiguous owners", err)
	}

	state.Applications["app-b/dev"] = sharedBackendAppState{
		Application: "app-b", Environment: "dev",
		SQL: map[string]sharedPostgresResource{"default": {
			Database: "db_b", Username: "role_a", CredentialReference: "credentials/postgres/b.password",
		}},
	}
	if err := verifySharedPostgresStateOwnership(state, "app-a/dev"); err == nil || !strings.Contains(err.Error(), "ambiguous owners") {
		t.Fatalf("verifySharedPostgresStateOwnership() role error = %v, want ambiguous owners", err)
	}
}

func TestSharedPostgresStateContainsCredentialReferencesNotSecrets(t *testing.T) {
	root := t.TempDir()
	adminRef, err := ensureSharedPostgresCredential(root, "provider-admin", "")
	if err != nil {
		t.Fatalf("ensure admin credential: %v", err)
	}
	appRef, err := ensureSharedPostgresCredential(root, "app-a/dev", "default")
	if err != nil {
		t.Fatalf("ensure app credential: %v", err)
	}
	adminSecret, err := readSharedBackendCredential(root, adminRef)
	if err != nil {
		t.Fatalf("read admin credential: %v", err)
	}
	appSecret, err := readSharedBackendCredential(root, appRef)
	if err != nil {
		t.Fatalf("read app credential: %v", err)
	}

	state := sharedBackendState{
		Version:                 sharedBackendStateVersion,
		PostgresAdminCredential: adminRef,
		Applications: map[string]sharedBackendAppState{
			"app-a/dev": {
				Application: "app-a", Environment: "dev",
				SQL: map[string]sharedPostgresResource{"default": {
					Database: "db_a", Username: "role_a", CredentialReference: appRef,
				}},
			},
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	if strings.Contains(serialized, adminSecret) || strings.Contains(serialized, appSecret) {
		t.Fatalf("shared backend state leaked PostgreSQL credentials")
	}
	if !strings.Contains(serialized, adminRef) || !strings.Contains(serialized, appRef) {
		t.Fatalf("shared backend state did not persist protected credential references")
	}
	for _, ref := range []string{adminRef, appRef} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(ref)))
		if err != nil {
			t.Fatalf("stat credential %s: %v", ref, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credential %s mode = %o, want 600", ref, info.Mode().Perm())
		}
	}
}
