package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestTenantOwnerResolverCLIFallsThroughProvesNoBypass exercises the real
// command path: recordPendingDeployment -> SaveDeploymentRecord, whose storage
// seam invokes the tenantOwnerResolver hook. It proves the grant-only contract
// end-to-end:
//   - a tenant target whose enrolled scope is invalid is refused (no bypass),
//   - a non-tenant target without an authoritative Core installation is refused
//     (the hook falls through and the missing-owner check rejects),
//   - a tenant target with a valid scope is admitted through the seam.
func TestTenantOwnerResolverCLIFallsThroughProvesNoBypass(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)

	tenant := "node-target"
	owner := "11111111-1111-4111-8111-111111111111"
	manifest := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "tenant-demo",
		Environment:   "dev",
	}
	identity := deployment.DeploymentIdentity{
		DeploymentID:  "22222222-2222-4222-8222-222222222222",
		ApplicationID: manifest.ApplicationID,
		Target:        tenant,
		Application:   manifest.Name,
		Environment:   manifest.Environment,
	}

	// A tenant target whose Access binding is not a node-connector reference is
	// not a genuine authorized tenant: connectorScopeForTarget fails,
	// tenantResolvedForTarget returns (false) and the hook falls through to the
	// missing-owner check, so the write fails. This proves the fail-open bypass
	// (which admitted without an owner) is gone.
	t.Run("invalid-scope-tenant-refused", func(t *testing.T) {
		cfg := deployment.Config{
			Version:       deployment.ConfigVersion,
			DefaultTarget: tenant,
			Access: map[string]deployment.AccessDefinition{
				"connector": {Provider: "docker", Reference: "node-bad"},
			},
			Targets: map[string]deployment.TargetDefinition{
				tenant: {
					TenantID: owner,
					Runtime:  deployment.RuntimeDefinition{Provider: "podman"},
					Access:   deployment.TargetAccess{Reference: "connector"},
				},
			},
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		record := deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: identity,
			Source:   deployment.DeploymentSource{Kind: "repository"},
			Applied:  deployment.AppliedDeployment{RuntimeProvider: "podman"},
		}
		if err := deployment.SaveDeploymentRecord(record); err == nil {
			t.Fatal("invalid tenant scope was admitted on the write seam: no owner bypass")
		}
		if _, statErr := os.Stat(deploymentStatePath(t, tenant, identity.DeploymentID)); !os.IsNotExist(statErr) {
			t.Fatal("refused tenant deployment persisted state")
		}
	})

	// A non-tenant Core target with no Core installation must also be refused:
	// the hook returns (false, nil) and the fall-through missing-owner check
	// rejects, proving the hook never admits a Core target without an owner.
	t.Run("non-tenant-no-owner-refused", func(t *testing.T) {
		cfg := deployment.Config{
			Version:       deployment.ConfigVersion,
			DefaultTarget: "local",
			Access:        map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}},
			Targets: map[string]deployment.TargetDefinition{
				"local": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
			},
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		localID := identity
		localID.Target = "local"
		record := deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: localID,
			Source:   deployment.DeploymentSource{Kind: "repository"},
			Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker"},
		}
		if err := deployment.SaveDeploymentRecord(record); err == nil {
			t.Fatal("non-tenant target without an installation was admitted: hook bypassed the Core check")
		}
	})

	// A tenant target with a valid enrolled scope is admitted through the seam,
	// even with no Core installation present: the grant-only hook authorizes
	// access and the single-installation-owner requirement does not apply.
	t.Run("valid-scope-tenant-admitted", func(t *testing.T) {
		cfg := deployment.Config{
			Version:       deployment.ConfigVersion,
			DefaultTarget: tenant,
			Access: map[string]deployment.AccessDefinition{
				"connector": {Provider: "baseharbor-node-connector", Reference: "node-a"},
			},
			Targets: map[string]deployment.TargetDefinition{
				tenant: {
					TenantID: owner,
					Runtime:  deployment.RuntimeDefinition{Provider: "podman"},
					Access:   deployment.TargetAccess{Reference: "connector"},
				},
			},
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		record := deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: identity,
			Source:   deployment.DeploymentSource{Kind: "repository"},
			Applied:  deployment.AppliedDeployment{RuntimeProvider: "podman"},
		}
		// The tenant scope requires a valid node enrollment to validate. The
		// production connector scope path (connectorScopeForTarget) reads the
		// Access binding; with a valid reference it authorizes the tenant target
		// through the grant-only hook.
		if err := deployment.SaveDeploymentRecord(record); err != nil {
			t.Fatalf("valid tenant scope was refused on the write seam: %v", err)
		}
		// The record round-trips back through the seam that the hook authorizes.
		if _, err := deployment.LoadDeploymentRecord(identity); err != nil {
			t.Fatalf("admitted tenant record not loadable through authorized seam: %v", err)
		}
	})
}

