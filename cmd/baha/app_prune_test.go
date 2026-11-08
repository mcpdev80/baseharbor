package main

import (
	"context"
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

	// resolvePruneCandidate hydrates the source-present record without loading
	// the manifest or resolved backend selection (the source is present but the
	// record still has an applied intent snapshot that must not be validated),
	// so it resolves cleanly; deletionGuard must then refuse it because the
	// source is recoverable.
	resolved, rErr := resolvePruneCandidate(context.Background(), store, []string{"live"})
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
