package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSharedValkeyTwoApplicationIsolationDestroy(t *testing.T) {
	if os.Getenv("BASEHARBOR_SHARED_VALKEY_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_SHARED_VALKEY_INTEGRATION=1 to run shared Valkey integration")
	}
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), "shared")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("DetectCompose() error = %v", err)
	}

	root := t.TempDir()
	dataDir := filepath.Join(root, "target-state")
	store := Store{Root: filepath.Join(root, "apps"), Namespace: "shared-valkey-it"}
	issuer := serviceissuer.New(t)

	appA := WithCacheInstances(New("app-a", "dev", false, true, false), "default", "sessions")
	appB := WithCacheInstances(New("app-b", "dev", false, true, false), "default")
	for _, m := range []Manifest{appA, appB} {
		if _, err := store.Create(m); err != nil {
			t.Fatalf("Store.Create(%s) error = %v", m.Name, err)
		}
	}

	for _, m := range []Manifest{appA, appB} {
		if err := ReconcileReferenceProviderRegistryAt(dataDir, m); err != nil {
			t.Fatalf("ReconcileReferenceProviderRegistryAt(%s) error = %v", m.Name, err)
		}
	}

	filesA, err := EnsureRuntime(ctx, issuer, store, appA)
	if err != nil {
		t.Fatalf("EnsureRuntime(appA) error = %v", err)
	}
	filesB, err := EnsureRuntime(ctx, issuer, store, appB)
	if err != nil {
		t.Fatalf("EnsureRuntime(appB) error = %v", err)
	}
	if _, err := ReconcileSharedBackends(ctx, compose, issuer, dataDir, store.Namespace, appA, filesA); err != nil {
		t.Fatalf("ReconcileSharedBackends(appA) error = %v", err)
	}
	defer func() { _ = DestroyAllSharedBackendsAt(context.Background(), compose, dataDir, store.Namespace) }()
	if _, err := ReconcileSharedBackends(ctx, compose, issuer, dataDir, store.Namespace, appB, filesB); err != nil {
		t.Fatalf("ReconcileSharedBackends(appB) error = %v", err)
	}

	if err := VerifySharedValkey(ctx, compose, dataDir, store.Namespace, appA); err != nil {
		t.Fatalf("VerifySharedValkey(appA) error = %v", err)
	}
	if err := VerifySharedValkey(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("VerifySharedValkey(appB) error = %v", err)
	}

	shared := SharedBackendFilesAt(dataDir, store.Namespace, "dev")
	state, err := loadSharedBackendState(shared.State, "dev")
	if err != nil {
		t.Fatalf("loadSharedBackendState() error = %v", err)
	}
	aState := state.Applications[sharedBackendApplicationKey(appA)]
	bState := state.Applications[sharedBackendApplicationKey(appB)]

	setSharedValkeySentinel(t, ctx, compose, shared, appA, "default", aState.Cache["default"], "A-ONLY")
	setSharedValkeySentinel(t, ctx, compose, shared, appA, "sessions", aState.Cache["sessions"], "A-SESSIONS")
	setSharedValkeySentinel(t, ctx, compose, shared, appB, "default", bState.Cache["default"], "B-ONLY")

	if got := readSharedValkeySentinel(t, ctx, compose, shared, appA, "default", aState.Cache["default"]); got != "A-ONLY" {
		t.Fatalf("appA default sentinel = %q", got)
	}
	if got := readSharedValkeySentinel(t, ctx, compose, shared, appA, "sessions", aState.Cache["sessions"]); got != "A-SESSIONS" {
		t.Fatalf("appA sessions sentinel = %q", got)
	}
	if got := readSharedValkeySentinel(t, ctx, compose, shared, appB, "default", bState.Cache["default"]); got != "B-ONLY" {
		t.Fatalf("appB sentinel = %q", got)
	}

	aCredential := filepath.Join(shared.Dir, filepath.FromSlash(aState.Cache["default"].CredentialReference))
	aSessionsCredential := filepath.Join(shared.Dir, filepath.FromSlash(aState.Cache["sessions"].CredentialReference))
	bCredential := filepath.Join(shared.Dir, filepath.FromSlash(bState.Cache["default"].CredentialReference))

	if err := ReleaseSharedBackendApplication(ctx, compose, dataDir, store.Namespace, appA); err != nil {
		t.Fatalf("ReleaseSharedBackendApplication(appA) error = %v", err)
	}
	if err := ReleaseApplicationProviderRegistryAt(dataDir, appA); err != nil {
		t.Fatalf("ReleaseApplicationProviderRegistryAt(appA) error = %v", err)
	}
	state, err = loadSharedBackendState(shared.State, "dev")
	if err != nil {
		t.Fatalf("load state after appA destroy: %v", err)
	}
	if _, exists := state.Applications[sharedBackendApplicationKey(appA)]; exists {
		t.Fatal("appA registration remains after destroy")
	}
	if _, exists := state.Applications[sharedBackendApplicationKey(appB)]; !exists {
		t.Fatal("appB registration was removed by appA destroy")
	}
	for _, path := range []string{aCredential, aSessionsCredential} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("appA credential remains after destroy: %s err=%v", path, err)
		}
	}
	if _, err := os.Stat(bCredential); err != nil {
		t.Fatalf("appB credential was removed by appA destroy: %v", err)
	}
	if err := VerifySharedValkey(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("appB not usable after appA destroy: %v", err)
	}
	if got := readSharedValkeySentinel(t, ctx, compose, shared, appB, "default", bState.Cache["default"]); got != "B-ONLY" {
		t.Fatalf("appB sentinel after appA destroy = %q, want B-ONLY", got)
	}

	// Reconcile the surviving consumer again after appA has been released. This
	// is the regression boundary for phantom reconciliation: stale provider
	// state must never recreate appA's Valkey services.
	if _, err := ReconcileSharedBackends(ctx, compose, issuer, dataDir, store.Namespace, appB, filesB); err != nil {
		t.Fatalf("ReconcileSharedBackends(appB after appA destroy) error = %v", err)
	}
	state, err = loadSharedBackendState(shared.State, "dev")
	if err != nil {
		t.Fatalf("load state after surviving app reconcile: %v", err)
	}
	if _, exists := state.Applications[sharedBackendApplicationKey(appA)]; exists {
		t.Fatal("appA registration was resurrected by later shared backend reconcile")
	}
	if _, exists := state.Applications[sharedBackendApplicationKey(appB)]; !exists {
		t.Fatal("appB registration missing after surviving app reconcile")
	}
	running, err := compose.RunningServicesProject(ctx, shared.Project, shared.Compose, shared.Env)
	if err != nil {
		t.Fatalf("inspect shared Valkey services after surviving app reconcile: %v", err)
	}
	for _, removed := range []string{
		sharedValkeyService(appA, "default"),
		sharedValkeyService(appA, "sessions"),
	} {
		for _, service := range running {
			if service == removed {
				t.Fatalf("appA shared Valkey service %q was resurrected by later reconcile", removed)
			}
		}
	}
	if err := VerifySharedValkey(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("appB not usable after later shared backend reconcile: %v", err)
	}
	if got := readSharedValkeySentinel(t, ctx, compose, shared, appB, "default", bState.Cache["default"]); got != "B-ONLY" {
		t.Fatalf("appB sentinel after later reconcile = %q, want B-ONLY", got)
	}

	if err := ReleaseSharedBackendApplication(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("ReleaseSharedBackendApplication(appB) error = %v", err)
	}
	if err := ReleaseApplicationProviderRegistryAt(dataDir, appB); err != nil {
		t.Fatalf("ReleaseApplicationProviderRegistryAt(appB) error = %v", err)
	}
	state, err = loadSharedBackendState(shared.State, "dev")
	if err != nil {
		t.Fatalf("load state after last consumer destroy: %v", err)
	}
	if len(state.Applications) != 0 {
		t.Fatalf("shared provider applications after last consumer destroy = %d, want 0", len(state.Applications))
	}
	if _, err := os.Stat(bCredential); !os.IsNotExist(err) {
		t.Fatalf("appB credential remains after last consumer destroy: err=%v", err)
	}
}

