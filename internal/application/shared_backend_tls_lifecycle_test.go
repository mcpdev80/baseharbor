package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestSharedBackendTLSLifecycleUsesPhysicalOwners(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), "shared")
	root := t.TempDir()
	issuer := newSharedCoreTestIssuer(t, root, "tls-target", false)
	m := New("consumer", "dev", true, true, false)
	shared := SharedBackendFilesAt(root, "tls-target", m.Environment)
	if err := os.MkdirAll(shared.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{
		Version: sharedBackendStateVersion, Environment: "core",
		CoreSQL: &issuer.core, ValkeyAdminCredential: "credentials/core-admin",
		ValkeyMembers: 1, Applications: map[string]sharedBackendAppState{},
	}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}
	sqlPKI := filepath.Join(filepath.Dir(issuer.core.Compose), "providers", "postgresql", "service-access", "pki")
	cachePKI := filepath.Join(shared.Dir, "valkey", "core", defaultServiceInstance, "service-access", "pki")
	writeLifecycle := func(dir, owner string, expires time.Time) {
		t.Helper()
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(map[string]any{
			"version": 2, "source": "managed-local", "lifecycle_owner": owner,
			"renewal_mode": "automatic-reconcile", "server_expires_at": expires,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeLifecycle(sqlPKI, "core-sql", time.Now().Add(60*24*time.Hour))
	writeLifecycle(cachePKI, "core-valkey", time.Now().Add(60*24*time.Hour))
	observations, err := InspectSharedBackendTLSLifecycleAt(root, "tls-target", m)
	if err != nil || len(observations) != 2 {
		t.Fatalf("physical TLS owners were not observed: %v", err)
	}
	if observations[0].Lifecycle.LifecycleOwner != "core-sql" || observations[1].Lifecycle.LifecycleOwner != "core-valkey" {
		t.Fatal("consumer TLS observation substituted another physical owner")
	}
	writeLifecycle(sqlPKI, "core-sql", time.Now().Add(-time.Hour))
	observations, err = InspectSharedBackendTLSLifecycleAt(root, "tls-target", m)
	if err != nil || observations[0].Lifecycle.Health != "critical" || observations[1].Lifecycle.Health != "ok" {
		t.Fatal("expired Core SQL certificate was hidden or changed cache health")
	}
	if err := os.Remove(filepath.Join(filepath.Dir(issuer.core.Compose), "providers", "postgresql", "runtime", "ca.pem")); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSharedBackendTLSLifecycleAt(root, "tls-target", m); err == nil {
		t.Fatal("missing physical Core SQL trust was accepted")
	}
}
