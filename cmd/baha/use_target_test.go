package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// publishBaseCARevocations publishes a revocation-free CRL for the base CA at
// the given issuer dir so the reading resolver's single tri-state revocation
// call observes a genuine, revocation-free list (a missing CRL is fail-closed
// on the consumption path). It revokes a throwaway serial "02" so the observed
// base serial is absent from the list.
func publishBaseCARevocations(t *testing.T, issuerDir string) error {
	t.Helper()
	iss, err := serviceaccess.EnsureLocalIssuer(issuerDir)
	if err != nil {
		return err
	}
	return iss.Revoke(context.Background(), "02")
}

// verifiedCoreForTarget registers a genuine verified Core with a materialized
// Keycloak core issuer under the plain target-state root (providers/keycloak/
// shared/core), returning the loaded State and the runtime root. The real Core
// keeps its management-connection identity at its Keycloak discovery issuer, so
// use/activate verifies the genuine Core identity from disk — NOT a local base
// CA (which the real Core never materializes).
func verifiedCoreForTarget(t *testing.T, name string) (coreinstallation.State, string) {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
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
			Identity: func(context.Context, coreinstallation.State) (string, error) {
				return "https://identity.localhost:8443/realms/baseharbor", nil
			},
			Verify: func(context.Context, coreinstallation.State) error { return nil },
		}); err != nil {
			t.Fatal(err)
		}
	}
	materializeKeycloakCoreIssuerForTarget(t, stateRoot, "https://identity.localhost:8443/realms/baseharbor")
	state, err := coreinstallation.Load(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	return state, runtimeRoot
}

func TestUseTargetCommandRegistered(t *testing.T) {
	root := rootCommand()
	var found bool
	for _, command := range root.Children {
		if command.Name == "use" {
			found = command.Run != nil
			if command.Usage != "baha use [TARGET]" {
				t.Fatalf("unexpected use contract: %q", command.Usage)
			}
			break
		}
	}
	if !found {
		t.Fatal("baha use is not registered")
	}
}

func TestUseTargetPersistsSelectionAndRejectsUnknownTarget(t *testing.T) {
	target := configureTestTarget(t)
	// The shared fixture installs a genuine verified Core; materialize its
	// Keycloak core issuer so the management connection verifies as live on the
	// use/target path from the Core's real discovery issuer (disk), not a local
	// base CA. materializeKeycloakCoreIssuerForTarget takes the plain target
	// state root (deployment.TargetStateRoot).
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	materializeKeycloakCoreIssuerForTarget(t, stateRoot, "https://identity.localhost:8443/realms/baseharbor")
	var out, errOut bytes.Buffer
	command := useTargetCommand()
	if err := command.Run(context.Background(), []string{target.Name}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	selected, err := readPersistedTarget()
	if err != nil || selected != target.Name {
		t.Fatalf("use did not persist target: %q %v", selected, err)
	}
	if err := command.Run(context.Background(), []string{"not-configured"}, &out, &errOut); err == nil {
		t.Fatal("unknown target accepted")
	}
	selected, err = readPersistedTarget()
	if err != nil || selected != target.Name {
		t.Fatal("rejected selection changed active target")
	}
}
