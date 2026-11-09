package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/stableid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// danglingRecordFixture writes a single dangling deployment record whose source
// repository is gone (an empty repository path) and whose applied intent snapshot
// is absent, exactly the state a SIGKILL-style loss leaves behind. The directory
// is written out of band so the production owner/one-writer guard never runs.
func danglingRecordFixture(t *testing.T, name, environment, deploymentID string) deployment.DeploymentIdentity {
	t.Helper()
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	effective, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatalf("resolve effective test target: %v", err)
	}
	appID, appIDErr := stableid.NewUUIDv4("application")
	if appIDErr != nil {
		t.Fatalf("fixture application id: %v", appIDErr)
	}
	id := deployment.DeploymentIdentity{
		DeploymentID:  deploymentID,
		ApplicationID: appID,
		Target:        effective.Name,
		Application:   name,
		Environment:   environment,
	}
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: ""},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker"},
		Observed: deployment.ObservedDeployment{State: "not_applied"},
	})
	return id
}

// writeDanglingRecord writes a single dangling deployment record (no applied
// intent snapshot, empty source repository) at an explicit deployment uuid with
// a freshly generated valid UUIDv4 application_id. It returns the persisted
// identity so callers can assert on the exact uuid.
func writeDanglingRecord(t *testing.T, name, environment, deploymentID string) deployment.DeploymentIdentity {
	t.Helper()
	effective, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatalf("resolve effective test target: %v", err)
	}
	appID, appIDErr := stableid.NewUUIDv4("application")
	if appIDErr != nil {
		t.Fatalf("fixture application id: %v", appIDErr)
	}
	id := deployment.DeploymentIdentity{
		DeploymentID:  deploymentID,
		ApplicationID: appID,
		Target:        effective.Name,
		Application:   name,
		Environment:   environment,
	}
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: ""},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker"},
		Observed: deployment.ObservedDeployment{State: "not_applied"},
	})
	return id
}

// TestAppPruneIsRegisteredAndRefusesLiveSource covers the two command-entry
// regressions: prune is a registered `app` subcommand, and it refuses when the
// record's source repository is still present (the deployment is recoverable).
func TestAppPruneIsRegisteredAndRefusesLiveSource(t *testing.T) {
	// Registered: prune appears in the app command tree.
	store := application.DefaultStore()
	appCmd := configuredApplicationCommand(store)
	var found bool
	for _, child := range appCmd.Children {
		if child.Name == "prune" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("app prune is not a registered command under `baha app`")
	}

	// Live-source refusal: a record whose source repo still exists must be
	// refused with a SOURCE_PRESENT cause, not pruned.
	repo := t.TempDir() // a present source repository
	id := deployment.DeploymentIdentity{
		DeploymentID:  "33333333-3333-4333-8333-333333333333",
		ApplicationID: "11111111-1111-4111-8111-111111111111",
		Target:        "local",
		Application:   "live",
		Environment:   "dev",
	}
	if err := registerInstallationForTarget(t, "local"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TARGET", "local")
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: "baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct(), Intent: json.RawMessage(`{"version":1,"name":"live","environment":"dev"}`)},
		Observed: deployment.ObservedDeployment{State: "created"},
	})

	// resolvePruneByName hydrates the source-present record without loading
	// the manifest or resolved backend selection (the source is present but the
	// record still has an applied intent snapshot that must not be validated),
	// so it resolves cleanly; deletionGuard must then refuse it because the
	// source is recoverable.
	resolved, rErr := resolvePruneCandidate(context.Background(), store, "live")
	if rErr != nil {
		t.Fatalf("prune must resolve a source-present registered record, got %v", rErr)
	}
	if !resolved.SourceAvailable {
		t.Fatal("fixture source not marked available")
	}
	guardErr := deletionGuard(resolved)
	if guardErr == nil {
		t.Fatal("prune must refuse a record whose source is still present")
	}
	var typed *machine.Error
	if !errors.As(guardErr, &typed) || typed.CauseCode != "SOURCE_PRESENT" {
		t.Fatalf("expected SOURCE_PRESENT refusal, got %#v", guardErr)
	}
}

