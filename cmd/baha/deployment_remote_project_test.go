package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func TestDeploymentTransitionsRetainSelectedRemoteProject(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	manifest := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"demo-app"}}}
	mustWriteWizardTestFile(t, application.RepositoryManifestName, manifest.YAML())
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	resolved.Target.TenantID = "00000000-0000-0000-0000-000000000001"
	resolved.Target.AccessProvider = "baseharbor-node-connector"
	resolved.Target.AccessReference = "node-a"
	record, err := recordPendingDeployment(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolved.DeploymentRecord = &record
	// Keep resolution's old snapshot distinct from the updated registry record.
	record = *resolved.DeploymentRecord
	project := &targetsession.ProjectRecord{
		Version:  1,
		Scope:    targetenrollment.Scope{TenantID: resolved.Target.TenantID, TargetID: resolved.Target.Name, NodeID: "node-a", Runtime: resolved.Target.RuntimeProvider},
		BundleID: "owned-project", Directory: "bundles/.object-" + strings.Repeat("a", 32),
		Files: []targetsession.ProjectFileRecord{{Path: "runtime.env", SHA256: strings.Repeat("b", 64), Mode: 0600}},
	}
	record.Applied.RemoteProject = project
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	if err := recordObservedDeployment(resolved, "failed", false); err != nil {
		t.Fatal(err)
	}
	// Resolution predates the binding: the transition must reload protected state.
	pending, err := recordDeploymentBeforeMutation(context.Background(), resolved, "applying")
	if err != nil || !reflect.DeepEqual(pending.Applied.RemoteProject, project) {
		t.Fatal("pending transition discarded remote realization", err)
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := markInactiveTargetDeployments([]deployment.DeploymentRecord{pending}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil || !reflect.DeepEqual(got.Applied.RemoteProject, project) || got.Observed.State != "ready" {
		t.Fatal("local absence discarded remote binding/observation", err)
	}
	resolved.Target.AccessReference = "node-b"
	if _, err := recordPendingDeployment(context.Background(), resolved); err == nil {
		t.Fatal("changed Node silently adopted persisted project")
	}
	after, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil || !reflect.DeepEqual(after, got) {
		t.Fatal("rejected Node change modified deployment state", err)
	}
}
