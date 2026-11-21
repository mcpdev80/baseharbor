package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// 0428-207 regression: coreDefaultNetworkBindingMissingOwnerMessage (the guard's
// refusal renderer at cmd/baha/core_network_binding.go, called from
// inspectCoreDefaultNetworkBinding) must name BOTH halves of the missing
// precondition (protected Core runtime inventory OR shared backend owner) and
// how to satisfy it, instead of the unactionable
// "existing native network lacks a protected Core or shared backend owner".
// This is the read-only round's corrected production site, NOT
// application_provider_impact.go:63.
func Test0428207CoreNetworkBindingGuardNamesMissingPreconditions(t *testing.T) {
	// Missing inventory (os.ErrNotExist): the App-Edit baseline where the
	// isolated Core has never materialized its runtime inventory. The guard
	// (core_network_binding.go:70) selects this wording ONLY when
	// errors.Is(err, os.ErrNotExist); a plain text error never reaches that
	// branch, so pass a wrapped sentinel that the production errors.Is check
	// actually matches.
	missingErr := fmt.Errorf("lstat \"/state/runtime\": no such file or directory: %w", os.ErrNotExist)
	msg := coreDefaultNetworkBindingMissingOwnerMessage(missingErr).Error()
	// (a) names the missing protected Core runtime inventory precondition ...
	if !strings.Contains(msg, "protected Core") {
		t.Fatalf("0428-207: message must name the missing protected Core inventory, got %q", msg)
	}
	// (d) the ENOENT branch (:70-72) names the missing runtime inventory
	// specifically; this phrase exists ONLY on the errors.Is(err, os.ErrNotExist)
	// production branch, so a plain-error test (which never took it) could not
	// have pinned it.
	if !strings.Contains(msg, "runtime inventory is missing") {
		t.Fatalf("0428-207: ENOENT missing-inventory message must name the missing runtime inventory, got %q", msg)
	}
	// ... AND the shared backend owner precondition.
	if !strings.Contains(msg, "shared backend owner") {
		t.Fatalf("0428-207: message must name the missing shared backend owner, got %q", msg)
	}
	// (b) names the check that verifies it (canonical protected owner shape).
	if !strings.Contains(msg, "canonical") {
		t.Fatalf("0428-207: message must name the verification check (canonical protected owner shape), got %q", msg)
	}
	// (c) names the user's next step (materialize the Core, then retry).
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "baha app start") && !strings.Contains(lower, "retry") {
		t.Fatalf("0428-207: message must name the user's next step, got %q", msg)
	}
	// The old unactionable string is replaced; only the message changed, never
	// the fail-closed refusal.
	if strings.Contains(msg, "lacks a protected Core or shared backend owner") {
		t.Fatalf("0428-207: old unactionable message must be replaced, got %q", msg)
	}
	t.Log("0428-207 missing-inventory guard names missing protected Core + shared backend owner, canonical check, and next step")
}

// 0428-207 noncanonical-shape variant: the native control-plane definition
// differs from its canonical protected owner shape (or the shared backend owner
// is absent). It must name BOTH preconditions too.
func Test0428207CoreNetworkBindingGuardNoncanonicalShape(t *testing.T) {
	msg := coreDefaultNetworkBindingMissingOwnerMessage(errors.New("native control-plane definition differs from its canonical owner shape")).Error()
	if !strings.Contains(msg, "protected Core") {
		t.Fatalf("0428-207: noncanonical message must name the missing protected Core, got %q", msg)
	}
	if !strings.Contains(msg, "shared backend owner") {
		t.Fatalf("0428-207: noncanonical message must name the missing shared backend owner, got %q", msg)
	}
	if !strings.Contains(msg, "canonical") {
		t.Fatalf("0428-207: noncanonical message must name the canonical protected owner check, got %q", msg)
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "baha app start") && !strings.Contains(lower, "retry") {
		t.Fatalf("0428-207: noncanonical message must name the user's next step, got %q", msg)
	}
	t.Log("0428-207 noncanonical-shape guard names both preconditions, canonical check, and next step")
}