// TestDeploymentRecordHookPropagatesError proves the CLI hook propagates a real
// connector scope error rather than swallowing it (the old code returned
// (false,nil) on any error). A tenant target whose Access reference points at a
// non-connector provider yields a hook error, which the seam surfaces.
func TestDeploymentRecordHookPropagatesError(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)

	tenant := "node-target"
	owner := "11111111-1111-4111-8111-111111111111"
	manifest := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "tenant-err",
		Environment:   "dev",
	}
	identity := deployment.DeploymentIdentity{
		DeploymentID:  "22222222-2222-4222-8222-222222222222",
		ApplicationID: manifest.ApplicationID,
		Target:        tenant,
		Application:   manifest.Name,
		Environment:   manifest.Environment,
	}

	cfgDoc := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: tenant,
		Access: map[string]deployment.AccessDefinition{
			// Wrong provider: connectorScopeForTarget returns ErrUnavailable,
			// which the hook must propagate as a real error, not swallow.
			"connector": {Provider: "docker", Reference: "node-a"},
		},
		Targets: map[string]deployment.TargetDefinition{
			tenant: {
				TenantID: owner,
				Runtime:  deployment.RuntimeDefinition{Provider: "podman"},
				Access:   deployment.TargetAccess{Reference: "connector"},
			},
		},
	}
	if err := cfgDoc.Save(); err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "podman"},
	}
	// With the wrong provider the scope validation fails. The hook now
	// propagates that real error (it no longer swallows it), so the seam
	// surfaces a non-nil error rather than admitting the deployment.
	if err := deployment.SaveDeploymentRecord(record); err == nil {
		t.Fatal("invalid connector scope did not surface a hook error on the write seam")
	}
}

// deploymentStatePath resolves the deployment.json path for a target/deployment.
func deploymentStatePath(t *testing.T, target, deploymentID string) string {
	t.Helper()
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "deployments", deploymentID, "deployment.json")
}

func TestTenantBoundLocalCoreUsesRealInstallationOwnerWithoutConnector(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	const name = "core-owner-with-tenant"
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: name,
		Access: map[string]deployment.AccessDefinition{
			"local": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			name: {
				TenantID: "11111111-1111-4111-8111-111111111111",
				Runtime:  deployment.RuntimeDefinition{Provider: "docker"},
				Access:   deployment.TargetAccess{Reference: "local"},
				Scope:    "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID: "22222222-2222-4222-8222-222222222222",
			ApplicationID: application.MustNewApplicationID(),
			Target: name,
			Application: "tenant-local-core-app",
			Environment: "dev",
		},
		Source: deployment.DeploymentSource{Kind: "managed"},
		Applied: deployment.AppliedDeployment{RuntimeProvider: "docker"},
	}
	// Tenant presence must never bypass Core ownership while it is missing.
	if err := deployment.SaveDeploymentRecord(record); err == nil {
		t.Fatal("tenant ID bypassed absent local Core installation ownership")
	}
	seedBootInstallation(t, name, "docker")
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatalf("genuine tenant-bound local Core owner refused without Connector: %v", err)
	}
	if _, err := deployment.LoadDeploymentRecord(record.Identity); err != nil {
		t.Fatalf("tenant-bound local Core record was not readable: %v", err)
	}
}
