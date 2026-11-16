package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// TestDeploymentOwnerMalformedStateIsNotForeign guards finding 3: a Core
// installation that LOADS but is malformed/invalid (invalid JSON, a missing
// mandatory capability, inconsistent readiness) is its own fail-closed named
// error ErrDeploymentOwnerMalformedState, never ErrDeploymentOwnerForeign.
// The one-installation-owner rule only proves foreignness from a POSITIVE
// loaded identity whose Target diverges; it never proves foreignness from a
// state it cannot even validate.
func TestDeploymentOwnerMalformedStateIsNotForeign(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	identity := testDeploymentIdentity(t, target, "app-malformed", "dev")
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("save against owned target failed: %v", err)
	}
	installPath := filepath.Join(data, "baseharbor", "targets", target, "runtime", "installation.json")

	// Corrupt the owner installation.json so coreinstallation.Load fails on
	// content validation (invalid JSON => "invalid Core installation state"),
	// a malformed-state signal, not a missing or foreign one.
	corrupt := "{ not valid json"
	if err := os.WriteFile(installPath, []byte(corrupt), 0o600); err != nil {
		t.Fatalf("corrupt owner installation: %v", err)
	}

	// The seam must fail closed with ErrDeploymentOwnerMalformedState, never
	// ErrDeploymentOwnerForeign (the rule never positively observed a foreign
	// identity) and never ErrDeploymentOwnerMissing (the file is present).
	ownerErr := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), identity.DeploymentID)
	if ownerErr == nil {
		t.Fatal("malformed owner was admitted on the seam")
	}
	if errors.Is(ownerErr, ErrDeploymentOwnerForeign) {
		t.Fatalf("malformed owner must NOT be classified as foreign: %#v", ownerErr)
	}
	if errors.Is(ownerErr, ErrDeploymentOwnerMissing) {
		t.Fatalf("malformed owner must NOT be classified as missing: %#v", ownerErr)
	}
	if !errors.Is(ownerErr, ErrDeploymentOwnerMalformedState) {
		t.Fatalf("malformed owner must be ErrDeploymentOwnerMalformedState, got: %#v", ownerErr)
	}

	// The public list seam propagates the same distinction.
	if _, listErr := ListDeployments(target); listErr == nil {
		t.Fatal("ListDeployments returned silently for a malformed owner")
	} else if errors.Is(listErr, ErrDeploymentOwnerForeign) {
		t.Fatalf("list must NOT report a malformed owner as foreign: %#v", listErr)
	} else if !errors.Is(listErr, ErrDeploymentOwnerMalformedState) {
		t.Fatalf("list must be ErrDeploymentOwnerMalformedState, got: %#v", listErr)
	}
}

// TestDeploymentOwnerGenuineForeignStaysForeign after a malformed-state guard
// remains in force: a positively-loaded installation whose divergent Target is
// a genuine foreign owner and MUST still be ErrDeploymentOwnerForeign. This
// proves the malformed-state guard does not over-close and swallow the real
// foreign case.
func TestDeploymentOwnerGenuineForeignStaysForeign(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	if err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), "11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatalf("authoritative installation should own its deployment: %v", err)
	}
	installPath := filepath.Join(data, "baseharbor", "targets", target, "runtime", "installation.json")
	raw, readErr := os.ReadFile(installPath)
	if readErr != nil {
		t.Fatalf("read installation to foreign-ize: %v", readErr)
	}
	var state coreinstallation.State
	if err := unmarshalInstallation(raw, &state); err != nil {
		t.Fatalf("parse installation: %v", err)
	}
	state.Spec.Target = "foreign-target"
	out, marshalErr := marshalInstallation(state)
	if marshalErr != nil {
		t.Fatalf("re-encode installation: %v", marshalErr)
	}
	if writeErr := os.WriteFile(installPath, out, 0o600); writeErr != nil {
		t.Fatalf("write foreign installation: %v", writeErr)
	}
	if err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("loaded-but-foreign installation must stay foreign, got: %#v", err)
	}
}