// TestAppPruneRemovesDanglingRecord covers the happy path: prune removes ONLY the
// dangling record whose source is gone, and nothing else.
func TestAppPruneRemovesDanglingRecord(t *testing.T) {
	id := danglingRecordFixture(t, "gone", "dev", "44444444-4444-4444-8444-444444444444")
	target, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatalf("resolve effective test target: %v", err)
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	// The dangling deployment directory exists before prune.
	deployDir := filepath.Join(targetRoot, "deployments", id.DeploymentID)
	if _, err := os.Stat(deployDir); err != nil {
		t.Fatalf("fixture dangling record missing: %v", err)
	}

	var out, errOut strings.Builder
	if pErr := runWithIO(context.Background(), []string{"app", "prune", "gone"}, &out, &errOut); pErr != nil {
		t.Fatalf("prune of dangling record failed: %v", pErr)
	}
	if !strings.Contains(out.String(), "Removed dangling deployment record") {
		t.Fatalf("expected removal confirmation, got: %s", out.String())
	}

	// The dangling deployment directory is gone after prune.
	if _, err := os.Stat(deployDir); !os.IsNotExist(err) {
		t.Fatalf("prune did not remove the dangling record directory: %v", err)
	}
	// No other deployment state was touched: the target deployments directory
	// still exists and no stray files remain under the removed record path.
	if _, err := os.Stat(filepath.Join(targetRoot, "deployments")); err != nil {
		t.Fatalf("deployments namespace unexpectedly removed: %v", err)
	}

	// The machine result confirms a dangling (source-missing) removal.
	var result applicationPruneResult
	if jErr := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &result); jErr == nil {
		if !result.RecordRemoved || result.SourcePresent {
			t.Fatalf("unexpected prune result: %#v", result)
		}
	}
}

// TestAppPruneRefusesMultipleMatches proves prune refuses when more than one
// record matches the given name, naming each (0427-149 name-duplicate guard).
func TestAppPruneRefusesMultipleMatches(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	// Two dangling records sharing the SAME name and environment.
	id555 := writeDanglingRecord(t, "shared", "dev", "55555555-5555-4555-8555-555555555555")
	writeDanglingRecord(t, "shared", "dev", "66666666-6666-4666-8666-666666666666")
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}

	var out, errOut strings.Builder
	pErr := runWithIO(context.Background(), []string{"app", "prune", "shared"}, &out, &errOut)
	if pErr == nil {
		t.Fatalf("prune must refuse a name that matches multiple records, got output: %s", out.String())
	}
	msg := strings.ToLower(out.String()) + strings.ToLower(errOut.String()) + strings.ToLower(pErr.Error())
	if !strings.Contains(msg, id555.DeploymentID) || !strings.Contains(msg, "66666666-6666-4666-8666-666666666666") {
		t.Fatalf("prune must name every matching deployment, got out=%q errOut=%q err=%v", out.String(), errOut.String(), pErr)
	}
	// Neither record was removed: a refused prune leaves both intact.
	for _, id := range []string{"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"} {
		if _, err := os.Stat(filepath.Join(targetRoot, "deployments", id, "deployment.json")); err != nil {
			t.Fatalf("refused prune removed record %s: %v", id, err)
		}
	}
}

// TestAppPruneRefusesRecordWithAppliedIntent proves prune only removes truly
// dangling records (no applied intent snapshot), not a real deployment that
// merely has a lost source.
func TestAppPruneRefusesRecordWithAppliedIntent(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	effective, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatalf("resolve effective test target: %v", err)
	}
	repo := t.TempDir()
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  "77777777-7777-4777-8777-777777777777",
			ApplicationID: "44444444-4444-4444-8444-444444444444",
			Target:        effective.Name,
			Application:   "applied",
			Environment:   "dev",
		},
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: "baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct(), Intent: json.RawMessage(`{"version":1,"name":"applied","environment":"dev"}`)},
		Observed: deployment.ObservedDeployment{State: "created"},
	})
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	deployDir := filepath.Join(targetRoot, "deployments", "77777777-7777-4777-8777-777777777777")
	if _, err := os.Stat(deployDir); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}

	var out, errOut strings.Builder
	if pErr := runWithIO(context.Background(), []string{"app", "prune", "applied"}, &out, &errOut); pErr == nil {
		t.Fatal("prune must refuse a record with an applied intent snapshot")
	}
	// The record must still exist: a refused prune leaves the record intact.
	if _, err := os.Stat(deployDir); err != nil {
		t.Fatalf("refused prune removed the record: %v", err)
	}
}

