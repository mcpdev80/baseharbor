package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// This file is the RED-FIRST regression for card
// t-20261106-r27-remote-up-podman-owner-missing (card number 0427-117). It
// exercises the REAL remote-podman app-apply owner seam exactly like
// recordDeploymentBeforeMutation does: a marker-proven isolated podman Target
// with no own installation.json must resolve against the boot installation
// that the CLI's OWN effective-target selection resolves (activated target ->
// persisted -> config default -> single target -> "local").
//
// On the broken commit every "expected" path fails with
// deployment.ErrDeploymentOwnerMissing (RED); at the fix commit the guarded
// TEST-SCOPE owner loan makes them pass while all fail-closed refusals stay
// refused. The test uses ONLY the exported seam and pre-existing helpers so it
// compiles — and fails — on the broken commit.

// bahaTestIsolationMarkerFile / bahaTestIsolationMarkerSchema name the
// machine-readable marker the loan verifier reads. Kept in this test-only file
// so a production binary never contains the trigger.
const bahaTestIsolationMarkerFile = "baseharbor.test-isolation.json"
const bahaTestIsolationMarkerSchema = "baseharbor.test-isolation/v1"

func writeBahaTestIsolationMarker(t *testing.T, targetName string) {
	t.Helper()
	targetRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve target state root for %q: %v", targetName, err)
	}
	if err := os.MkdirAll(targetRoot, 0700); err != nil {
		t.Fatalf("create target state root for marker: %v", err)
	}
	info, err := os.Lstat(targetRoot)
	if err != nil {
		t.Fatalf("stat target state root: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("target state root must be a protected directory, got %v", info.Mode())
	}
	marker := map[string]string{"version": bahaTestIsolationMarkerSchema, "target": strings.TrimSpace(targetName)}
	raw, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("encode test isolation marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, bahaTestIsolationMarkerFile), append(raw, '\n'), 0600); err != nil {
		t.Fatalf("write test isolation marker: %v", err)
	}
}

// seedBootInstallation provisions a genuine, READY Core installation under the
// runtime root of the boot Target (identity-less no-op steps, same fixture
// shape as internal/deployment/deployment_owner_binding_test.go).
func seedBootInstallation(t *testing.T, bootTarget, runtime string) coreinstallation.State {
	t.Helper()
	root, err := targetRuntimeStateRootByName(bootTarget)
	if err != nil {
		t.Fatalf("resolve runtime root for %q: %v", bootTarget, err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	spec := coreinstallation.Spec{Target: bootTarget, Runtime: runtime, MachineRole: coreinstallation.Development}
	steps := coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}
	state, err := coreinstallation.Run(context.Background(), root, spec, steps)
	if err != nil {
		t.Fatalf("provision boot installation: %v", err)
	}
	if !state.Ready {
		t.Fatal("fixture boot installation is not ready")
	}
	return state
}

// loanIsolatedPodmanConfig persists a boot Target that owns the installation
// and an isolated podman test Target without one. The boot Target is the
// configuration DEFAULT so the CLI effective-target selection resolves it —
// exactly like the remote-podman apply path selects its boot Core.
func loanIsolatedPodmanConfig(t *testing.T, boot, iso string) {
	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	bootDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "podman"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	isoDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "podman"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: bootDef,
			iso:  isoDef,
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist isolated-podman loan config: %v", err)
	}
}

func loanIsolatedIdentity(t *testing.T, target, app string) deployment.DeploymentIdentity {
	t.Helper()
	return deployment.DeploymentIdentity{
		DeploymentID:  "77777777-7777-4777-8777-777777777777",
		ApplicationID: application.MustNewApplicationID(),
		Target:        target,
		Application:   app,
		Environment:   "dev",
	}
}

// isolatePodmanBootTarget sets up: XDG roots, a boot Target owning a genuine
// podman installation, and a separate isolated podman Target with an empty
// (but protected) state root. Returns the boot installation identity.
func isolatePodmanBootTarget(t *testing.T, boot, iso string) coreinstallation.State {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := seedBootInstallation(t, boot, "podman")
	loanIsolatedPodmanConfig(t, boot, iso)
	isoRoot, err := deployment.TargetStateRoot(iso)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(isoRoot, 0700); err != nil {
		t.Fatal(err)
	}
	return state
}

// saveThroughApplySeam mimics recordDeploymentBeforeMutation: Verify first,
// then Save (Save re-verifies internally); keeps the error shape identical to
// the real apply path.
func saveThroughApplySeam(t *testing.T, target, app string) error {
	t.Helper()
	identity := loanIsolatedIdentity(t, target, app)
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.VerifyDeploymentOwner(target, filepath.Join(root, "runtime"), identity.DeploymentID); err != nil {
		return err
	}
	record := deployment.DeploymentRecord{Identity: identity}
	record.Applied.Intent = json.RawMessage(`{"version":1}`)
	return deployment.SaveDeploymentRecord(record)
}

func mustBootRuntimeRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := targetRuntimeStateRootByName(name)
	if err != nil {
		t.Fatalf("resolve runtime root for %q: %v", name, err)
	}
	return root
}

func mustLoadCfg(t *testing.T) deployment.Config {
	t.Helper()
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// TestPodmanOwnerLoanIsolatedApplyPasses is the exact-defect cell: a
// marker-proven isolated podman Target with no installation.json must pass the
// owner seam against the boot installation the CLI selects. On the broken
// commit the save fails with ErrDeploymentOwnerMissing (RED).
func TestPodmanOwnerLoanIsolatedApplyPasses(t *testing.T) {
	const boot, iso = "w427-podman-boot", "w427-podman-iso"
	state := isolatePodmanBootTarget(t, boot, iso)

	if _, err := coreinstallation.Load(mustBootRuntimeRoot(t, iso)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated podman target must have no installation.json: err=%v", err)
	}
	writeBahaTestIsolationMarker(t, iso)

	t.Logf("BASEHARBOR_TARGET=%q DefaultTarget=%q", os.Getenv("BASEHARBOR_TARGET"), mustLoadCfg(t).DefaultTarget)
	if err := saveThroughApplySeam(t, iso, "podman-isolated"); err != nil {
		t.Fatalf("marker-proven isolated podman target must pass the owner seam (card t-20261106-r27-remote-up-podman-owner-missing): %v", err)
	}

	// The loan must never materialize a second installation.json: the boot
	// installation stays the single durable owner (ADR 0021).
	isoRoot, err := deployment.TargetStateRoot(iso)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(isoRoot, "runtime", "installation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loan must not write an installation.json at the test target: err=%v", err)
	}
	bootState, err := coreinstallation.Load(mustBootRuntimeRoot(t, boot))
	if err != nil {
		t.Fatal(err)
	}
	if bootState.ID != state.ID {
		t.Fatal("boot installation identity changed through the loan path")
	}
}

// TestPodmanOwnerLoanConnectorBoundStaysFailClosed pins the fail-closed guard:
// a connector-bound isolated target never receives a loan, before AND after
// the fix, with the SAME sentinel ErrDeploymentOwnerMissing.
func TestPodmanOwnerLoanConnectorBoundStaysFailClosed(t *testing.T) {
	const boot, node = "w427-podman-boot", "w427-podman-node"
	access := map[string]deployment.AccessDefinition{
		"local":                     {Provider: "local", Reference: "local"},
		"baseharbor-node-connector": {Provider: "baseharbor-node-connector", Reference: "node-a"},
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}, Scope: "local"},
			node: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "baseharbor-node-connector"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist loan config: %v", err)
	}
	seedBootInstallation(t, boot, "podman")
	if _, err := coreinstallation.Load(mustBootRuntimeRoot(t, node)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("connector node must have no installation.json: err=%v", err)
	}
	writeBahaTestIsolationMarker(t, node)
	if err := saveThroughApplySeam(t, node, "podman-node"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("connector-bound isolated target must stay fail-closed owner-missing: %v", err)
	}
}

// TestPodmanOwnerLoanUnmarkedStaysFailClosed pins the unmarked guard: an
// isolated target WITHOUT the isolation marker never receives a loan.
func TestPodmanOwnerLoanUnmarkedStaysFailClosed(t *testing.T) {
	const boot, prod = "w427-podman-boot", "w427-podman-prod"
	isolatePodmanBootTarget(t, boot, prod)
	// Deliberately NO marker: a production-shaped ownerless Target must never
	// receive a loan.
	if err := saveThroughApplySeam(t, prod, "podman-prod"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("unmarked isolated target must stay fail-closed owner-missing: %v", err)
	}
}

// -----------------------------------------------------------------------------
// PRODUCTION (ungated) regression — the fix in this lane (card 0427-117 r5).
//
// Every test below runs with NO bahaTestProcessEnv seam and WITHOUT
// testing.Testing() provenance, so they exercise the exact production path
// recordDeploymentBeforeMutation runs from a normal `baha` binary. The helpers
// above (isolatePodmanBootTarget, saveThroughApplySeam) are shared with the
// test-scoped regression; they are here with an un-gated config. On the broken
// commit (the test-gated loan) the pass cell fails with ErrDeploymentOwnerMissing
// (RED); at the fix the guarded local-boot loan makes it pass.
// -----------------------------------------------------------------------------

// repoSourceForIsolatedApply writes a repository manifest with a backend SQL
// service (so the apply lifecycle passes its Core/readiness prerequisite and
// reaches the real recordDeploymentBeforeMutation owner loan) under a working
// directory and registers the authored application, returning the working dir.
func repoSourceForIsolatedApply(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	m := application.New(name, "dev", true, false, false)
	mustWriteInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m)
	return root
}

