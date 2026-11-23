package application

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

func TestSharedPgAdminTrustsSingleGatewayForwardedHost(t *testing.T) {
	var compose strings.Builder
	writeSharedPostgresUICompose(&compose)
	if !strings.Contains(compose.String(), "PGADMIN_CONFIG_PROXY_X_HOST_COUNT: \"1\"") {
		t.Fatalf("pgAdmin must trust the host forwarded by its single BaseHarbor gateway proxy:\n%s", compose.String())
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

func TestSharedValkeyComposeLeavesUIDToTheRuntime(t *testing.T) {
	var b strings.Builder
	writeSharedValkeyCompose(&b, sharedBackendAppState{Application: "demo", Environment: "dev"}, "default")
	got := b.String()
	if strings.Contains(got, "user:") {
		t.Fatalf("shared Valkey compose must leave UID assignment to the runtime:\n%s", got)
	}
}

func TestSharedPostgresComposeDoesNotEnableLegacySpiloAdminUsers(t *testing.T) {
	var b strings.Builder
	writeSharedPostgresCompose(&b, sharedBackendState{Environment: "dev", PostgresMembers: 3})
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
	writeSharedPostgresCompose(&b, sharedBackendState{Environment: "dev", PostgresMembers: 3})
	got := b.String()

	for _, want := range []string{
		"PGROOT: /home/postgres/pgroot",
		"PGDATA: /home/postgres/pgroot/data",
		"shared-postgres-data-1:/home/postgres/pgroot",
		"chmod 0600 /run/baseharbor/tls/server-key.pem",
		"if [ \"$$(id -u)\" = 0 ]; then chown postgres:postgres /run/baseharbor/tls/server-key.pem; fi",
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
		var blockLines []string
		for i, line := range strings.Split(got[start:], "\n") {
			if i > 0 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
				break
			}
			blockLines = append(blockLines, line)
		}
		block := strings.Join(blockLines, "\n")
		if strings.Contains(block, `cap_drop: ["ALL"]`) {
			t.Fatalf("Spilo member %s must retain bootstrap capabilities:\n%s", member, block)
		}
		if !strings.Contains(block, "/run/baseharbor/tls:rw,noexec,nosuid,nodev,mode=1777") {
			t.Fatalf("Spilo member %s TLS tmpfs must permit a platform-assigned UID to create private owner-only key copies:\n%s", member, block)
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

func sharedBackendStateVersionString(n int) string { return strconv.Itoa(n) }

// TestSharedBackendStateVersionMismatchNamesVersions ensures the human surface
// gets a distinct, upgrade-from-outside message: it names the version found and
// the supported version and points at the supported next step, and it is
// distinguishable from a corrupt or unreadable state file.
func TestSharedBackendStateVersionMismatchNamesVersions(t *testing.T) {
	for _, found := range []int{1, 2, 99} {
		data := `{"version":` + strconv.Itoa(found) + `,"environment":"dev"}`
		_, err := decodeSharedBackendState([]byte(data))
		if err == nil {
			t.Fatalf("version %d must fail closed", found)
		}
		var mismatch *SharedBackendStateVersionMismatch
		if !errors.As(err, &mismatch) {
			t.Fatalf("version %d did not return SharedBackendStateVersionMismatch: %v", found, err)
		}
		if mismatch.Found != found {
			t.Fatalf("mismatch.Found = %d, want %d", mismatch.Found, found)
		}
		if mismatch.Expected != sharedBackendStateVersion {
			t.Fatalf("mismatch.Expected = %d, want %d", mismatch.Expected, sharedBackendStateVersion)
		}
		msg := err.Error()
		if !strings.Contains(msg, "upgrade-from-outside") {
			t.Fatalf("message must signal upgrade-from-outside: %q", msg)
		}
		if !strings.Contains(msg, strconv.Itoa(found)) || !strings.Contains(msg, strconv.Itoa(sharedBackendStateVersion)) {
			t.Fatalf("message must name found and expected version: %q", msg)
		}
	}
}

// TestSharedBackendStateVersionMismatchIsDistinguishableFromCorruption ensures
// IsSharedBackendStateVersionMismatch is true only for the version gate, so a
// caller can tell it apart from a corrupt or unreadable file.
func TestSharedBackendStateVersionMismatchIsDistinguishableFromCorruption(t *testing.T) {
	_, err := decodeSharedBackendState([]byte(`{"version":2}`))
	if err == nil {
		t.Fatal("version gate must report as version mismatch")
	}
	if !IsSharedBackendStateVersionMismatch(err) {
		t.Fatal("version gate must report as version mismatch")
	}
	// A parse failure is corruption, not a version mismatch.
	_, err = decodeSharedBackendState([]byte(`not json`))
	if IsSharedBackendStateVersionMismatch(err) {
		t.Fatal("corrupt state must not be reported as version mismatch")
	}
	if IsSharedBackendStateVersionMismatch(nil) {
		t.Fatal("nil must not be a version mismatch")
	}
}

// TestSharedBackendStateLoadReportsVersionReason runs the real load path over a
// v2 state written to disk and asserts the version reason (not a generic error)
// reaches the caller, and that nothing on disk was migrated.
func TestSharedBackendStateLoadReportsVersionReason(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	v2 := `{"version":2,"environment":"dev"}`
	if err := os.WriteFile(statePath, []byte(v2), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadSharedBackendState(statePath, "dev")
	if err == nil {
		t.Fatal("v2 state must fail closed")
	}
	var mismatch *SharedBackendStateVersionMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected version mismatch, got %v", err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("load must not rewrite the provider state (fail-closed stays byte-identical)")
	}
}

// TestSharedBackendStateCorrectVersionStillLoads keeps the happy path green so
// the version gate cannot drift into a silent accept.
func TestSharedBackendStateCorrectVersionStillLoads(t *testing.T) {
	data := `{"version":` + strconv.Itoa(sharedBackendStateVersion) + `,"environment":"dev"}`
	state, err := decodeSharedBackendState([]byte(data))
	if err != nil {
		t.Fatalf("supported version must load: %v", err)
	}
	if state.Version != sharedBackendStateVersion {
		t.Fatalf("version = %d, want %d", state.Version, sharedBackendStateVersion)
	}
}