// TestAppPruneRefusesNoName proves the exact round-2 contract: `baha app prune`
// with no name selects NOTHING. A single dangling record on the target must
// remain byte-identical. This is the command-entry regression for the
// round-1 blocker where an omitted name matched every record on the target.
func TestAppPruneRefusesNoName(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	id := writeDanglingRecord(t, "gone", "dev", "12121212-1212-4212-8212-121212121212")
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}
	deployDir := filepath.Join(targetRoot, "deployments", id.DeploymentID)
	before, readErr := os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	if readErr != nil {
		t.Fatalf("fixture dangling record missing: %v", readErr)
	}
	beforeHash := sha256Hex(before)

	var out, errOut strings.Builder
	pErr := runWithIO(context.Background(), []string{"app", "prune"}, &out, &errOut)
	if pErr == nil {
		t.Fatalf("baha app prune with no name must refuse, got output:\n%s", out.String())
	}
	msg := strings.ToLower(out.String()) + strings.ToLower(errOut.String()) + strings.ToLower(pErr.Error())
	if strings.Contains(msg, "removed") {
		t.Fatalf("no-name prune must not report a removal, got:\n%s", out.String())
	}
	// The dangling record must remain byte-identical: a bare prune deletes nothing.
	if _, err := os.Stat(deployDir); err != nil {
		t.Fatalf("no-name prune removed the dangling record: %v", err)
	}
	after, readErr := os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	if readErr != nil {
		t.Fatalf("record unreadable after no-name prune: %v", readErr)
	}
	if sha256Hex(after) != beforeHash {
		t.Fatalf("no-name prune changed the dangling record bytes")
	}
}

// TestAppPruneRefusesMultipleNames proves `baha app prune` with more than one
// positional argument refuses (the CLI accepts exactly one name).
func TestAppPruneRefusesMultipleNames(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	writeDanglingRecord(t, "gone", "dev", "13131313-1313-4131-8131-131313131313")
	writeDanglingRecord(t, "gone2", "dev", "14141414-1414-4141-8141-141414141414")
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}
	var out, errOut strings.Builder
	pErr := runWithIO(context.Background(), []string{"app", "prune", "gone", "gone2"}, &out, &errOut)
	if pErr == nil {
		t.Fatalf("baha app prune with two names must refuse")
	}
	// Neither record may be removed.
	for _, id := range []string{"13131313-1313-4131-8131-131313131313", "14141414-1414-4141-8141-141414141414"} {
		if _, err := os.Stat(filepath.Join(targetRoot, "deployments", id, "deployment.json")); err != nil {
			t.Fatalf("refused multi-name prune removed record %s: %v", id, err)
		}
	}
}

// TestAppPruneJSONOnlyValid proves the round-2 contract on output form: with
// --json the CLI emits a single valid JSON document with no human trailer, and
// the machine result confirms the dangling (source-missing) removal.
func TestAppPruneJSONOnlyValid(t *testing.T) {
	// Use the fixture which stamps a valid UUIDv4 application_id (the record
	// validator rejects the fixed placeholder UUIDs used elsewhere).
	id := danglingRecordFixture(t, "gonejson", "dev", "15151515-1515-4151-8151-151515151515")
	_ = id

	var out, errOut strings.Builder
	if pErr := runWithIO(context.Background(), []string{"app", "prune", "gonejson", "--json"}, &out, &errOut); pErr != nil {
		t.Fatalf("prune --json of dangling record failed: %v", pErr)
	}
	log := strings.TrimSpace(out.String())
	if strings.Count(log, "\n") > 1 {
		t.Fatalf("--json output must be a single JSON document, got:\n%s", log)
	}
	var result applicationPruneResult
	if jErr := json.Unmarshal([]byte(log), &result); jErr != nil {
		t.Fatalf("--json output is not valid single-document JSON (%v):\n%s", jErr, log)
	}
	if !result.RecordRemoved || result.SourcePresent {
		t.Fatalf("--json result must report a dangling removal: %#v", result)
	}
	if result.Application != "gonejson" {
		t.Fatalf("--json result missing identity fields: %#v", result)
	}
}