func setSharedValkeySentinel(t *testing.T, ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, m Manifest, instance string, resource sharedValkeyResource, value string) {
	t.Helper()
	password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
	if err != nil {
		t.Fatalf("read shared Valkey credential: %v", err)
	}
	service := sharedValkeyService(m, instance)
	command := "VALKEYCLI_AUTH=" + shellQuote(password) + " valkey-cli -h 127.0.0.1 -p 6379 SET isolation_probe " + shellQuote(value)
	out, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, service, "sh", "-ec", command)
	if err != nil {
		t.Fatalf("set sentinel in %s/%s: %v", m.Name, instance, err)
	}
	if strings.TrimSpace(out) != "OK" {
		t.Fatalf("set sentinel in %s/%s = %q", m.Name, instance, strings.TrimSpace(out))
	}
}

func readSharedValkeySentinel(t *testing.T, ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, m Manifest, instance string, resource sharedValkeyResource) string {
	t.Helper()
	password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
	if err != nil {
		t.Fatalf("read shared Valkey credential: %v", err)
	}
	service := sharedValkeyService(m, instance)
	command := "VALKEYCLI_AUTH=" + shellQuote(password) + " valkey-cli -h 127.0.0.1 -p 6379 GET isolation_probe"
	out, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, service, "sh", "-ec", command)
	if err != nil {
		t.Fatalf("read sentinel from %s/%s: %v", m.Name, instance, err)
	}
	return strings.TrimSpace(out)
}