// 0428-207 fail-closed boundary: the guard refuses; it never silently returns
// (false, nil) when a native network has no verifiable protected owner. This
// unit test asserts the message content; what it does NOT prove is stated in
// the akte: the guard's real entry (len(actual) != 0, a native default network
// present) requires a runtime daemon, i.e. the baha-docker lifecycle proof, and
// this unit test only pins the message wording that the production guard returns.
func Test0428207CoreNetworkBindingGuardFailClosedNeverSilent(t *testing.T) {
	missingErr := coreDefaultNetworkBindingMissingOwnerMessage(errors.New("missing"))
	if missingErr == nil {
		t.Fatalf("0428-207: missing-owner guard must never be nil")
	}
	missing := missingErr.Error()
	if strings.Contains(missing, "lacks a protected Core or shared backend owner") {
		t.Fatalf("0428-207: old unactionable message must be replaced, got %q", missing)
	}
}

// 0428-207 activation-path regression (acceptance criterion 2): the guard must
// report the actionable message through the REAL production function
// inspectCoreDefaultNetworkBinding, not only through the renderer in a unit
// test that calls the renderer directly. Here we drive the actual guard with a
// real ResolvedTarget and Files: an isolated Core whose shared-backend owner
// files exist but are unprotected (unlocked permissions), so the guard's
// non-ENOENT shared-owner check (core_network_binding.go:24-26) fires WITHOUT a
// runtime daemon and returns the guarded, wrapped refusal. We assert that
// inspectCoreDefaultNetworkBinding itself returns a non-nil error carrying that
// guarded wording, proving the production guard is wired end-to-end on App Edit
// activation. The daemon-gated ENOENT branch (ExistingControlPlaneRuntimeInventory
// at core_network_binding.go:55-57) is intentionally out of scope here; the
// renderer ENOENT wording is pinned by Test0428207CoreNetworkBindingGuardNamesMissingPreconditions.
func Test0428207CoreNetworkBindingGuardActivationPathReturnsGuardedError(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// inspectCoreDefaultNetworkBinding resolves the shared owner from
	// targetDataRoot(target) = deployment.TargetStateRoot(target.Name), i.e.
	// <BASEHARBOR_STATE_DIR>/targets/<name>/providers/shared-backends/core.
	// Place the fixture there (not under the generic state root) so the guard
	// actually reads it.
	target := deployment.ResolvedTarget{Name: "core-test"}
	dataRoot, err := targetDataRoot(target)
	if err != nil {
		t.Fatalf("0428-207: targetDataRoot: %v", err)
	}
	sharedDir := filepath.Join(dataRoot, "providers", "shared-backends", "core")
	if err := os.MkdirAll(sharedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// state.json is written with 0777 permissions so ExistingSharedBackendInventoryAt
	// returns a non-ENOENT error ("shared backend owner files are unprotected or
	// noncanonical"); the guard's core_network_binding.go:24-26 path fires.
	if err := os.WriteFile(filepath.Join(sharedDir, "state.json"), []byte(`{"environment":"core"}`), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "provider.env"), []byte("BASEHARBOR_X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Project:         bhruntime.SharedProjectName(target.Name),
		ResourceProject: bhruntime.SharedResourceProjectName(target.Name),
	}

	guarded, err := inspectCoreDefaultNetworkBinding(context.Background(), target, files)
	// The fail-closed guard never silently returns (false, nil) when it cannot
	// verify the protected owner.
	if err == nil {
		t.Fatalf("0428-207: inspection through the real guard must report the missing protected owner, got (false, nil)")
	}
	_ = guarded
	// The message must carry the guarded shared-owner refusal the production
	// function returns, proving the real guard (not a stub) produced it.
	if !strings.Contains(err.Error(), "verify existing shared network owner") {
		t.Fatalf("0428-207: real guard must return the protected-owner refusal through inspectCoreDefaultNetworkBinding, got %q", err.Error())
	}
	t.Log("0428-207 inspectCoreDefaultNetworkBinding returns the guarded protected-owner refusal through the real production function")
}