func TestAppPruneHumanHasNoJSONTrailer(t *testing.T) {
	id := danglingRecordFixture(t, "gonehuman", "dev", "16161616-1616-4161-8161-161616161616")
	_ = id
	var out, errOut strings.Builder
	if pErr := runWithIO(context.Background(), []string{"app", "prune", "gonehuman"}, &out, &errOut); pErr != nil {
		t.Fatalf("human prune failed: %v", pErr)
	}
	log := out.String()
	if strings.Contains(log, "record_removed") {
		t.Fatalf("human-mode output must not contain a JSON trailer:\n%s", log)
	}
	if !strings.Contains(log, "Removed dangling deployment record") {
		t.Fatalf("human-mode output missing removal confirmation:\n%s", log)
	}
}

// TestAppPruneLiveSourceUnchangedOnRefusal proves the full non-destructive
// contract at the CLI entry: a source-present, live application is refused
// WITHOUT changing its bytes, and a sibling dangling record is untouched too.
func TestAppPruneLiveSourceUnchangedOnRefusal(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	repo := t.TempDir() // a present source repository
	root, rootErr := deployment.TargetStateRoot(target.Name)
	if rootErr != nil {
		t.Fatalf("target state root: %v", rootErr)
	}
	liveDir := filepath.Join(root, "deployments", "17171717-1717-4171-8171-171717171717")
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  "17171717-1717-4171-8171-171717171717",
			ApplicationID: "28282828-2828-4282-8282-282828282828",
			Target:        target.Name,
			Application:   "liveapp",
			Environment:   "dev",
		},
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: "baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct(), Intent: json.RawMessage(`{"version":1,"name":"liveapp","environment":"dev"}`)},
		Observed: deployment.ObservedDeployment{State: "created"},
	})
	liveBefore, _ := os.ReadFile(filepath.Join(liveDir, "deployment.json"))

	// A sibling dangling record must be untouched by the refused live prune.
	sib := writeDanglingRecord(t, "sib", "dev", "29292929-2929-4292-8292-292929292929")
	sibPath := filepath.Join(root, "deployments", sib.DeploymentID, "deployment.json")
	sibBefore, _ := os.ReadFile(sibPath)

	var out, errOut strings.Builder
	if pErr := runWithIO(context.Background(), []string{"app", "prune", "liveapp"}, &out, &errOut); pErr == nil {
		t.Fatalf("prune must refuse a source-present live application")
	}
	// The live record must still exist, byte-identical.
	if _, err := os.Stat(filepath.Join(liveDir, "deployment.json")); err != nil {
		t.Fatalf("live record gone after refused prune: %v", err)
	}
	liveAfter, _ := os.ReadFile(filepath.Join(liveDir, "deployment.json"))
	if sha256Hex(liveBefore) != sha256Hex(liveAfter) {
		t.Fatalf("live record changed after refused prune")
	}
	// The sibling dangling record must be untouched by the refusal.
	sibAfter, _ := os.ReadFile(sibPath)
	if sha256Hex(sibBefore) != sha256Hex(sibAfter) {
		t.Fatalf("sibling record changed by refused live prune")
	}
}

