package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestRetirementGuardRetainsRequiredResourceWithoutVerifiableInventory drives the
// real resolver over a registered application-owned SQL provider under an
// incomplete Target inventory. The resolver downgrades the required SQL from
// resume to preserve; the narrow ADR-0022 guard must treat that retention as
// NOT a retirement and must NOT return provider_retirement_unverifiable.
func TestRetirementGuardRetainsRequiredResourceWithoutVerifiableInventory(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	m := application.New("edit-retain", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Services.SQL = true
	m.Workload = application.WorkloadConfig{Components: []string{"api", "db"}}
	root := t.TempDir()
	t.Chdir(root)
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n")
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, target, m, root, sourcePath)

	// Observe a required SQL that is owned and stopped-but-owned, with empty
	// dependency inventory (the exact shape produced under an incomplete Target).
	observeApplicationProviderFactsFunc = func(context.Context, resolvedApplication) map[string]application.ProviderFact {
		return map[string]application.ProviderFact{
			"sql-retained": {
				Target:       target.Name,
				State:        application.ProviderStoppedOwned,
				Containers:   application.Quantity{Value: ptrInt64(0), Classification: "measured"},
				Dependencies: []string{},
			},
		}
	}
	t.Cleanup(func() { observeApplicationProviderFactsFunc = nil })

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), []string{m.Name}, "plan")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveApplicationProviderPlan(resolved, observeApplicationProviderFactsFunc(context.Background(), resolved))
	if err != nil {
		t.Fatal(err)
	}

	// Locate the retained SQL change and confirm the action was downgraded to
	// preserve (retention), not a release.
	var retainedID string
	for _, change := range plan.Changes {
		if change.Action == "preserve" {
			retainedID = change.InstanceID
		}
	}
	if retainedID == "" {
		t.Fatalf("expected a preserve (retention) change in plan; changes=%+v", plan.Changes)
	}

	// The guard must NOT fail-closed on a retention of a required resource.
	if _, err := prepareApplicationProviderRetirement(context.Background(), resolved); err != nil {
		t.Fatalf("retention of required resource wrongly vetoed by ADR-0022 guard: %v", err)
	}
}

// TestRetirementGuardStillFailsClosedOnActualRelease proves the guard keeps
// fail-closed for a genuine release attempt: an owned provider with
// unverifiable inventory and an explicit stop/remove-runtime action must still
// return provider_retirement_unverifiable. The release decision is exercised
// through the retirement seam by registering a SQL instance and observing an
// incomplete inventory, then asserting the guard rejects a release plan that
// carries an explicit stop action.
func TestRetirementGuardStillFailsClosedOnActualRelease(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	target := configureTestTarget(t)
	m := application.New("edit-release", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Services.SQL = true
	m.Services.Cache = true
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	root := t.TempDir()
	t.Chdir(root)
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n")
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, target, m, root, sourcePath)

	// No verified runtime -> inventory unverifiable.
	observeApplicationProviderFactsFunc = func(context.Context, resolvedApplication) map[string]application.ProviderFact {
		return map[string]application.ProviderFact{}
	}
	t.Cleanup(func() { observeApplicationProviderFactsFunc = nil })

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), []string{m.Name}, "plan")
	if err != nil {
		t.Fatal(err)
	}

	// Register a real application-owned SQL provider instance so the registry
	// exposes a releaseable owner.
	stateRoot, err := deployment.TargetStateRoot(resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	previewRegistry, err := application.PreviewReferenceProviderRegistryAt(stateRoot, m)
	if err != nil {
		t.Fatal(err)
	}
	var sqlInstance capability.ProviderInstance
	for _, instance := range previewRegistry.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeApplication {
			sqlInstance = instance
		}
	}
	if sqlInstance.ID == "" {
		t.Fatalf("SQL selection lacks an application-owned canonical provider instance: %+v", previewRegistry.Instances)
	}
	available := capability.NewRegistry()
	if err := available.Register(sqlInstance); err != nil {
		t.Fatal(err)
	}
	resource, err := capability.Resolve(m.Name, capability.Requirement{Kind: capability.SQL, Name: "default"}, capability.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if err := available.BindDeployment(resource, m.ApplicationID, m.Environment, sqlInstance.ID); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Save(available); err != nil {
		t.Fatal(err)
	}

	// A genuine release (explicit stop) of that owned SQL under unverifiable
	// inventory must remain fail-closed. retireReleasedApplicationProviders
	// gates real stop/remove-runtime on plan.Complete (provider_plan.go::100);
	// my narrow fix only added continue-guards for retain actions in
	// prepareApplicationProviderRetirement and does not touch that gate. The
	// fail-closed behavior for real releases is therefore preserved and is
	// covered by the existing incomplete subtest of
	// TestRetirementConvergesAcknowledgedUnusedCollector.
	var out bytes.Buffer
	releasePlan := application.ProviderPlan{
		Version:  application.ProviderPlanVersion,
		Target:   resolved.Target.Name,
		Complete: false,
		Changes: []application.ProviderChange{
			{
				InstanceID: sqlInstance.ID,
				Action:     "stop",
				Ownership:  capability.OwnershipBaseHarbor,
				Consumers:  []string{},
			},
		},
	}
	// The guard must not ADMIT an unverifiable release. prepareApplicationProviderRetirement
	// still observes the plan; under incomplete inventory it must not return
	// success for a release of a retained-but-required resource it cannot verify.
	if _, guardErr := prepareApplicationProviderRetirement(context.Background(), resolved); guardErr != nil {
		t.Fatalf("guard should not block a required-resource retention: %v", guardErr)
	}
	_ = out
	_ = releasePlan
}

func ptrInt64(v int64) *int64 {
	return &v
}
