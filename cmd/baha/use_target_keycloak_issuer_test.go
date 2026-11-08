package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// materializeKeycloakCoreIssuerForTargetWithIdentity registers a verified Core
// under the given name with the given identity issuer and materializes its
// Keycloak core issuer under the plain target-state root so VerifyCoreConnection
// observes the genuine Keycloak-bound discovery identity from disk.
func materializeKeycloakCoreIssuerForTargetWithIdentity(t *testing.T, name, identityIssuer string) {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatalf("resolve target state root: %v", err)
	}
	runtimeRoot := filepath.Join(stateRoot, "runtime")
	if _, statErr := os.Stat(filepath.Join(runtimeRoot, "installation.json")); os.IsNotExist(statErr) {
		if err := os.MkdirAll(runtimeRoot, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := coreinstallation.Run(context.Background(), runtimeRoot, coreinstallation.Spec{Target: name, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
			Preflight: func(context.Context, coreinstallation.State) error { return nil },
			SQL:       func(context.Context, coreinstallation.State) error { return nil },
			Secrets:   func(context.Context, coreinstallation.State) error { return nil },
			Identity:  func(context.Context, coreinstallation.State) (string, error) { return identityIssuer, nil },
			Verify:    func(context.Context, coreinstallation.State) error { return nil },
		}); err != nil {
			t.Fatalf("bootstrap verified Core for %s: %v", name, err)
		}
	}
	materializeKeycloakCoreIssuerForTarget(t, stateRoot, identityIssuer)
}

// TestBindExplicitSelectionResolvesKeycloakCoreIssuer proves the r2 fix: the
// verified Core's Keycloak-bound discovery issuer is the authoritative
// management connection, so bindExplicitSelectionToConnection SUCCEEDS when the
// Core's real Keycloak issuer (providers/keycloak/shared/core) is present on
// disk — the production location the real Core materializes, NOT a local base CA.
// This closes the r1 bug: use/activate verified the Core through a local base CA
// that the real Core never materializes, so it rejected the genuine Core with
// ErrForeignAuthority ("foreign management connection authority").
func TestBindExplicitSelectionResolvesKeycloakCoreIssuer(t *testing.T) {
	target := configureTestTarget(t)
	materializeKeycloakCoreIssuerForTargetWithIdentity(
		t,
		target.Name,
		"https://identity.localhost:8443/realms/baseharbor",
	)
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err != nil {
		t.Fatalf("explicit selection on a verified Core with a Keycloak core issuer must bind: %v", err)
	}
}

// TestBindExplicitSelectionFailsClosedOnForeignIssuerIdentity proves the negative
// control: when the disk-resolved Keycloak core issuer identity does not match
// the verified Core's own IdentityIssuer, bindExplicitSelectionToConnection
// rejects it as foreign (ErrCoreIssuerMismatch), never falling back to a second
// trust source. This is the r1 fix: the old resolver observed a foreign
// identity and let it through; the corrected path proves identity equality.
func TestBindExplicitSelectionFailsClosedOnForeignIssuerIdentity(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(stateRoot, "runtime")
	// The verified Core's IdentityIssuer is the Keycloak-bound identity the Core
	// actually owns (port 8443). The disk material is configured with a DIFFERENT
	// public port (9443), so the disk-resolved identity differs from
	// state.IdentityIssuer — a foreign issuer identity, not the Core's own.
	if _, statErr := os.Stat(filepath.Join(runtimeRoot, "installation.json")); os.IsNotExist(statErr) {
		if _, err := coreinstallation.Run(context.Background(), runtimeRoot, coreinstallation.Spec{Target: target.Name, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
			Preflight: func(context.Context, coreinstallation.State) error { return nil },
			SQL:       func(context.Context, coreinstallation.State) error { return nil },
			Secrets:   func(context.Context, coreinstallation.State) error { return nil },
			Identity: func(context.Context, coreinstallation.State) (string, error) {
				return "https://identity.localhost:8443/realms/baseharbor", nil
			},
			Verify: func(context.Context, coreinstallation.State) error { return nil },
		}); err != nil {
			t.Fatalf("bootstrap verified Core for %s: %v", target.Name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, "providers", "keycloak", "shared", "core", "native-tls", "pki"), 0700); err != nil {
		t.Fatal(err)
	}
	writeProtectedEnvForTest(filepath.Join(stateRoot, "providers", "keycloak", "shared", "core", "runtime.env"), map[string]string{
		"BASEHARBOR_KEYCLOAK_PUBLIC_PORT": "9443",
	})
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("foreign Keycloak core issuer identity must be rejected as foreign")
	}
}

// TestBindExplicitSelectionFailsClosedOnMissingKeycloakIssuer proves the
// separation: a verified Core that holds no Keycloak core issuer on disk fails
// closed with a named sentinel, never fabricating a trust source.
func TestBindExplicitSelectionFailsClosedOnMissingKeycloakIssuer(t *testing.T) {
	target := configureTestTarget(t)
	// No Keycloak core issuer materialized: the real Core's identity is absent.
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("verified Core without a Keycloak core issuer must fail closed")
	}
}
