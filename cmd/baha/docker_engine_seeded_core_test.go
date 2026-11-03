package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestDockerBindingAcceptsSeededCoreOwnerRejectsLegacy probes the state-root
// docker-binding guard (validateTargetDockerBinding) pair-wise: a validly
// seeded Core installation (authoritative owner) is accepted as a bound Core
// for the Target, while an unbound legacy runtime dir (compose.yaml, no Core
// owner) stays rejected. Both cases run in the same test so the acceptance and
// the fail-closed boundary are verified against one another.
//
// Contract basis: the deployment seam (VerifyDeploymentOwner ->
// coreinstallation.Load(runtime)) already proves the one-installation-owner
// binding, so a genuine Core owner governs the Target regardless of whether the
// docker-engine.json probe landed here first. Foreign/absent Core state keeps
// failing closed.
func TestDockerBindingAcceptsSeededCoreOwnerRejectsLegacy(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	engine := bhruntime.DockerEngineObservation{Endpoint: "unix:///var/run/docker.sock", Mode: "rootful", DaemonID: "seeded-daemon", Verified: true}
	provider := bindingRuntimeFixture{engine: engine}

	// seedCoreOwnerForTarget provisions a genuine authoritative Core
	// installation (coreinstallation.Run) into a specific installRoot, mirroring
	// the internal/application seedManagedTargetOwner fixture.
	seedCoreOwnerForTarget := func(t *testing.T, installRoot string) (coreinstallation.State, error) {
		t.Helper()
		spec := coreinstallation.Spec{Target: "seeded-core", Runtime: "docker", MachineRole: coreinstallation.Development}
		steps := coreinstallation.Steps{
			Preflight: func(context.Context, coreinstallation.State) error { return nil },
			SQL:       func(context.Context, coreinstallation.State) error { return nil },
			Secrets:   func(context.Context, coreinstallation.State) error { return nil },
			Identity: func(context.Context, coreinstallation.State) (string, error) {
				return "https://identity.localhost:8443/realms/baseharbor", nil
			},
			Verify: func(context.Context, coreinstallation.State) error { return nil },
		}
		return coreinstallation.Run(context.Background(), installRoot, spec, steps)
	}

	// --- case A: validly seeded Core owner -> accepted ---
	t.Run("seeded-core-accepted", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		target := deployment.ResolvedTarget{Name: "seeded-core", RuntimeProvider: "docker", AccessProvider: "local"}
		installRoot, _ := targetRuntimeStateRoot(target)
		if err := os.MkdirAll(installRoot, 0700); err != nil {
			t.Fatal(err)
		}
		// Seed a genuine authoritative Core owner exactly as registerInstallationForTarget does.
		if _, err := os.Stat(filepath.Join(installRoot, "installation.json")); os.IsNotExist(err) {
			if _, runErr := seedCoreOwnerForTarget(t, installRoot); runErr != nil {
				t.Fatalf("seed Core owner: %v", runErr)
			}
		}
		if err := validateTargetDockerBinding(context.Background(), target, provider, true); err != nil {
			t.Fatalf("validly seeded Core owner must be accepted as bound Core: %v", err)
		}
	})

	// --- case B: unbound legacy runtime, no Core owner -> rejected ---
	t.Run("legacy-core-rejected", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		target := deployment.ResolvedTarget{Name: "legacy-core", RuntimeProvider: "docker", AccessProvider: "local"}
		root, _ := targetRuntimeStateRoot(target)
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		// A leftover compose.yaml without a Core installation is "existing Core
		// state without a Docker engine binding" with no proven owner.
		if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("protected-original-core"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validateTargetDockerBinding(context.Background(), target, provider, true); err == nil {
			t.Fatal("unproven legacy runtime must stay rejected (no Core owner)")
		}
	})
}
