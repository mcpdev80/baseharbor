package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// TestTenantScopeErrorRefusedAgainstExistingCoreOwner is the strict negative
// regression for the pre-bypass bypass: a tenant target whose enrolled access
// scope is corrupt/unreadable MUST be refused on the real Save/Load write seam
// even when a valid Core-owner already governs the Target. The old code
// (dea01b83 and earlier) translated that scope error into (false, nil) and the
// fall-through single-installation-owner check admitted the deployment — the
// bypass. The hook now propagates (false, scopeErr) so the seam surfaces the
// real error.
//
// The assertion is strict on three axes the earlier hook-only tests (which
// only checked `err != nil` at a *missing* owner) did not cover:
//   - a real Core-owner IS present, so the missing-owner fall-through cannot be
//     what refuses the write — only the scope error can;
//   - the error is a *specific* connector-scope error, not a nil pass or a
//     generic "owner missing";
//   - no deployment state is written and Load is rejected with the same error.
func TestTenantScopeErrorRefusedAgainstExistingCoreOwner(t *testing.T) {
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
		Name:          "tenant-corrupt-scope",
		Environment:   "dev",
	}
	identity := deployment.DeploymentIdentity{
		DeploymentID:  "22222222-2222-4222-8222-222222222222",
		ApplicationID: manifest.ApplicationID,
		Target:        tenant,
		Application:   manifest.Name,
		Environment:   manifest.Environment,
	}

	// Start with a GUELTIGEM (valid) enrolled scope: a baseharbor-node-connector
	// access reference grants through the grant-only TenantOwnerResolver hook so
	// the tenant is admitted on the write seam even with an existing Core-owner.
	// A real record is saved here first so the byte comparison below has a real
	// baseline: the corrupt-scope regression can only prove "bytes unchanged" if
	// a valid record was actually persisted before the scope was corrupted. This
	// is the setup the earlier version lacked (it asserted the file was absent
	// and then loaded that absent record, which could not distinguish a
	// fail-closed write seam from a silent one).
	cfgDoc := deployment.Config{
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
	if err := cfgDoc.Save(); err != nil {
		t.Fatal(err)
	}

	// Provision a REAL Core-owner on the Target's runtime root so the
	// missing-owner fall-through in VerifyDeploymentOwner CANNOT be what
	// refuses the write. The scope error is the sole gate. Without this, the
	// assertion below would pass for the wrong reason.
	installRoot := filepath.Join(targetRuntimeRoot(t, tenant))
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCoreInstallation(t, installRoot, tenant); err != nil {
		t.Fatal(err)
	}

	validRecord := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "podman"},
	}
	// Save on the real write seam must be admitted for a valid enrolled scope.
	if err := deployment.SaveDeploymentRecord(validRecord); err != nil {
		t.Fatalf("valid-scope tenant must be admitted on the write seam: %v", err)
	}
	statePath := deploymentStatePath(t, tenant, identity.DeploymentID)
	validBytes, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read back the valid-scope record bytes: %v", readErr)
	}
	if len(validBytes) == 0 {
		t.Fatal("valid-scope record wrote no bytes to the write seam")
	}

	// Now corrupt the enrolled access scope: a non-connector provider makes
	// connectorScopeForTarget fail with a real connector-scope error, which the
	// TenantOwnerResolver hook propagates as a genuine rejection (never a silent
	// fall-through admitted by the existing Core-owner).
	cfgDoc.Access["connector"] = deployment.AccessDefinition{Provider: "docker", Reference: "node-corrupt"}
	if err := cfgDoc.Save(); err != nil {
		t.Fatalf("rewrite config to corrupt scope: %v", err)
	}

	// Save on the real write seam must now be refused with a tenant-scope-chain
	// error, never admitted through the existing core-owner, and MUST NOT
	// overwrite the valid record that is already on disk.
	saveErr := deployment.SaveDeploymentRecord(validRecord)
	assertRejectionInChain(t, "Save (corrupt scope)", saveErr)
	if _, statErr := os.Stat(statePath); statErr != nil && !os.IsNotExist(statErr) {
		t.Fatalf("corrupt-scope save should leave the existing state in place: %v", statErr)
	}
	corruptedBytes, readErr2 := os.ReadFile(statePath)
	if readErr2 != nil {
		t.Fatalf("read back the record after the refused corrupt-scope save: %v", readErr2)
	}
	if !bytes.Equal(validBytes, corruptedBytes) {
		t.Fatalf("corrupt-scope save mutated the valid record on disk: valid=%q corrupt=%q", validBytes, corruptedBytes)
	}

	// Load on the same seam must also be refused with a tenant-scope-chain error,
	// still leaving the valid record bytes untouched.
	_, loadErr := deployment.LoadDeploymentRecord(identity)
	assertRejectionInChain(t, "Load (corrupt scope)", loadErr)
	afterBytes, readErr3 := os.ReadFile(statePath)
	if readErr3 != nil {
		t.Fatalf("read back the record after the refused corrupt-scope load: %v", readErr3)
	}
	if !bytes.Equal(validBytes, afterBytes) {
		t.Fatalf("corrupt-scope load mutated the valid record on disk: valid=%q after=%q", validBytes, afterBytes)
	}
}