// TestProductionOwnerLoanLocalIsolatedApplyPasses is the exact-defect cell: it
// drives the REAL production apply command runner — recordDeploymentBeforeMutation,
// the exact function runApplicationApplyLifecycle calls at the write seam —
// against a genuinely ownerless isolated local podman Target whose boot Target
// owns a READY Core, and asserts the write is authorized by the un-gated owner
// loan. The isolated Target has no own installation.json, so only the loan
// (resolved against the boot Core the effective-target selection names) can
// authorize the write.
//
// recordDeploymentBeforeMutation installs the real apply command context via
// setLoanExecutionContext and calls deployment.VerifyDeploymentOwner, the exact
// seam the production apply uses. The loan resolves the boot Core through
// bahaBootCoreTargetName (no BASEHARBOR_TARGET is set here, so it falls to the
// config default, the boot Core) — a DIFFERENT Target than the isolated one, so
// the equal-name guard does not refuse.
//
// On the broken commit (the test-gated loan) the write fails with the verbatim
// ErrDeploymentOwnerMissing (RED); at the fix the un-gated loan grants and the
// pending record is written (GREEN). No test-process provenance is set up: the
// loan must activate from a normal `baha` command context.
func TestProductionOwnerLoanLocalIsolatedApplyPasses(t *testing.T) {
	const boot, iso = "p427-podman-boot", "p427-podman-iso"

	// Set up a boot Target owning a genuine podman installation. Only the boot
	// Target is configured as the config default (DefaultTarget == boot); the
	// isolated Target is a configured-but-ownerless sibling that exists only as
	// a filesystem state root.
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := seedBootInstallation(t, boot, "podman")

	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}, Scope: "local"},
			iso:  {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}, Scope: "local"},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist isolated-podman loan config: %v", err)
	}

	if _, err := coreinstallation.Load(mustBootRuntimeRoot(t, iso)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated podman target must have no installation.json: err=%v", err)
	}
	// The isolated Target's state root must be a real, protected directory.
	isoRoot, err := deployment.TargetStateRoot(iso)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(isoRoot, 0700); err != nil {
		t.Fatal(err)
	}

	// Build a repository-sourced resolved application for the isolated Target.
	// The apply command runner initializes the repository identity the same way
	// before it resolves the application; we mirror that here so the manifest
	// carries its canonical ApplicationID.
	root := repoSourceForIsolatedApply(t, "podman-isolated")
	selection, err := application.ResolveRepositoryEnvironment(root, "dev")
	if err != nil {
		t.Fatalf("resolve repository environment for isolated Target: %v", err)
	}
	isoTarget, err := cfg.ResolveTarget(iso, "")
	if err != nil {
		t.Fatalf("resolve isolated Target: %v", err)
	}
	selection, err = resolveRepositoryLifecycleIntent(context.Background(), isoTarget, selection, "apply")
	if err != nil {
		t.Fatalf("initialize isolated Target repository identity: %v", err)
	}
	resolved, err := resolvedRepositoryApplication(isoTarget, isoRoot, selection, "apply")
	if err != nil {
		t.Fatalf("resolve isolated Target application for real apply runner: %v", err)
	}

	// Drive the REAL production apply command runner: recordDeploymentBeforeMutation
	// installs the apply command context (setLoanExecutionContext) and calls
	// VerifyDeploymentOwner, exactly as the production apply does. We provide the
	// operator confirm digest the way the production apply does (via the apply
	// context) so the repository candidate submission binds consent and the write
	// proceeds to the pending state.
	submission, _ := candidateSubmissionForResolvedApp(context.Background(), resolved)
	confirmDigest := delivery.CandidateConsentDigest(submission)
	applyCtx := withCandidateConsentDigest(context.Background(), confirmDigest)
	if _, err := recordDeploymentBeforeMutation(applyCtx, resolved, "applying", confirmDigest); err != nil {
		t.Fatalf("real CLI app apply must pass the un-gated production owner loan for the ownerless local isolated podman target; got %v", err)
	}

	// The owner loan must have produced a resolvable pending deployment record
	// for the isolated Target (proving the un-gated loan authorized the write
	// through the real production runner).
	record, found, findErr := deployment.FindDeployment(iso, resolved.Manifest.ApplicationID, "dev")
	if findErr != nil {
		t.Fatalf("find deployment record after real apply: %v", findErr)
	}
	if !found {
		t.Fatalf("real CLI apply must write a pending deployment record for the isolated target %q", iso)
	}
	if record.Observed.State != "applying" && record.Observed.State != "configured" && !record.Observed.Ready {
		t.Fatalf("real CLI apply must record the deployment as applying/ready, got state=%q ready=%v", record.Observed.State, record.Observed.Ready)
	}

	// The loan must never materialize a second installation.json: the boot
	// installation stays the single durable owner (ADR 0021).
	if _, err := os.Lstat(filepath.Join(isoRoot, "runtime", "installation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loan must not write an installation.json at the test target: err=%v", err)
	}
	bootState, err := coreinstallation.Load(mustBootRuntimeRoot(t, boot))
	if err != nil {
		t.Fatal(err)
	}
	if bootState.ID != state.ID {
		t.Fatal("boot installation identity changed through the loan path")
	}
}

