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
		Version:                 sharedBackendStateVersion,
		PostgresAdminCredential: "credentials/postgres/provider-admin.password",
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

func TestVerifySharedPostgresStateOwnershipRejectsProviderAdminLeakage(t *testing.T) {
	state := sharedBackendState{
		Version:                 sharedBackendStateVersion,
		PostgresAdminCredential: "credentials/postgres/provider-admin.password",
		Applications: map[string]sharedBackendAppState{
			"app-a/dev": {
				Application: "app-a", Environment: "dev",
				SQL: map[string]sharedPostgresResource{"default": {
					Database: "app_a_dev", Username: "baseharbor_admin", CredentialReference: "credentials/postgres/app-a-dev-default.password",
				}},
			},
		},
	}
	if err := verifySharedPostgresStateOwnership(state, "app-a/dev"); err == nil || !strings.Contains(err.Error(), "provider administrator role") {
		t.Fatalf("verifySharedPostgresStateOwnership() admin role error = %v", err)
	}

	resource := state.Applications["app-a/dev"]
	resource.SQL["default"] = sharedPostgresResource{
		Database: "app_a_dev", Username: "baha_app_a_dev", CredentialReference: state.PostgresAdminCredential,
	}
	state.Applications["app-a/dev"] = resource
	if err := verifySharedPostgresStateOwnership(state, "app-a/dev"); err == nil || !strings.Contains(err.Error(), "provider administrator credential") {
		t.Fatalf("verifySharedPostgresStateOwnership() admin credential error = %v", err)
	}
}

func TestVerifySharedPostgresStateOwnershipRejectsProviderDatabases(t *testing.T) {
	for _, database := range []string{"postgres", "template0", "template1"} {
		state := sharedBackendState{
			Version:                 sharedBackendStateVersion,
			PostgresAdminCredential: "credentials/postgres/provider-admin.password",
			Applications: map[string]sharedBackendAppState{
				"app-a/dev": {
					Application: "app-a", Environment: "dev",
					SQL: map[string]sharedPostgresResource{"default": {
						Database: database, Username: "baha_app_a_dev", CredentialReference: "credentials/postgres/app-a-dev-default.password",
					}},
				},
			},
		}
		if err := verifySharedPostgresStateOwnership(state, "app-a/dev"); err == nil || !strings.Contains(err.Error(), "provider database") {
			t.Fatalf("verifySharedPostgresStateOwnership(%q) error = %v", database, err)
		}
	}
}

func TestVerifySharedPostgresStateOwnershipRequiresProviderAdminCredential(t *testing.T) {
	state := sharedBackendState{
		Version: sharedBackendStateVersion,
		Applications: map[string]sharedBackendAppState{
			"app-a/dev": {
				Application: "app-a", Environment: "dev",
				SQL: map[string]sharedPostgresResource{"default": {
					Database: "app_a_dev", Username: "baha_app_a_dev", CredentialReference: "credentials/postgres/app-a-dev-default.password",
				}},
			},
		},
	}
	if err := verifySharedPostgresStateOwnership(state, "app-a/dev"); err == nil || !strings.Contains(err.Error(), "administrator credential reference is missing") {
		t.Fatalf("verifySharedPostgresStateOwnership() error = %v", err)
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

func TestSharedValkeyComposeUsesNumericNonRootIdentity(t *testing.T) {
	var b strings.Builder
	writeSharedValkeyCompose(&b, sharedBackendAppState{Application: "demo", Environment: "dev"}, "default")
	got := b.String()
	if !strings.Contains(got, `user: "999:1000"`) {
		t.Fatalf("shared Valkey compose missing numeric non-root identity:\n%s", got)
	}
	if strings.Contains(got, `user: "valkey"`) {
		t.Fatalf("shared Valkey compose must not use symbolic runtime identity:\n%s", got)
	}
}

func TestSharedPostgresComposeDoesNotEnableLegacySpiloAdminUsers(t *testing.T) {
	var b strings.Builder
	writeSharedPostgresCompose(&b, sharedBackendState{Environment: "dev"})
	got := b.String()

	if strings.Contains(got, "USE_ADMIN:") {
		t.Fatalf("shared PostgreSQL Compose must not enable Spilo legacy bootstrap.users path:\n%s", got)
	}
	if !strings.Contains(got, "PGUSER_SUPERUSER: postgres") {
		t.Fatalf("shared PostgreSQL Compose must retain postgres as Spilo bootstrap superuser:\n%s", got)
	}
}

func TestSharedPostgresComposeUsesPreparedTLSRuntime(t *testing.T) {
	var b strings.Builder
	writeSharedPostgresCompose(&b, sharedBackendState{Environment: "dev"})
	got := b.String()

	for _, want := range []string{
		"uid=$(id -u postgres); gid=$(id -g postgres)",
		"chmod 0755 /run/baseharbor",
		"chown \"0:$gid\" /run/baseharbor/tls/server-cert.pem /run/baseharbor/tls/server-key.pem",
		"chmod 0640 /run/baseharbor/tls/server-key.pem",
		"./postgresql/runtime:/run/baseharbor/tls-source:ro",
		"exec /bin/sh /launch.sh init",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("shared PostgreSQL Compose missing %q:\n%s", want, got)
		}
	}
	for _, member := range []string{
		sharedPostgresMemberService("dev", 1),
		sharedPostgresMemberService("dev", 2),
		sharedPostgresMemberService("dev", 3),
	} {
		start := strings.Index(got, "  "+member+":\n")
		if start < 0 {
			t.Fatalf("missing shared PostgreSQL member %s:\n%s", member, got)
		}
		end := strings.Index(got[start+2:], "\n  ")
		block := got[start:]
		if end >= 0 {
			block = got[start : start+2+end]
		}
		if strings.Contains(block, `cap_drop: ["ALL"]`) {
			t.Fatalf("Spilo member %s must retain bootstrap capabilities:\n%s", member, block)
		}
		if !strings.Contains(block, "/run/baseharbor/tls:rw,noexec,nosuid,nodev,mode=0750") {
			t.Fatalf("Spilo member %s must use private TLS tmpfs:\n%s", member, block)
		}
	}
}

func TestSharedValkeyComposePreservesPasswordForContainerShell(t *testing.T) {
	var b strings.Builder
	writeSharedValkeyCompose(&b, sharedBackendAppState{Application: "demo", Environment: "dev"}, "default")
	got := b.String()
	for _, want := range []string{
		`printf 'requirepass %s\n' "$$VALKEY_PASSWORD"`,
		`printf 'masterauth %s\n' "$$VALKEY_PASSWORD"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("shared Valkey Compose missing escaped runtime variable %q:\n%s", want, got)
		}
	}
}