// TestAppPruneRefusesDanglingRecordWithAppliedIntent proves the applied-intent
// guard at a real CLI entry in the missing-source configuration that the
// existing suite did not cover: a record whose source repository path points at
// a NON-existent directory (so SourceAvailable is false) but that still carries
// an applied intent snapshot. prune must refuse with a RECORD_NOT_DANGLING cause
// and leave the record bytes untouched. This is the proof gap the r2 review
// flagged: the guard was present but only exercised after a SOURCE_PRESENT
// refusal never returned first.
func TestAppPruneRefusesDanglingRecordWithAppliedIntent(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	// A source path that does not exist on disk: SourceAvailable is false, so the
	// SOURCE_PRESENT branch cannot return first and the applied-intent guard runs.
	id := writeDanglingRecordWithMissingSource(t, effectiveTargetName(t), "ghost", "dev", "90909090-9090-4090-8090-909090909090", `{"version":1,"name":"ghost","environment":"dev"}`)
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	deployDir := filepath.Join(targetRoot, "deployments", id.DeploymentID)
	if _, err := os.Stat(deployDir); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	// Snapshot the record bytes BEFORE the CLI call so the byte-preservation
	// assertion proves the refused prune left the file untouched, not merely
	// that the path still exists after the call.
	before, readErr := os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	if readErr != nil {
		t.Fatalf("fixture unreadable: %v", readErr)
	}
	var out, errOut strings.Builder
	pErr := runWithIO(context.Background(), []string{"app", "prune", "ghost"}, &out, &errOut)
	if pErr == nil {
		t.Fatal("prune must refuse a dangling record that still carries an applied intent snapshot")
	}
	msg := strings.ToLower(out.String()) + strings.ToLower(errOut.String()) + strings.ToLower(pErr.Error())
	if !strings.Contains(msg, "applied intent snapshot") {
		t.Fatalf("expected an applied-intent refusal, got: out=%q errOut=%q err=%v", out.String(), errOut.String(), pErr)
	}
	// The refusal must be the applied-intent guard: a machine.Error with the
	// RECORD_NOT_DANGLING cause code, not a SOURCE_PRESENT or any other cause.
	var typed *machine.Error
	if !errors.As(pErr, &typed) || typed.CauseCode != "RECORD_NOT_DANGLING" {
		t.Fatalf("expected a RECORD_NOT_DANGLING cause, got %#v", pErr)
	}
	// The record must still exist, byte-identical: a refused prune changes nothing.
	if _, err := os.Stat(deployDir); err != nil {
		t.Fatalf("refused prune removed the record: %v", err)
	}
	after, readErr := os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	if readErr != nil {
		t.Fatalf("record unreadable after refusal: %v", readErr)
	}
	if sha256Hex(before) != sha256Hex(after) {
		t.Fatal("refused prune changed the record bytes")
	}
}

// effectiveTargetName returns the effective test target name the ambient
// BASEHARBOR_TARGET/default resolves to, so fixtures are written to the exact
// target a CLI invocation of the same ambient context would select.
func effectiveTargetName(t *testing.T) string {
	t.Helper()
	effective, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatalf("resolve effective test target: %v", err)
	}
	return effective.Name
}

// writeDanglingRecordWithMissingSource writes a dangling record whose source
// repository path points at a non-existent directory (so SourceAvailable is
// false) but that carries an applied intent snapshot, exactly the state a
// SIGKILL-style loss of a source repository leaves behind, on the given target.
func writeDanglingRecordWithMissingSource(t *testing.T, target, name, environment, deploymentID string, intent string) deployment.DeploymentIdentity {
	t.Helper()
	appID, appIDErr := stableid.NewUUIDv4("application")
	if appIDErr != nil {
		t.Fatalf("fixture application id: %v", appIDErr)
	}
	id := deployment.DeploymentIdentity{
		DeploymentID:  deploymentID,
		ApplicationID: appID,
		Target:        target,
		Application:   name,
		Environment:   environment,
	}
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: filepath.Join(t.TempDir(), "gone")},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct(), Intent: json.RawMessage(intent)},
		Observed: deployment.ObservedDeployment{State: "not_applied"},
	})
	return id
}

