package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRejectedApplyAndTargetTeardownKeepSourcesWithoutGhostApplyingState(t *testing.T) {
	target := configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"demo-app"}}}
	mustWriteWizardTestFile(t, application.RepositoryManifestName, m.YAML())
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := recordPendingDeployment(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Observed.State != "configured" || !pending.Observed.VerifiedAt.IsZero() {
		t.Fatalf("configuration state: %#v", pending.Observed)
	}
	inputs := repositoryInitEnvPathFromStateRoot(resolved.stateRoot())
	mustWriteWizardTestFile(t, inputs, "BASEHARBOR_APP_HOSTNAME=localhost\n")
	if err := executeApplicationApplyLifecycle(context.Background(), application.DefaultStore(), nil, io.Discard, io.Discard); err == nil {
		t.Fatal("invalid workload contract applied")
	}
	failed, err := deployment.LoadDeploymentRecord(pending.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Observed.State != "failed" || failed.Observed.Ready || !failed.Observed.VerifiedAt.IsZero() {
		t.Fatalf("failed apply state: %#v", failed.Observed)
	}
	// Emulate an old v0.4.21 registration left applying before target teardown.
	failed.Observed = deployment.ObservedDeployment{State: "applying", VerifiedAt: time.Now()}
	if err := deployment.SaveDeploymentRecord(failed); err != nil {
		t.Fatal(err)
	}
	inactive, err := inactiveTargetDeployments(context.Background(), target.Name)
	if err != nil || len(inactive) != 1 {
		t.Fatalf("inactive registration: %#v %v", inactive, err)
	}
	containers := []bhruntime.RuntimeContainer{{Project: bhruntime.ApplicationProjectName(target.Name, m.Name, m.Environment), Running: true}}
	if err := markInactiveTargetDeployments(inactive, containers); err != nil {
		t.Fatal(err)
	}
	live, err := deployment.LoadDeploymentRecord(pending.Identity)
	if err != nil || live.Observed.State != "applying" {
		t.Fatal("live workload incorrectly declared absent")
	}
	if err := markInactiveTargetDeployments(inactive, nil); err != nil {
		t.Fatal(err)
	}
	after, err := deployment.LoadDeploymentRecord(pending.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if after.Observed.State != "not_applied" || after.Observed.Ready || !after.Observed.VerifiedAt.IsZero() {
		t.Fatalf("ghost after teardown: %#v", after.Observed)
	}
	var originalIntent, retainedIntent bytes.Buffer
	if err := json.Compact(&originalIntent, pending.Applied.Intent); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&retainedIntent, after.Applied.Intent); err != nil {
		t.Fatal(err)
	}
	if after.Source != pending.Source || !bytes.Equal(originalIntent.Bytes(), retainedIntent.Bytes()) {
		t.Fatal("teardown changed source/intent")
	}
	if _, err := os.Stat(inputs); err != nil {
		t.Fatalf("teardown removed inputs: %v", err)
	}
	if _, err := os.Stat(application.RepositoryManifestName); err != nil {
		t.Fatal("teardown removed repository contract")
	}
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "list"}, &out, io.Discard); err != nil || !strings.Contains(out.String(), "not_applied") || strings.Contains(out.String(), "applying") {
		t.Fatalf("list still claims applying: %s %v", out.String(), err)
	}
	out.Reset()
	if err := runWithIO(context.Background(), []string{"status"}, &out, io.Discard); err != nil || !strings.Contains(out.String(), "NOT APPLIED") {
		t.Fatalf("status disagrees: %s %v", out.String(), err)
	}
	// Even partial runtime state must survive, and cannot be relabeled as absent.
	runtimeDir := filepath.Join(resolved.stateRoot(), "state", m.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	mustWriteWizardTestFile(t, filepath.Join(runtimeDir, "owned-data"), "preserve")
	after.Observed = deployment.ObservedDeployment{State: "failed"}
	if err := deployment.SaveDeploymentRecord(after); err != nil {
		t.Fatal(err)
	}
	inactive, err = inactiveTargetDeployments(context.Background(), target.Name)
	if err != nil || len(inactive) != 0 {
		t.Fatal("partial runtime treated as absent")
	}
	if err := markInactiveTargetDeployments([]deployment.DeploymentRecord{after}, nil); err != nil {
		t.Fatal(err)
	}
	preserved, err := deployment.LoadDeploymentRecord(pending.Identity)
	if err != nil || preserved.Observed.State != "failed" {
		t.Fatal("partial runtime state was masked")
	}
	data, err := os.ReadFile(filepath.Join(runtimeDir, "owned-data"))
	if err != nil || string(data) != "preserve" {
		t.Fatal("application data changed")
	}
}
