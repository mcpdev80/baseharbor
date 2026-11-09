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
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/stableid"
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