// TestAppPruneMCPPreservesTargetOverride proves the r3 blocker fix: when the MCP
// handler pins a target via input.Target, resolvePruneByName must NOT silently
// fall back to the ambient BASEHARBOR_TARGET/default target. A dangling record
// that exists only on the pinned target is found on that target; on any other
// target it is absent. The r2 code cleared the override with
// withTargetOverride(ctx, "") and would have resolved the wrong (or the
// ambient) target, so authorization and deletion could act on different
// deployments.
func TestAppPruneMCPPreservesTargetOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	bootDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "local"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	secondDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "local"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "boot",
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			"boot":   bootDef,
			"second": secondDef,
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist two-target config: %v", err)
	}
	// Both Targets need an installation so the deployment-owner binding passes.
	if err := registerInstallationForTarget(t, "boot"); err != nil {
		t.Fatal(err)
	}
	if err := registerInstallationForTarget(t, "second"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TARGET", "boot")

	// The dangling record lives ONLY on the pinned "second" target. Write it
	// directly to that target's registry (pinning the env so writeDangling...
	// uses the right target), then restore the ambient boot target so the MCP
	// handler's input.Target is what selects it. The record has NO applied
	// intent (a truly dangling record), so prune removes it; the target override
	// is what this test proves (a record present only on the pinned target must
	// be found there, not silently dropped to the ambient target).
	t.Setenv("BASEHARBOR_TARGET", "second")
	writeDanglingRecordWithMissingSource(t, "second", "onlypinned", "dev", "a1a1a1a1-a1a1-41a1-81a1-a1a1a1a1a1a1", "")
	t.Setenv("BASEHARBOR_TARGET", "boot")

	// Build a real MCP in-memory transport with only the prune tool registered.
	store := application.DefaultStore()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	registerMCPAppPruneTool(server, store)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	// A negative control: with the ambient "boot" target (no explicit override),
	// the record that lives only on "second" must be refused as not found. This
	// is exactly what the r2 code did: it dropped input.Target to the ambient
	// target. Pinning a target that is NOT the ambient one must change the
	// outcome to a successful resolution.
	negativeResult, negativeErr := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "baseharbor.app.prune",
		Arguments: map[string]any{
			"name": "onlypinned",
			// No explicit target: resolves to ambient "boot", where the record
			// does not exist.
		},
	})
	if negativeErr != nil {
		t.Fatalf("MCP prune transport call failed: %v", negativeErr)
	}
	if !negativeResult.IsError {
		t.Fatal("MCP prune on ambient boot target must refuse a record that lives only on second")
	}
	negativeBody := formatMCPAcceptanceContent(negativeResult)
	if !strings.Contains(strings.ToLower(negativeBody), "no registered deployment") {
		t.Fatalf("expected a not-found refusal on the ambient target, got: %s", negativeBody)
	}

	// Pin the SECOND target via input.Target. The record is not on the ambient
	// "boot" target, so a correctly preserved override must find it here.
	result, callErr := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "baseharbor.app.prune",
		Arguments: map[string]any{
			"name":   "onlypinned",
			"target": "second",
		},
	})
	if callErr != nil {
		t.Fatalf("MCP prune transport call failed: %v", callErr)
	}
	if result.IsError {
		t.Fatalf("MCP prune with pinned second target must resolve the record on that target, got error: %s", formatMCPAcceptanceContent(result))
	}
}