// TestProductionOwnerLoanConnectorBoundStaysFailClosed pins the fail-closed
// guard: a connector-bound isolated target never receives a loan, before AND
// after the fix, with the SAME sentinel ErrDeploymentOwnerMissing, and without
// any test-process provenance.
func TestProductionOwnerLoanConnectorBoundStaysFailClosed(t *testing.T) {
	const boot, node = "p427-podman-boot", "p427-podman-node"
	access := map[string]deployment.AccessDefinition{
		"local":                     {Provider: "local", Reference: "local"},
		"baseharbor-node-connector": {Provider: "baseharbor-node-connector", Reference: "node-a"},
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}, Scope: "local"},
			node: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "baseharbor-node-connector"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist loan config: %v", err)
	}
	seedBootInstallation(t, boot, "podman")
	if _, err := coreinstallation.Load(mustBootRuntimeRoot(t, node)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("connector node must have no installation.json: err=%v", err)
	}
	if err := saveThroughApplySeam(t, node, "podman-node"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("connector-bound isolated target must stay fail-closed owner-missing (no test provenance): %v", err)
	}
}

// TestProductionOwnerLoanUnmarkedLocalStaysFailClosed pins the unmarked guard:
// an ownerless Local target with no boot Core that owns it never receives a
// loan and stays ErrDeploymentOwnerMissing, without any test-process
// provenance — the same sentinel as the broken commit.
func TestProductionOwnerLoanUnmarkedLocalStaysFailClosed(t *testing.T) {
	const boot, prod = "p427-podman-boot", "p427-podman-prod"
	isolatePodmanBootTarget(t, boot, prod)
	if err := saveThroughApplySeam(t, prod, "podman-prod"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("unmarked local isolated target must stay fail-closed owner-missing without test provenance: %v", err)
	}
}

// TestProductionOwnerLoanRealApplyRunnerStaysFailClosed drives the REAL
// production apply command runner (executeApplicationApplyLifecycle — the exact
// entry point runApplicationApplyLifecycle exposes) for a repository-sourced
// application selected on a genuinely ownerless isolated local podman Target,
// and asserts that BOTH the broken commit and the fix commit refuse with the
// verbatim sentinel ErrDeploymentOwnerMissing.
//
// This is the fail-closed integration path: the real apply runner resolves the
// effective Target through the CLI's OWN effective-target selection, and that
// selection is byte-identical to the owner loan's boot-Target selection
// (BASEHARBOR_TARGET -> persisted -> DefaultTarget -> single target -> local).
// Setting BASEHARBOR_TARGET=iso makes the runner apply to iso while the loan
// resolves the same boot Target, so boot == iso and the loan refuses. The real
// runner therefore CANNOT grant for an ownerless isolated Target — the grant
// can only come from a boot Target different from the requested one, which the
// shared selection order never produces. This test pins that fail-closed
// invariant; the grant behaviour is covered separately by
// TestProductionOwnerLoanLocalIsolatedApplyPasses, which resolves the isolated
// Target for `resolved` by hand while the loan's boot falls to the config
// default (DefaultTarget=boot, boot != iso).
func TestProductionOwnerLoanRealApplyRunnerStaysFailClosed(t *testing.T) {
	const boot, iso = "p427-podman-boot", "p427-podman-iso"
	// isolatePodmanBootTarget persists DefaultTarget=boot with a genuine boot
	// installation and a protected-but-empty iso state root.
	isolatePodmanBootTarget(t, boot, iso)
	repoSourceForIsolatedApply(t, "podman-isolated")
	// Point the real apply runner at the ownerless isolated Target.
	t.Setenv("BASEHARBOR_TARGET", iso)
	var out, errOut bytes.Buffer
	err := executeApplicationApplyLifecycle(context.Background(), application.DefaultStore(), []string{}, &out, &errOut)
	if !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("real CLI app apply for the ownerless isolated podman target must stay fail-closed with ErrDeploymentOwnerMissing, got: %v", err)
	}
	// The loan must never materialize a second installation.json: the boot
	// installation stays the single durable owner (ADR 0021).
	isoRoot, err := deployment.TargetStateRoot(iso)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(isoRoot, "runtime", "installation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("real apply runner must not write an installation.json at the test target: err=%v", err)
	}
}
