package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

const testDeploymentID = "22222222-2222-4222-8222-222222222222"

// Only source-contract tests use this fixture. Actual Core readiness and
// bootstrap behavior are exercised separately; no global test bypass exists.
func assumeVerifiedCoreForSourceTest(t *testing.T) {
	t.Helper()
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error { return nil }
	t.Cleanup(func() { applicationCorePrerequisite = previous })
}

func configureTestTarget(t *testing.T) deployment.ResolvedTarget {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "docker-dev",
		Access: map[string]deployment.AccessDefinition{
			"local-docker": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"docker-dev": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-docker"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	// A managed deployment requires its authoritative Core installation to
	// govern the Target; register one so the production deployment-owner binding
	// passes. Coreless, discovery, and read-only decision tests that assert an
	// empty data root must instead use configureTestTargetEmpty (below).
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatal(err)
	}
	return target
}

// configureTestTargetEmpty registers a Target without a Core installation,
// for coreless / discovery / read-only tests that assert no installation-owned
// Target state was created.
func configureTestTargetEmpty(t *testing.T) deployment.ResolvedTarget {
	target := configureTestTarget(t)
	if stateRoot, err := deployment.TargetStateRoot(target.Name); err == nil {
		// A removal failure or unresolved target root is a setup error, not a
		// benign no-op: the test asserts a specific empty data root, so a
		// swallowed error would silently leave installation-owned state behind
		// and let the assertion pass on the wrong filesystem. Fail closed.
		if err := os.RemoveAll(stateRoot); err != nil {
			t.Fatalf("remove installation-owned state for %s: %v", target.Name, err)
		}
	} else {
		t.Fatalf("resolve target root for empty fixture: %v", err)
	}
	return target
}

// configureTestTargetWithOwner is retained for callers that register the
// authoritative Core installation explicitly; configureTestTarget now provisions
// an owner by default, so it is equivalent but documented for clarity.
func configureTestTargetWithOwner(t *testing.T) deployment.ResolvedTarget {
	return configureTestTarget(t)
}

// registerInstallationForTarget persists a valid Core installation under the
// Target's runtime root (targets/<target>/runtime) if one is not already
// present, so deployment-owner binding passes for managed-deployment tests.
func registerInstallationForTarget(t *testing.T, targetName string) error {
	t.Helper()
	installRoot, err := installationRuntimeRoot(t, targetName)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(filepath.Join(installRoot, "installation.json")); !os.IsNotExist(statErr) {
		return statErr
	}
	_, runErr := coreinstallation.Run(context.Background(), installRoot, coreinstallation.Spec{Target: targetName, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	})
	return runErr
}

// installationRuntimeRoot resolves the Core installation runtime root
// (XDG_DATA_HOME/baseharbor/targets/<target>/runtime), the path
// deployment.TargetStateRoot derives (with the shared "baseharbor/" prefix).
func installationRuntimeRoot(t *testing.T, targetName string) (string, error) {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		return "", err
	}
	return filepath.Join(stateRoot, "runtime"), nil
}

func registerTestDeployment(t *testing.T, target deployment.ResolvedTarget, m application.Manifest, repositoryRoot, manifestPath string) deployment.DeploymentRecord {
	t.Helper()
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  testDeploymentID,
			ApplicationID: m.ApplicationID,
			Target:        target.Name,
			Application:   m.Name,
			Environment:   m.Environment,
		},
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: repositoryRoot,
			Manifest:   manifestPath,
		},
		Applied: deployment.AppliedDeployment{
			Intent:          intent,
			RuntimeProvider: target.RuntimeProvider,
		},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func testDeploymentStore(t *testing.T, target deployment.ResolvedTarget, m application.Manifest) application.Store {
	t.Helper()
	identity := deployment.DeploymentIdentity{
		DeploymentID:  testDeploymentID,
		ApplicationID: m.ApplicationID,
		Target:        target.Name,
		Application:   m.Name,
		Environment:   m.Environment,
	}
	root, err := deployment.DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Applied:  deployment.AppliedDeployment{RuntimeProvider: target.RuntimeProvider},
	}); err != nil {
		t.Fatal(err)
	}
	return application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}
}
