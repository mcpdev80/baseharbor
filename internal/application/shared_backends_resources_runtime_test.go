package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// noopRuntimeEmbeds bhruntime.RuntimeProvider and overrides only the methods the
// shared-destroy path calls. It never builds or tears down containers.
type noopRuntimeEmbeds struct {
	bhruntime.RuntimeProvider
}

func (noopRuntimeEmbeds) DestroyProjectRemoveOrphans(context.Context, string, string, string) error {
	return nil
}

// registerSharedPlatformSource goes through the productive RegisterProviderSignals
// path (the owner's 0428-166 collision/owner-guard in provider_registration.go),
// with a real provider-kind and a real shared SharingBoundary, so the regression
// proves the shared-destroy prune against the same registration key a real
// deploy writes - not a registry write that bypasses that guard.
func registerSharedPlatformSource(t *testing.T, env string, descriptor capability.IntegrationDescriptor) {
	t.Helper()
	kind := descriptor.Provider.Kind
	id := string(kind) + ":shared:" + env
	if err := observability.RegisterProviderSignals(observability.ProviderSignalRegistration{
		ID:              id,
		Descriptor:      descriptor,
		Class:           observability.SourcePlatformProvider,
		Scope:           capability.ScopeShared,
		SharingBoundary: "environment:" + env,
		Enabled:         map[observability.SignalKind]bool{observability.SignalLogs: true, observability.SignalTraces: true},
		Signals:         map[string]observability.ProviderSignalRuntime{"logs": {Target: "shared"}, "traces": {Target: "shared"}},
	}); err != nil {
		t.Fatalf("register shared platform source env %q provider %q: %v", env, kind, err)
	}
}

func sharedPlatformCounts(t *testing.T, boundary string) map[string]int {
	t.Helper()
	logs, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: boundary},
		nil, true, true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, s := range logs {
		counts[string(s.Provider)]++
	}
	return counts
}

// TestSharedDestroyRemovesOwnedPlatformSources asserts that DestroyAllSharedBackendsAt
// tears down exactly the platform-scoped observability rows it owns (the one
// environment directory present) for BOTH postgres and valkey, and that a
// following re-registration with a fresh owner succeeds through the productive
// registration path.
func TestSharedDestroyRemovesOwnedPlatformSources(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// "core" owns one postgres + one valkey platform source; "other" owns the
	// same pair under a different boundary. Only "core" is on disk.
	registerSharedPlatformSource(t, "core", capability.PostgreSQLIntegration)
	registerSharedPlatformSource(t, "core", capability.ValkeyIntegration)
	registerSharedPlatformSource(t, "other", capability.PostgreSQLIntegration)
	registerSharedPlatformSource(t, "other", capability.ValkeyIntegration)

	before := sharedPlatformCounts(t, "environment:core")
	if before["postgresql"] != 1 || before["valkey"] != 1 {
		t.Fatalf("precondition: expected 1 postgres + 1 valkey under core, got %v", before)
	}

	dataDir := t.TempDir()
	if err := setupSharedBackendDir(t, dataDir, "core"); err != nil {
		t.Fatal(err)
	}

	if err := DestroyAllSharedBackendsAt(context.Background(), noopRuntimeEmbeds{}, dataDir, "orphan"); err != nil {
		t.Fatal(err)
	}

	if after := sharedPlatformCounts(t, "environment:core"); len(after) != 0 {
		t.Fatalf("owned platform sources (both providers) not removed by shared destroy: %#v", after)
	}

	// Re-registration with a fresh owner now succeeds for the same boundary,
	// through the productive RegisterProviderSignals guard.
	registerSharedPlatformSource(t, "core", capability.PostgreSQLIntegration)
	registerSharedPlatformSource(t, "core", capability.ValkeyIntegration)
	after := sharedPlatformCounts(t, "environment:core")
	if after["postgresql"] != 1 || after["valkey"] != 1 {
		t.Fatalf("re-registration did not recover (stale owner survived): %#v", after)
	}
}

// TestSharedDestroyPreservesForeignPlatformSources is the cross-tenant wipe
// guard: destroying "core" must NOT remove the postgres + valkey platform rows
// owned by a different boundary ("other"). The most dangerous variant; must stay
// green for BOTH providers.
func TestSharedDestroyPreservesForeignPlatformSources(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	registerSharedPlatformSource(t, "core", capability.PostgreSQLIntegration)
	registerSharedPlatformSource(t, "core", capability.ValkeyIntegration)
	registerSharedPlatformSource(t, "other", capability.PostgreSQLIntegration)
	registerSharedPlatformSource(t, "other", capability.ValkeyIntegration)

	dataDir := t.TempDir()
	if err := setupSharedBackendDir(t, dataDir, "core"); err != nil {
		t.Fatal(err)
	}

	if err := DestroyAllSharedBackendsAt(context.Background(), noopRuntimeEmbeds{}, dataDir, "orphan"); err != nil {
		t.Fatal(err)
	}

	after := sharedPlatformCounts(t, "environment:other")
	if after["postgresql"] != 1 || after["valkey"] != 1 {
		t.Fatalf("foreign platform sources were wiped by shared destroy (cross-tenant, both providers): %#v", after)
	}
}

func TestSharedDestroyKeepsEnvironmentForRegistryPruneRetry(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	registerSharedPlatformSource(t, "core", capability.PostgreSQLIntegration)
	registerSharedPlatformSource(t, "core", capability.ValkeyIntegration)

	dataDir := t.TempDir()
	if err := setupSharedBackendDir(t, dataDir, "core"); err != nil {
		t.Fatal(err)
	}
	// Let the PostgreSQL prune succeed, then inject a Valkey failure. This is
	// the partial-progress case that used to lose the environment identity.
	valkeyFailure := errors.New("injected valkey prune failure")
	prune := func(provider capability.ProviderKind, class observability.SourceClass, scope capability.ProviderScope, boundary string, keepIDs map[string]struct{}) error {
		if provider == capability.ProviderValkey {
			return valkeyFailure
		}
		return observability.PruneOwnedPlatformSources(provider, class, scope, boundary, keepIDs)
	}
	if err := destroyAllSharedBackendsAt(context.Background(), noopRuntimeEmbeds{}, dataDir, "orphan", prune); !errors.Is(err, valkeyFailure) {
		t.Fatalf("expected injected Valkey prune failure, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "providers", "shared-backends", "core")); err != nil {
		t.Fatalf("environment identity removed after partial registry prune: %v", err)
	}
	if after := sharedPlatformCounts(t, "environment:core"); len(after) != 1 || after["valkey"] != 1 {
		t.Fatalf("expected successful PostgreSQL prune and retained Valkey source before retry, got %#v", after)
	}
	if err := DestroyAllSharedBackendsAt(context.Background(), noopRuntimeEmbeds{}, dataDir, "orphan"); err != nil {
		t.Fatalf("retry after registry recovery: %v", err)
	}
	if after := sharedPlatformCounts(t, "environment:core"); len(after) != 0 {
		t.Fatalf("retry left owned sources behind: %#v", after)
	}
}

// setupSharedBackendDir writes one environment directory (with state.json +
// compose.yaml) so DestroyAllSharedBackendsAt processes it.
func setupSharedBackendDir(t *testing.T, dataDir, environment string) error {
	t.Helper()
	root := filepath.Join(filepath.Clean(dataDir), "providers", "shared-backends")
	dir := filepath.Join(root, environment)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		return err
	}
	return nil
}