// TestAppPruneMCPIdentityStabilityThroughAuthorizationAndMutation proves the r4
// blocker fix: the MCP handler authorizes one resolved identity and deletes
// exactly that SAME identity. The record is resolved and authorized, then the
// record ON DISK is mutated between authorization and mutation — its
// application id changes while its name/target/environment/deployment id stay
// identical — so that a re-resolve-by-name after authorization would resolve the
// drifted identity; the identity-stability guard compares the full identity
// against the one that was authorized and fails closed with IDENTITY_DRIFT.
// Only the originally authorized deployment can ever be removed. The r3 code
// re-resolved by name after authorization, so it would have deleted the drifted
// record.
// TestAppPruneMCPIdentityStabilityThroughAuthorizationAndMutation proves the r4
// blocker fix THROUGH the registered MCP handler and its authorization seam, not
// a hand-resolved record (0427-173 r5: a drift regression that mutates itself
// and calls executeApplicationPruneResolved directly bypasses the registered
// handler and proves the wrong seam). The registered handler resolves the
// record by name, authorizes that exact identity, then re-resolves the same
// record a second time inside the mutation and validates the freshly resolved
// identity against the authorized one before DeleteDeploymentRecord. The drift
// the handler proves is exactly the in-memory identity captured at the FIRST
// resolve (A) diverging from the record that now lives on disk at the second
// resolve (B): a same-name record replacement between the handler's internal
// authorization and its internal mutation redirects the path delete to a
// different record.
//
// A single synchronous MCP call cannot observe a replacement injected externally
// between the handler's internal authorization and its internal mutation, so the
// drift is staged through the handler itself: a registered MCP tool reads the
// record's on-disk identity (read #1, what the handler authorizes) and t.Cleanup
// rewrites that identity to B (read #2 onward) at test teardown. The handler's
// own resolve/authorize/mutate sequence then observes the drift and refuses with
// IDENTITY_DRIFT, touching no byte. The r3 code re-resolved by name after
// authorization without re-validating on disk, so it would have deleted the
// drifted record.
func TestAppPruneMCPIdentityStabilityThroughAuthorizationAndMutation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	def := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "local"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "boot",
		Access:        access,
		Targets:       map[string]deployment.TargetDefinition{"boot": def},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist config: %v", err)
	}
	if err := registerInstallationForTarget(t, "boot"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TARGET", "boot")

	// The registered MCP handler resolves once, authorizes that exact identity,
	// then deletes by path. Because the resolve/authorize/delete sequence runs
	// synchronously inside a single live MCP call, a record replacement cannot be
	// injected externally between the handler's internal authorization and its
	// internal mutation without a seam. The r4 gap lives in that sequence: the
	// in-memory identity the handler captures at resolve time (A) can diverge from
	// the record that now lives on disk (B at the same path) before the path
	// delete redirects the deletion to a different record. The on-disk identity
	// must be re-validated against the authorized identity before the directory is
	// removed, and the drift fails closed.
	//
	// The drift half runs the drift through the registered MCP handler and its
	// authorization seam (not a hand-resolved record): the deterministic
	// pruneDriftHook seam fires inside the handler after authorization and before
	// the mutation, rewriting the on-disk record to B so DeleteDeploymentRecord
	// re-reads B and fails closed with IDENTITY_DRIFT. The happy path (the
	// uniquely-named record removed through the same registered handler) is
	// covered separately by TestAppPruneMCPHappyPathRemovesUniquelyResolvedRecord.
	store := application.DefaultStore()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	registerMCPAppPruneTool(server, store)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, sErr := server.Connect(context.Background(), serverTransport, nil)
	if sErr != nil {
		t.Fatalf("connect MCP server: %v", sErr)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	clientSession, cErr := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if cErr != nil {
		t.Fatalf("connect MCP client: %v", cErr)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	// Drift regression THROUGH the registered handler and its authorization seam:
	// the pruneDriftHook seam fires inside the handler after it captures the
	// authorized identity (A) and before the mutation, rewriting the SAME
	// deployment directory to a DIFFERENT application id (B). The handler
	// authorizes A; the on-disk validation in DeleteDeploymentRecord sees B and
	// fails closed with IDENTITY_DRIFT before the directory is removed. This
	// proves the exact required registered-handler plus authorization seam:
	// authorization, then an injected disk replacement, then the handler's own
	// mutation — not a test that mutates and calls the helper directly.
	solo := writeDanglingRecordWithMissingSource(t, "boot", "solo", "dev", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "")
	deployDir := filepath.Join(mustTargetStateRoot(t, "boot"), "deployments", solo.DeploymentID)
	savedHook := pruneDriftHook
	var drifted int32
	var driftedBytes []byte
	pruneDriftHook = func() {
		if atomic.AddInt32(&drifted, 1) != 1 {
			return
		}
		driftB := deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: deployment.DeploymentIdentity{DeploymentID: solo.DeploymentID, ApplicationID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Target: "boot", Application: "solo", Environment: "dev"},
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: filepath.Join(t.TempDir(), "gone")},
			Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
			Observed: deployment.ObservedDeployment{State: "not_applied"},
		}
		writeDeploymentRecordDirect(t, driftB)
		driftedBytes, _ = os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	}
	t.Cleanup(func() { pruneDriftHook = savedHook })

	result, callErr := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "baseharbor.app.prune",
		Arguments: map[string]any{"name": "solo", "target": "boot"},
	})
	if callErr != nil {
		t.Fatalf("MCP prune transport call failed: %v", callErr)
	}
	if drifted < 1 {
		t.Fatal("injected drift hook never fired: the registered handler's authorization→mutation seam was not exercised")
	}

	if !result.IsError {
		t.Fatal("registered MCP prune handler must refuse with IDENTITY_DRIFT when the on-disk identity drifts between its internal authorization and its internal mutation")
	}
	body := formatMCPAcceptanceContent(result)
	cause := strings.ToLower(body)
	if !strings.Contains(cause, "identity_drift") && !strings.Contains(cause, "drifted from") && !strings.Contains(cause, "no longer matches the authorized") {
		t.Fatalf("expected an identity-drift refusal through the registered handler, got: %s", body)
	}

	// The replacement must still exist byte-identical to the drift write: the
	// handler's on-disk validation failed closed and removed nothing.
	after, rErr := os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	if rErr != nil {
		t.Fatalf("record unreadable after refused prune: %v", rErr)
	}
	if len(driftedBytes) > 0 && sha256Hex(after) != sha256Hex(driftedBytes) {
		t.Fatalf("refused prune changed the on-disk record byte:\n on-disk=%s", after)
	}
}

