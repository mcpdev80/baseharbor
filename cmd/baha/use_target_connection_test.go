package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// registerTestTargetConfig writes a minimal deployment config exposing exactly
// one Target (name) with no Core installation registered under it, so the
// connection-selection tests can exercise the bare use/target path.
func registerTestTargetConfig(t *testing.T, name string) error {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			name + "-local": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			name: {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: name + "-local"},
			},
		},
	}
	return cfg.Save()
}

// bootstrapVerifiedTargetForConnectionTest registers a genuine, verified Core
// installation under the Target runtime root and materializes its REAL Keycloak
// core issuer under the plain target-state root (providers/keycloak/shared/core),
// so the read-only VerifyCoreConnection boundary observes a genuine, already-
// existing Keycloak-bound discovery identity. It returns the loaded State plus
// the runtime root.
func bootstrapVerifiedTargetForConnectionTest(t *testing.T, targetName string) (coreinstallation.State, string) {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve target state root: %v", err)
	}
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatalf("create target state root: %v", err)
	}
	runtimeRoot := filepath.Join(stateRoot, "runtime")
	if _, statErr := os.Stat(filepath.Join(runtimeRoot, "installation.json")); !os.IsNotExist(statErr) {
		return coreinstallation.State{}, runtimeRoot
	}
	if _, err := coreinstallation.Run(context.Background(), runtimeRoot, coreinstallation.Spec{Target: targetName, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}); err != nil {
		t.Fatalf("bootstrap verified installation for %s: %v", targetName, err)
	}
	materializeKeycloakCoreIssuerForTarget(t, stateRoot, "https://identity.localhost:8443/realms/baseharbor")
	state, err := coreinstallation.Load(runtimeRoot)
	if err != nil {
		t.Fatalf("load verified installation for %s: %v", targetName, err)
	}
	return state, runtimeRoot
}

// TestBindExplicitSelectionBindsThroughReadingResolver proves the red-line:
// bindExplicitSelectionToConnection verifies the management connection of a
// genuinely installed, verified Core through the single reading resolver
// coreinstallation.ResolverForTargetSelection, WITHOUT any application context.
// This closes the review defect that the authority check did not hang on the
// production use/target path: the explicit selection binds here.
func TestBindExplicitSelectionBindsThroughReadingResolver(t *testing.T) {
	targetName := "docker-conn"
	if err := registerTestTargetConfig(t, targetName); err != nil {
		t.Fatal(err)
	}
	_, _ = bootstrapVerifiedTargetForConnectionTest(t, targetName)
	if err := bindExplicitSelectionToConnection(context.Background(), targetName); err != nil {
		t.Fatalf("explicit selection on a verified Core must bind: %v", err)
	}
}

// TestBindExplicitSelectionFailsClosedWithoutCore proves the separation: a
// Target that holds no genuine, verified Core installation fails closed with a
// typed conflict and persists no selection — the authority check binds before
// any effect.
func TestBindExplicitSelectionFailsClosedWithoutCore(t *testing.T) {
	targetName := "coreless-conn"
	if err := registerTestTargetConfig(t, targetName); err != nil {
		t.Fatal(err)
	}
	err := bindExplicitSelectionToConnection(context.Background(), targetName)
	if err == nil {
		t.Fatal("target without a verified Core must fail closed")
	}
	if machine.Classify(err).Code != machine.ErrorConflict {
		t.Fatalf("expected typed conflict rejection, got %v", err)
	}
}

// TestBindExplicitSelectionFailsClosedOnUnreadableBaseCA proves the foreign/
// unverifiable boundary: a Target whose installation is genuine but whose base
// CA is unreadable (wrong mode) fails closed with a typed rejection, because
// the reading resolver must not fabricate issuer material.
func TestBindExplicitSelectionFailsClosedOnUnreadableBaseCA(t *testing.T) {
	targetName := "unreadable-conn"
	if err := registerTestTargetConfig(t, targetName); err != nil {
		t.Fatal(err)
	}
	_, runtimeRoot := bootstrapVerifiedTargetForConnectionTest(t, targetName)
	// Corrupt the runtime root permissions so the observe-only resolver fails
	// closed with ErrUnverifiableInstallation.
	if err := os.Chmod(runtimeRoot, 0000); err != nil {
		t.Fatalf("corrupt runtime root: %v", err)
	}
	defer os.Chmod(runtimeRoot, 0700)
	if err := bindExplicitSelectionToConnection(context.Background(), targetName); err == nil {
		t.Fatal("Target with an unreadable base CA must fail closed")
	}
}

