package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func stopSelectiveApplicationWithConsent(ctx context.Context, out io.Writer) error {
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "down")
	if err != nil {
		return err
	}
	plan, err := buildApplicationStopPlan(ctx, resolved)
	if err != nil {
		return err
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	return runWithIO(withProviderChangeConsent(ctx, &consent), []string{"app", "down"}, out, out)
}

// Native fixtures explicitly acknowledge the current domain plan. Ordinary
// --yes must not silently provide either provider-change acknowledgement.
func destroySelectiveApplicationWithConsent(ctx context.Context) error {
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "destroy")
	if err != nil {
		return err
	}
	_, plan, err := buildApplicationRuntimeRetirementPlan(ctx, resolved)
	if err != nil {
		return err
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Impact.PlanID, First: true, Second: true}
	ctx = withProviderChangeConsent(ctx, &consent)
	return runWithIO(ctx, []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{})
}

func qualifySecretsDefaultRuntimeRetirement(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, root string, before resolvedApplication, originalSecret, originalEnv [32]byte, volumes []bhruntime.ProjectResource) {
	t.Helper()
	if err := destroySelectiveApplicationWithConsent(ctx); err != nil {
		t.Fatalf("default runtime retirement: %v", err)
	}
	record, err := deployment.LoadDeploymentRecord(before.DeploymentIdentity)
	if err != nil || record.Identity != before.DeploymentIdentity || record.Observed.State != "runtime-retired" || record.Observed.Ready || record.Applied.GeneratedState["previous_intent"] != "" || record.Applied.GeneratedState["retired_intent"] == "" {
		t.Fatalf("retirement lost verified canonical owner: %v", err)
	}
	t.Log("default retirement: canonical runtime-retired owner and archived live intent verified")
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(before.Target.Name))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range actual {
		if resource.Kind == "container" {
			t.Fatal("default last-consumer retirement left provider runtime")
		}
	}
	for _, volume := range volumes {
		if !application.ResourceNamedExists(actual, volume) {
			t.Fatal("default retirement deleted retained data volume")
		}
	}
	data, err := os.ReadFile(inventory.Files.Env)
	if err != nil || sha256.Sum256(data) != originalEnv {
		t.Fatal("default retirement changed protected provider material")
	}
	t.Logf("default retirement: actual selected provider containers absent; %d owned data volumes and protected ENV retained", len(volumes))
	result, err := applicationDestroyResult(before, false)
	if err != nil || !result.OwnerPreserved || result.DataDeleted == nil || *result.DataDeleted || result.ProviderChangeResult == nil {
		t.Fatalf("default retirement receipt: %v", err)
	}
	t.Log("default retirement: retained-owner structured result verified; beginning same-owner reactivation")
	applySelectiveIdentityIntent(t, ctx)
	after, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil || after.Manifest.ApplicationID != before.Manifest.ApplicationID || after.DeploymentIdentity != before.DeploymentIdentity {
		t.Fatal("reactivation regenerated canonical owner")
	}
	status, _, err := collectResolvedApplicationStatus(ctx, after)
	if err != nil || !status.Ready {
		t.Fatalf("reactivated workload not ready: %v", err)
	}
	files := application.RuntimeFilesFor(after.Store, after.Manifest)
	service := applicationsecret.NewForApplicationRuntime(after.Store, provider, inventory.Files, after.Manifest, files)
	value, err := service.Get(ctx, after.Manifest.Name, "API_TOKEN")
	if err != nil || len(value) == 0 || sha256.Sum256(value) != originalSecret {
		clear(value)
		t.Fatal("default destroy/re-add lost actual retained secret delivery")
	}
	clear(value)
	t.Log(fmt.Sprintf("runtime=%s default destroy/re-add: provider runtime absent, owned volumes and protected ENV retained; same deployment/application ID and actual original secret readable after reactivation", provider.Kind()))
}