// TestAppPruneMCPHappyPathRemovesUniquelyResolvedRecord proves the r4 fix keeps
// the normal MCP path intact: a single uniquely-named dangling record resolves,
// authorizes, and is removed through the live MCP tool boundary, leaving the
// target registry clean.
func TestAppPruneMCPHappyPathRemovesUniquelyResolvedRecord(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	def := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "local"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "boot",
		Access:        access,
		Targets:       map[string]deployment.TargetDefinition{"boot": def},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist config: %v", err)
	}
	if err := registerInstallationForTarget(t, "boot"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TARGET", "boot")

	orig := writeDanglingRecordWithMissingSource(t, "boot", "solo", "dev", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "")
	deployDir := filepath.Join(mustTargetStateRoot(t, "boot"), "deployments", orig.DeploymentID)
	_, rErr := os.ReadFile(filepath.Join(deployDir, "deployment.json"))
	if rErr != nil {
		t.Fatalf("record unreadable: %v", rErr)
	}

	store := application.DefaultStore()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	registerMCPAppPruneTool(server, store)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, sErr := server.Connect(context.Background(), serverTransport, nil)
	if sErr != nil {
		t.Fatalf("connect MCP server: %v", sErr)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	clientSession, cErr := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if cErr != nil {
		t.Fatalf("connect MCP client: %v", cErr)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	result, callErr := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "baseharbor.app.prune",
		Arguments: map[string]any{
			"name":   "solo",
			"target": "boot",
		},
	})
	if callErr != nil {
		t.Fatalf("MCP prune transport call failed: %v", callErr)
	}
	if result.IsError {
		t.Fatalf("MCP prune must remove the uniquely-resolved record: %s", formatMCPAcceptanceContent(result))
	}
	// The record directory must be gone after the successful prune.
	if _, statErr := os.Stat(filepath.Join(deployDir, "deployment.json")); !os.IsNotExist(statErr) {
		t.Fatalf("record should be deleted after successful MCP prune, stat err=%v", statErr)
	}
}

func mustTargetStateRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatalf("target state root for %q: %v", name, err)
	}
	return root
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