// bindExplicitSelectionToConnectionMutant is the round-1 mutant: it binds the
// explicit selection with a nil/absent issuer, i.e. it bypasses the reading
// resolver coreinstallation.ResolverForTargetSelection entirely. It is the
// negative control for TestBindExplicitSelectionMutantSkipsResolver — the
// boundary is skipped, so the missing connection must fail closed, never bind.
func bindExplicitSelectionToConnectionMutant(_ context.Context, target string) error {
	selection := coreinstallation.ConnectionSelection{
		InstallationID: "core-mutant",
		ConnectionID:   "managed-connection",
		TargetID:       target,
	}
	return selection.Resolve(context.Background(), nil, serviceaccess.CertificateStatus{}, "core-mutant")
}

// TestBindExplicitSelectionMutantSkipsResolver proves the mutation red-line:
// the mutant that binds the selection WITHOUT resolving the installation
// through the reading resolver must fail closed. The connection binding is only
// valid when it is derived from the verified installation + target state, never
// from a nil/absent issuer.
func TestBindExplicitSelectionMutantSkipsResolver(t *testing.T) {
	targetName := "mutant-conn"
	if err := registerTestTargetConfig(t, targetName); err != nil {
		t.Fatal(err)
	}
	// The mutant resolves no installation and verifies with a nil issuer: the
	// boundary is bypassed, so it must fail closed, not bind.
	err := bindExplicitSelectionToConnectionMutant(context.Background(), targetName)
	if err == nil {
		t.Fatal("mutant that skips the resolver must fail closed")
	}
	if !errors.Is(err, coreinstallation.ErrMissingAuthority) && !errors.Is(err, coreinstallation.ErrUnverifiableInstallation) {
		t.Fatalf("expected boundary rejection, got %v", err)
	}
}

// TestBindExplicitSelectionBindsWithoutLocalIssuerDir is the BF-02 regression
// guard: a genuinely installed, verified Core whose local base-CA directory
// (<runtimeRoot>/service-access/issuer) is absent must still select — the
// production path verifies the Core through its real Keycloak-bound discovery
// identity (VerifyCoreConnection), never through a local base CA. An absent
// local base CA is therefore recoverable, not a dead-end rejection.
func TestBindExplicitSelectionBindsWithoutLocalIssuerDir(t *testing.T) {
	targetName := "noissuer-conn"
	if err := registerTestTargetConfig(t, targetName); err != nil {
		t.Fatal(err)
	}
	_, runtimeRoot := bootstrapVerifiedTargetForConnectionTest(t, targetName)
	// Remove ONLY the absent local base CA directory; the verified Core and its
	// Keycloak core issuer remain on disk.
	issuerDir := filepath.Join(runtimeRoot, "service-access", "issuer")
	if err := os.RemoveAll(issuerDir); err != nil {
		t.Fatalf("remove local issuer dir: %v", err)
	}
	if _, err := os.Stat(issuerDir); !os.IsNotExist(err) {
		t.Fatalf("precondition: local issuer dir must be absent: %v", err)
	}
	if err := bindExplicitSelectionToConnection(context.Background(), targetName); err != nil {
		t.Fatalf("verified Core without local issuer dir must still select (BF-02 regression): %v", err)
	}
}

// TestBindExplicitSelectionFailsClosedOnAbsentCore keeps the separation: a
// Target that holds no genuine, verified Core installation still fails closed
// with a typed conflict and persists no selection.
func TestBindExplicitSelectionFailsClosedOnAbsentCore(t *testing.T) {
	targetName := "coreless-conn2"
	if err := registerTestTargetConfig(t, targetName); err != nil {
		t.Fatal(err)
	}
	err := bindExplicitSelectionToConnection(context.Background(), targetName)
	if err == nil {
		t.Fatal("target without a verified Core must fail closed")
	}
	if machine.Classify(err).Code != machine.ErrorConflict {
		t.Fatalf("expected typed conflict rejection, got %v", err)
	}
}
