package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestApplicationImpactAuthorizationRequiresConcreteConsentWithoutWrites(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.Manifest.Services.SQL = true
	ctx := withAssumeYes(machineNoninteractiveContext(context.Background()), true)
	err := authorizeApplicationProviderChanges(ctx, r, strings.NewReader("yes\nyes\n"), io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired {
		t.Fatalf("ordinary --yes granted provider change consent: %v", err)
	}
	impact, ok := failure.Details.(application.ProviderImpactPlan)
	if !ok || impact.PlanID == "" || impact.ConfirmationsRequired != 2 {
		t.Fatalf("unknown actual consequences were not disclosed: %+v", failure)
	}
	consent := application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true}
	if err := authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), r, nil, io.Discard); err != nil {
		t.Fatalf("explicitly acknowledged unknown outcome permanently blocked: %v", err)
	}
	consent.PlanID = "stale"
	err = authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), r, nil, io.Discard)
	if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
		t.Fatalf("stale consent accepted: %v", err)
	}
	if _, err := os.Stat(r.TargetStateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authorization wrote provider or deployment state: %v", err)
	}
}

func TestBrokerPrerequisiteDecisionIsReadOnly(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.Manifest.Services.Secrets = true
	if err := requireResolvedApplicationCore(context.Background(), r, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.TargetStateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prerequisite decision materialized broker runtime: %v", err)
	}
}

func TestSelectiveSecretsPlanHasStableConsentWithoutAuthorityChanges(t *testing.T) {
	configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)
	intent := "version: 1\napp:\n  name: selective-secrets\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  secrets: true\nsecrets:\n  required:\n    - name: API_TOKEN\n      generate:\n        type: hex\n        bytes: 32\n"
	if err := os.WriteFile(filepath.Join(selection.RepositoryRoot, "baseharbor.yaml"), []byte(intent), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := machineNoninteractiveContext(context.Background())
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	r, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	first, err := buildResolvedApplicationPlan(ctx, r)
	if err != nil || first.ProviderPlan.Impact == nil {
		t.Fatalf("Secrets impact unavailable: %v", err)
	}
	consent := application.ProviderChangeConsent{PlanID: first.ProviderPlan.Impact.PlanID, First: true, Second: true}
	for attempt := 0; attempt < 4; attempt++ {
		r, err = resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatal(err)
		}
		if err := revalidateApplicationProviderIntent(ctx, r); err != nil {
			t.Fatalf("intent revalidation: %v", err)
		}
		next, err := buildResolvedApplicationPlan(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if next.ProviderPlan.Impact.PlanID != consent.PlanID {
			t.Fatalf("an unallocated deployment proposal invalidates the existing consent")
		}
		if err := authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), r, nil, io.Discard); err != nil {
			t.Fatalf("unchanged Secrets authority invalidates the plan at attempt %d: %v", attempt, err)
		}
	}
	if records, err := deployment.ListDeployments(r.Target.Name); err != nil || len(records) != 0 {
		t.Fatalf("planning/consent allocated deployment authority: records=%d error=%v", len(records), err)
	}
	if err := os.WriteFile(r.ManifestPath, []byte(strings.Replace(intent, "bytes: 32", "bytes: 48", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	var failure *machine.Error
	err = authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), r, nil, io.Discard)
	if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
		t.Fatalf("changed repository intent accepted earlier consent: %v", err)
	}
}

func TestIdentityFootprintIncludesExactSQLWithoutManagementBundle(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.Manifest.Services.Identity = true
	plan, err := buildResolvedApplicationPlan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Footprint.Core != application.CoreNotRequired || len(plan.Footprint.Requirements) != 1 || len(plan.Footprint.Additional) != 2 {
		t.Fatalf("Identity storage omitted or blanket Core required: %+v", plan.Footprint)
	}
	ids := map[string]bool{}
	for _, provider := range plan.Footprint.Additional {
		ids[provider.InstanceID] = true
	}
	for _, provider := range plan.Footprint.Additional {
		if !provider.Ancillary && (len(provider.Dependencies) != 1 || !ids[provider.Dependencies[0]]) {
			t.Fatalf("Identity -> selected SQL dependency omitted: %+v", provider)
		}
	}
	if _, err := os.Stat(r.TargetStateRoot); !os.IsNotExist(err) {
		t.Fatal("Identity plan materialized runtime")
	}
}
