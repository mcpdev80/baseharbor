package application

import (
	"context"
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

// registerSharedPlatformSource writes one platform-scoped observability source
// under a specific SharingBoundary ("environment:<env>"), mirroring how shared
// postgres/valkey register during ensure.
func registerSharedPlatformSource(t *testing.T, env string, provider capability.ProviderKind) {
	t.Helper()
	source := observability.SignalSource{
		ID:              string(provider) + ":shared:" + env,
		Kind:            observability.SignalLogs,
		Provider:        provider,
		Class:           observability.SourcePlatformProvider,
		Scope:           capability.ScopeShared,
		SharingBoundary: "environment:" + env,
		Target:          "postgres",
		Protocol:        "stdout-stderr",
	}
	if err := observability.UpdateSignal(source); err != nil {
		t.Fatalf("register shared platform source env %q: %v", env, err)
	}
}

// TestSharedDestroyRemovesOwnedPlatformSources asserts that DestroyAllSharedBackendsAt
// tears down exactly the platform-scoped observability rows it owns (the one
// environment directory present) and that a following re-registration with a
// fresh owner succeeds.
func TestSharedDestroyRemovesOwnedPlatformSources(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// Two environments own platform-scoped rows; only "core" is on disk.
	registerSharedPlatformSource(t, "core", capability.ProviderPostgreSQL)
	registerSharedPlatformSource(t, "other", capability.ProviderPostgreSQL)

	registryBefore, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: "environment:core"},
		nil, true, true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(registryBefore) != 1 {
		t.Fatalf("expected 1 core platform source before destroy, got %d", len(registryBefore))
	}

	dataDir := t.TempDir()
	if err := setupSharedBackendDir(t, dataDir, "core"); err != nil {
		t.Fatal(err)
	}

	if err := DestroyAllSharedBackendsAt(context.Background(), noopRuntimeEmbeds{}, dataDir, "orphan"); err != nil {
		t.Fatal(err)
	}

	// Owned environment removed.
	afterCore, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: "environment:core"},
		nil, true, true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterCore) != 0 {
		t.Fatalf("owned platform sources not removed by shared destroy: %#v", afterCore)
	}

	// Re-registration with a fresh owner now succeeds for the same boundary.
	registerSharedPlatformSource(t, "core", capability.ProviderPostgreSQL)
	afterRe, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: "environment:core"},
		nil, true, true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRe) != 1 {
		t.Fatalf("re-registration did not recover, got %d sources", len(afterRe))
	}
}

// TestSharedDestroyPreservesForeignPlatformSources is the cross-tenant wipe guard:
// destroying one environment must NOT remove platform-scoped rows owned by a
// different environment. This is the most dangerous variant and must stay green.
func TestSharedDestroyPreservesForeignPlatformSources(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	registerSharedPlatformSource(t, "core", capability.ProviderPostgreSQL)
	registerSharedPlatformSource(t, "other", capability.ProviderPostgreSQL)

	dataDir := t.TempDir()
	if err := setupSharedBackendDir(t, dataDir, "core"); err != nil {
		t.Fatal(err)
	}

	if err := DestroyAllSharedBackendsAt(context.Background(), noopRuntimeEmbeds{}, dataDir, "orphan"); err != nil {
		t.Fatal(err)
	}

	// The foreign environment's row must survive.
	afterOther, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: "environment:other"},
		nil, true, true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterOther) != 1 {
		t.Fatalf("foreign platform source was wiped by shared destroy (cross-tenant): %#v", afterOther)
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