// TestValidTenantScopeGrantedAgainstExistingCoreOwner is the companion positive
// control: a tenant with a valid enrolled scope is admitted on the same seam
// even with the same existing Core-owner present (the grant-only hook
// authorizes the Coreless tenant; the single-installation-owner requirement
// does not apply). It proves the negative case above is not just refusing
// everything.
func TestValidTenantScopeGrantedAgainstExistingCoreOwner(t *testing.T) {
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
		Name:          "tenant-valid-scope",
		Environment:   "dev",
	}
	identity := deployment.DeploymentIdentity{
		DeploymentID:  "33333333-3333-4333-8333-333333333333",
		ApplicationID: manifest.ApplicationID,
		Target:        tenant,
		Application:   manifest.Name,
		Environment:   manifest.Environment,
	}

	// A node-connector access reference with a valid enrollment scope grants
	// through the grant-only hook.
	cfgDoc := deployment.Config{
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
	if err := cfgDoc.Save(); err != nil {
		t.Fatal(err)
	}

	// Same existing Core-owner as in the negative case.
	installRoot := filepath.Join(targetRuntimeRoot(t, tenant))
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCoreInstallation(t, installRoot, tenant); err != nil {
		t.Fatal(err)
	}

	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "podman"},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatalf("valid tenant scope was refused against an existing Core-owner: %v", err)
	}
	if _, err := deployment.LoadDeploymentRecord(identity); err != nil {
		t.Fatalf("admitted valid-scope tenant not loadable through the seam: %v", err)
	}
}

// targetRuntimeRoot resolves the Target's Core installation runtime root
// (targets/<target>/runtime), matching installationRuntimeRoot in
// target_test_helpers_test.go.
func targetRuntimeRoot(t *testing.T, target string) string {
	t.Helper()
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "runtime")
}

// writeCoreInstallation persists a valid Core installation.json on the given
// runtime root. It mirrors the on-disk shape coreinstallation.Load accepts
// (version, owner, v4 installation ID, shared placement, ready phase with the
// three mandatory capabilities). The file is written 0600 so the protected
// regular-file check passes.
func writeCoreInstallation(t *testing.T, installRoot, target string) error {
	t.Helper()
	state := map[string]interface{}{
		"version":         "baseharbor.core-installation/v1",
		"installation_id": "44444444-4444-4444-8444-444444444444",
		"owner":           "baseharbor",
		"spec":            map[string]interface{}{"target": target, "runtime": "docker", "machine_role": "development", "ha": false},
		"defaults":        map[string]interface{}{"local_workspaces": true, "explicit_sources": false, "protected_management": false},
		"placement":       "shared",
		"phase":           "ready",
		"ready":           true,
		"capabilities":    map[string]interface{}{"sql": true, "secrets": true, "identity": true},
		"identity_issuer": "https://identity.localhost:8443/realms/baseharbor",
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	path := filepath.Join(installRoot, "installation.json")
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	// Sanity: it must load as a valid owner here.
	if _, err := coreinstallation.Load(installRoot); err != nil {
		return err
	}
	return nil
}

// unwrapDeploymentError unwraps a *deployment.DeploymentRecordStateError (the
// seam wrapper) to the underlying owner/scope error for error-kind checks.
func unwrapDeploymentError(err error) error {
	var stateErr *deployment.DeploymentRecordStateError
	if errors.As(err, &stateErr) {
		return stateErr.Err
	}
	return err
}

// assertRejectionInChain verifies that the seam produced a genuine rejection and
// that it belongs to the tenant-scope chain for THIS concrete fixture, not a
// nil pass and not an unrelated/soft error.
//
// The corrupt-scope fixture (TestTenantScopeErrorRefusedAgainstExistingCoreOwner)
// provisions a REAL Core-owner on the Target runtime root so that
// VerifyDeploymentOwner reaches the authoritative installation check and the
// ONLY gate that can refuse the write is the connector-scope resolution for the
// corrupt Access reference. That resolution propagates exactly
// targetsession.ErrUnavailable ("authenticated Connector session is
// unavailable") — never ErrDeploymentOwnerMissing (an owner IS present) and
// never ErrDeploymentOwnerForeign (the installation is valid, not rewritten).
//
// The check is therefore strict on BOTH sides: it fails on nil (no soft pass on
// admission) and requires errors.Is(err, targetsession.ErrUnavailable). A nil
// error or any error that is not ErrUnavailable fails the test. This closes the
// "test passes on nil" (and "test passes on the wrong rejection reason")
// anti-pattern the mission forbids.
func assertRejectionInChain(t *testing.T, ctx string, got error) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: expected an ErrUnavailable rejection, got nil (admitted)", ctx)
	}
	cause := unwrapDeploymentError(got)
	if !errors.Is(cause, targetsession.ErrUnavailable) {
		t.Fatalf("%s: rejection %v is not targetsession.ErrUnavailable", ctx, got)
	}
}
