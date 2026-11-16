package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteApplyCannotPublishBeforeCurrentProviderAcknowledgement(t *testing.T) {
	ctx, core, _ := remoteApplicationCoreFixture(t)
	ctx = machineNoninteractiveContext(withCoreAuthority(ctx, core))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	m := application.WithWorkloadComponents(application.New("remote-consent", "dev", true, false, false), "api")
	mustWriteInitializedAuthoredManifest(t, "baseharbor.yaml", m)
	r, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordPendingDeployment(ctx, r); err != nil {
		t.Fatal(err)
	}
	root, _ := deployment.DeploymentRoot(r.DeploymentIdentity)
	path := filepath.Join(root, "deployment.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = executeRemoteApplicationApply(withAssumeYes(ctx, true), r, io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired || failure.CauseCode != "impact-confirmation-required" || failure.Details == nil {
		t.Fatal("remote apply reached transport/publication without common consent", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("refused remote consent changed protected publication/intent")
	}
}

// Metadata boundary only: the caller must first verify the actual immutable
// remote publication. This test does not claim enrollment or native readiness.
func TestVerifiedRemoteReactivationPreservesFrozenSourceProvenanceAndRestoresBindings(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	original := r.Manifest
	retired, _ := json.Marshal(applicationRuntimeRetirementIntent(original))
	record.Source.Digest, record.Applied.LastAppliedRef = "published-source-digest", "published-source-reference"
	record.Applied.GeneratedState["retired_intent"] = string(record.Applied.Intent)
	record.Applied.GeneratedState["previous_intent"] = string(record.Applied.Intent)
	record.Applied.Intent = retired
	record.Observed.State = "runtime-retired"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	if err := application.ReleaseApplicationProviderReferencesAt(r.TargetStateRoot, original); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.RepositoryRoot, "compose.yaml"), []byte("services:\n  edited:\n    image: invalid.example/never:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recordVerifiedRemoteReactivation(r, original); err != nil {
		t.Fatal(err)
	}
	actual, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Source.Digest != record.Source.Digest || actual.Applied.LastAppliedRef != record.Applied.LastAppliedRef || !actual.Observed.Ready || actual.Observed.State != "ready" || actual.Applied.GeneratedState["previous_intent"] != "" {
		t.Fatal("reactivation adopted edited local source or lost verified owner")
	}
	var intent application.Manifest
	if json.Unmarshal(actual.Applied.Intent, &intent) != nil || !intent.Services.SQL || intent.ApplicationID != original.ApplicationID {
		t.Fatal("reactivation did not restore canonical verified intent")
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(r.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil || len(registry.Bindings) != 1 {
		t.Fatal("verified reactivation did not restore selected provider reference", err)
	}
}
