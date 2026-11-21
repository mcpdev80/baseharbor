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
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/delivery"
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
	if err := runWithIOAsTestOperator(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
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
	challengeErr := authorizeApplicationProviderChanges(ctx, r, nil, io.Discard)
	var challenge *machine.Error
	if !errors.As(challengeErr, &challenge) || challenge.Code != machine.ErrorApprovalRequired {
		t.Fatalf("missing read-only authorization challenge: %v", challengeErr)
	}
	submission, _ := candidateSubmissionForResolvedApp(ctx, r)
	if digest := remoteChallengeCandidateDigest(challenge.Next); digest == "" || digest != delivery.CandidateConsentDigest(submission) {
		t.Fatal("provider challenge does not identify the exact source to be approved")
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

// 0428-119 r2 / CODEX Finding 3, F3 request path: the real apply seam in
// cmd/baha must treat a registry change that ONLY differs in the external
// Reference, the OwnerEnvironment, or a duplicate-masked capability set as an
// authority change and refuse to hand stale consent forward. The test seeds a
// matching SQL instance into the authoritative provider-registry.json BEFORE the
// first plan, so both plans already carry it; it then mutates that SAME
// instance's authority-bearing field and drives authorizeApplicationProviderChanges
// again: the post-consent revalidation gate must return the typed
// stale-impact-consent conflict and no deployment state may be allocated.
//
// B1 proof: on the ORIGINAL comparators (before F1/F2), the reference/owner-env
// change alone was misread as equivalent and the stale consent was accepted.
func TestRequestPathRejectsReferenceRegistryChange(t *testing.T) {
	registryAuthorityChangeRequest(t, "reference", func(reg *capability.Registry) {
		for i := range reg.Instances {
			reg.Instances[i].Reference = "ext/swapped-reference"
		}
	})
}

func TestRequestPathRejectsOwnerEnvironmentRegistryChange(t *testing.T) {
	registryAuthorityChangeRequest(t, "owner-environment", func(reg *capability.Registry) {
		for i := range reg.Instances {
			reg.Instances[i].OwnerEnvironment = "other-env"
		}
	})
}

func TestRequestPathRejectsDuplicateCapabilityMaskingRegistryChange(t *testing.T) {
	registryAuthorityChangeRequest(t, "capability", func(reg *capability.Registry) {
		for i := range reg.Instances {
			reg.Instances[i].Provider.Capabilities = []capability.Kind{capability.SQL, capability.SQL}
		}
	})
}

// 0428-119 r4 / CODEX r3 Finding P1, request path: the real apply seam in
// cmd/baha must treat a registry change that ONLY differs in the owning manifest
// name (OwnerApplication, same OwnerApplicationID) as an authority change and
// refuse to hand stale consent forward. This drives the production
// authorizeApplicationProviderChanges seam against a persisted, authoritative
// provider-registry.json.
//
// OwnerApplication is only authority-bearing on an APPLICATION-scoped instance
// (a shared instance may not carry an application owner, so the seed is
// application-scoped with a valid UUIDv4 owner id). The first plan carries the
// seeded instance; the mutation then changes only that instance's
// OwnerApplication to a different manifest name, and the post-consent
// revalidation gate must fail closed.
func TestRequestPathRejectsOwnerApplicationRegistryChange(t *testing.T) {
	configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)
	intent := "version: 1\napp:\n  name: request-path-consent\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  sql: true\n"
	if err := os.WriteFile(filepath.Join(selection.RepositoryRoot, "baseharbor.yaml"), []byte(intent), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := machineNoninteractiveContext(context.Background())
	stateRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	// Seed an APPLICATION-scoped SQL instance (must carry an application owner).
	seed := capability.NewRegistry()
	if err := seed.Register(capability.ProviderInstance{
		ID:                 "sql-app-owned",
		Provider:           capability.PostgreSQL,
		Scope:              capability.ScopeApplication,
		Ownership:          capability.OwnershipBaseHarbor,
		OwnerApplicationID: "22222222-2222-4222-8222-222222222222",
		OwnerApplication:   "request-path-consent",
	}); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Update(func(reg *capability.Registry) error { *reg = seed; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := runWithIOAsTestOperator(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	r, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	first, err := buildResolvedApplicationPlan(ctx, r)
	if err != nil || first.ProviderPlan.Impact == nil {
		t.Fatalf("SQL impact unavailable: %v", err)
	}
	challengeErr := authorizeApplicationProviderChanges(ctx, r, nil, io.Discard)
	var challenge *machine.Error
	if !errors.As(challengeErr, &challenge) || challenge.Code != machine.ErrorApprovalRequired {
		t.Fatalf("missing read-only authorization challenge: %v", challengeErr)
	}
	consent := application.ProviderChangeConsent{PlanID: first.ProviderPlan.Impact.PlanID, First: true, Second: true}

	// Mutate the authoritative provider registry on disk: change the SAME
	// seeded instance's OwnerApplication to a different manifest name while
	// keeping OwnerApplicationID identical. The store validates the mutated
	// registry before persisting it, so an invalid test setup fails here.
	store := capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}
	if err := store.Update(func(reg *capability.Registry) error {
		for i := range reg.Instances {
			reg.Instances[i].OwnerApplication = "other-manifest"
		}
		return nil
	}); err != nil {
		t.Fatalf("mutated registry failed validation (test setup): %v", err)
	}

	// Re-resolve and drive the real apply seam: the changed authority must not
	// be accepted on the stale consent.
	r2, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := revalidateApplicationProviderIntent(ctx, r2); err != nil {
		t.Fatalf("intent revalidation: %v", err)
	}
	var failure *machine.Error
	err = authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), r2, nil, io.Discard)
	if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
		t.Fatalf("an OwnerApplication-only registry change was accepted on stale consent at the request seam: %v", err)
	}
	// A failed stale-consent authorization must not allocate any deployment:
	// the fail-closed gate rejects before execution admission.
	if records, err := deployment.ListDeployments(r2.Target.Name); err != nil || len(records) != 0 {
		t.Fatalf("stale-authority authorization allocated deployment state: records=%d error=%v", len(records), err)
	}
}

func registryAuthorityChangeRequest(t *testing.T, mutate string, apply func(reg *capability.Registry)) {
	configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)
	intent := "version: 1\napp:\n  name: request-path-consent\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  sql: true\n"
	if err := os.WriteFile(filepath.Join(selection.RepositoryRoot, "baseharbor.yaml"), []byte(intent), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := machineNoninteractiveContext(context.Background())
	// Seed the authoritative registry with a shared SQL instance BEFORE the
	// first plan, so the first plan already carries it and the mutation only
	// changes that instance's authority-bearing field.
	stateRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	seed := capability.NewRegistry()
	if err := seed.Register(capability.ProviderInstance{
		ID:        "postgresql",
		Provider:  capability.PostgreSQL,
		Scope:     capability.ScopeShared,
		Ownership: capability.OwnershipBaseHarbor,
	}); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Update(func(reg *capability.Registry) error { *reg = seed; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := runWithIOAsTestOperator(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	r, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	first, err := buildResolvedApplicationPlan(ctx, r)
	if err != nil || first.ProviderPlan.Impact == nil {
		t.Fatalf("SQL impact unavailable: %v", err)
	}
	challengeErr := authorizeApplicationProviderChanges(ctx, r, nil, io.Discard)
	var challenge *machine.Error
	if !errors.As(challengeErr, &challenge) || challenge.Code != machine.ErrorApprovalRequired {
		t.Fatalf("missing read-only authorization challenge: %v", challengeErr)
	}
	consent := application.ProviderChangeConsent{PlanID: first.ProviderPlan.Impact.PlanID, First: true, Second: true}

	// Mutate the authoritative provider registry on disk: change the SAME
	// seeded instance's authority-bearing field to a change the OLD comparators
	// misread as equivalent. The store validates the mutated registry before
	// persisting it, so an invalid test setup fails here.
	store := capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}
	if err := store.Update(func(reg *capability.Registry) error {
		apply(reg)
		return nil
	}); err != nil {
		t.Fatalf("mutated registry failed validation (test setup): %v", err)
	}

	// Re-resolve and drive the real apply seam: the changed authority must not
	// be accepted on the stale consent.
	r2, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := revalidateApplicationProviderIntent(ctx, r2); err != nil {
		t.Fatalf("intent revalidation: %v", err)
	}
	var failure *machine.Error
	err = authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), r2, nil, io.Discard)
	if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
		t.Fatalf("a %s-only registry change was accepted on stale consent at the request seam: %v", mutate, err)
	}
	// A failed stale-consent authorization must not allocate any deployment:
	// the fail-closed gate rejects before execution admission.
	if records, err := deployment.ListDeployments(r2.Target.Name); err != nil || len(records) != 0 {
		t.Fatalf("stale-authority authorization allocated deployment state: records=%d error=%v", len(records), err)
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
