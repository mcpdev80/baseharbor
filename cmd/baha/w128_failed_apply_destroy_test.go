package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestW128DestroyAfterInterruptedFirstApplyReconcilesRetainedOwners is the
// permanent red-first regression for card 0428-286
// (t-20261116-r28-bug-failed-apply-destroy).
//
// The J-Commerce live transcript shows the state this test reproduces:
//   - `baha app apply` on a fresh application writes its "applying" deployment
//     record and retains the intended provider owners in the Target provider
//     registry BEFORE any runtime is realized (app_apply.go),
//   - preflight then rejects the candidate (the reported case: privileged
//     cadvisor and a docker.sock-mounted floci) and the apply fails,
//   - `baha app destroy --yes` afterwards failed closed with
//     "Retained provider ownership could not be verified through its canonical
//     deployment and native renderer" (machine.ErrorOwnershipAmbiguous,
//     cause retained_provider_ownership_unverified). On baseline the retained
//     owner can never be verified: the retention seam stores it without the
//     canonical provider descriptor, so it is unbindable, the consumer union
//     cannot resolve it, and the ownership preflight demands native evidence
//     for a realization that never existed,
//   - `baha app list` reported the failed record as DEGRADED while
//     `baha app status` reported NOT APPLIED, and the failed application was
//     never removed from the inventory.
//
// Historical note: this state is distinct from the accepted v0.4.27 card
// t-20261104-r27-destroy-after-aborted-apply (#1375, w154 tests): that state
// carries an applied intent with last_applied_ref after a provider change
// aborted mid-apply; this state is an interrupted FIRST apply whose retained
// provider owners were never bound. Do not reopen that card from here.
//
// The test drives the REAL pending-ownership retention seam and the REAL CLI
// destroy. It requires no Docker: nothing was ever realized, so the owned
// cleanup set is empty by verified absence.
func TestW128DestroyAfterInterruptedFirstApplyReconcilesRetainedOwners(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	// The wizard-selected SQL/cache defaults never materialize in this
	// interrupted first apply; the broker (runtime PKI) and workload owners
	// are the application-scoped retained owners this card names.
	m := application.WithWorkloadComponents(withoutSharedBackends(application.New("demo", "dev", false, false, true)), "demo-app")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for apply: %v", err)
	}

	// (1) Reconstruct the interrupted-first-apply state exactly as the real
	// apply sequence leaves it:
	//   a. the "applying" deployment record written before any mutation,
	//   b. the retained provider owners from the same seam,
	//   c. NO materialized runtime (preflight rejected the candidate before
	//      EnsureRuntime; the reported failure is preflight itself, which no
	//      unit test can drive without an engine).
	if _, err := recordDeploymentBeforeMutation(context.Background(), resolved, "applying", ""); err != nil {
		t.Fatalf("record applying state: %v", err)
	}
	if err := application.RetainPendingProviderOwnersAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatalf("retain pending provider owners: %v", err)
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatalf("load applying record: %v", err)
	}
	if record.Observed.State != "applying" || record.Source.Kind != "incomplete" {
		t.Fatalf("precondition: interrupted first-apply record state=%q kind=%q", record.Observed.State, record.Source.Kind)
	}
	// The first-apply record carries no last-applied reference: a healthy
	// retry must first bind one, and the interrupted state never had one.
	if dimension := record.References[deployment.ReferenceApplied]; dimension != nil && strings.TrimSpace(dimension.Release) != "" {
		t.Fatalf("precondition: first-apply record already carries an applied reference %q", dimension.Release)
	}
	rtDir := application.RuntimeFilesFor(resolved.Store, resolved.Manifest).Dir
	if _, statErr := os.Lstat(rtDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("precondition: runtime dir must be unmaterialized, statErr=%v", statErr)
	}

	// (2) The interrupted apply retained exactly the app-scoped owners and
	// bound nothing: this is the "partial provider registration" the card
	// names, and it is what destroy must reconcile safely.
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatalf("load provider registry: %v", err)
	}
	retained := 0
	for _, instance := range registry.Instances {
		if instance.Scope != capability.ScopeApplication || instance.OwnerApplicationID != resolved.Manifest.ApplicationID {
			continue
		}
		if len(registry.BindingsForProviderInstance(instance.ID)) != 0 {
			t.Fatalf("precondition: retained owner %s already has a binding; first-apply retention must bind nothing", instance.ID)
		}
		retained++
	}
	if retained == 0 {
		t.Fatal("precondition: the interrupted apply retained no provider owner")
	}

	// (3) The ownership preflight seam that failed in the transcript must
	// reconcile the never-realized, unbound owners instead of refusing, and it
	// must persist that reconciliation so EVERY later surface — app list, app
	// status, the destroy mutation seam's consumer revalidation — sees the
	// same canonical registry. On baseline this step fails with the exact
	// reported ownership error (red proof).
	if err := checkRetainedApplicationProviderOwnership(context.Background(), resolved); err != nil {
		var structured *machine.Error
		if errors.As(err, &structured) && structured.CauseCode == "retained_provider_ownership_unverified" {
			t.Fatalf("baseline reproduction of the reported ownership refusal: %v", err)
		}
		t.Fatalf("retained provider ownership must reconcile the never-realized owners, got: %v", err)
	}
	reconciled, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatalf("reload provider registry after reconciliation: %v", err)
	}
	for _, instance := range reconciled.Instances {
		if instance.Scope == capability.ScopeApplication && instance.OwnerApplicationID == resolved.Manifest.ApplicationID &&
			len(instance.Provider.Capabilities) == 0 {
			t.Fatalf("ownership reconciliation left %s unbindable", instance.ID)
		}
	}

	// (4) The interrupted FIRST apply left no last-applied reference, so the
	// stale-applied guard must not refuse the recovery destroy (a record that
	// never applied cannot be "stale").
	if err := record.Stale(); err != nil {
		var stateErr *deployment.DeploymentRecordStateError
		if errors.As(err, &stateErr) && stateErr.Kind == "stale" {
			t.Fatalf("baseline reproduction: first-apply state misread as stale: %v", err)
		}
		t.Fatalf("the interrupted first-apply record must not block recovery destroy: %v", err)
	}

	// (4) The real CLI destroy must succeed. On baseline this is the exact
	// reported failure: "nothing was deleted (retained provider ownership:
	// Retained provider ownership could not be verified...)" with rc!=0.
	var out, errOut bytes.Buffer
	if err := executeApplicationDestroyLifecycle(machineNoninteractiveContext(context.Background()), application.DefaultStore(), []string{"--yes"}, &out, &errOut); err != nil {
		var structured *machine.Error
		if errors.As(err, &structured) && structured.CauseCode == "retained_provider_ownership_unverified" {
			t.Fatalf("baseline reproduction of the reported destroy failure: %v", err)
		}
		t.Fatalf("destroy of the interrupted-first-apply state must return rc=0, got: %v\noutput:\n%s%s", err, out.String(), errOut.String())
	}

	// (5) Exclusively app-owned records are gone; the failed app no longer
	// appears in the inventory that reported DEGRADED on baseline.
	if _, statErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity); !isMissingDeploymentRecord(statErr) {
		t.Fatalf("destroy left the failed deployment record in place: err=%v", statErr)
	}
	afterRegistry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatalf("reload provider registry: %v", err)
	}
	for _, instance := range afterRegistry.Instances {
		if instance.Scope == capability.ScopeApplication && instance.OwnerApplicationID == resolved.Manifest.ApplicationID {
			t.Fatalf("destroy left a retained provider owner %s registered", instance.ID)
		}
	}
	for _, binding := range afterRegistry.Bindings {
		if binding.ApplicationID == resolved.Manifest.ApplicationID {
			t.Fatalf("destroy left a provider binding for the failed application: %+v", binding)
		}
	}
	if appDirExists(resolved) {
		t.Fatal("destroy left the application state directory behind")
	}

	// (6) A second destroy is idempotent (rc=0), the state an operator who
	// retries after the failed apply must be able to reach.
	out.Reset()
	errOut.Reset()
	if err := executeApplicationDestroyLifecycle(machineNoninteractiveContext(context.Background()), application.DefaultStore(), []string{"--yes"}, &out, &errOut); err != nil {
		t.Fatalf("second destroy must be idempotent, got: %v\noutput:\n%s%s", err, out.String(), errOut.String())
	}
}

// TestW128SharedOnlyFailedApplyDestroyNeedsNoRuntime (negative guard, part of
// the same regression file): an application whose only retained owner is the
// shared SQL module has NOTHING application-scoped to reclaim, and the shared
// provider infrastructure must survive a failed-apply destroy untouched.
func TestW128SharedOnlyFailedApplyDestroyNeedsNoRuntime(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", true, false, false), "demo-app")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for apply: %v", err)
	}
	if _, err := recordDeploymentBeforeMutation(context.Background(), resolved, "applying", ""); err != nil {
		t.Fatalf("record applying state: %v", err)
	}
	if err := application.RetainPendingProviderOwnersAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatalf("retain pending provider owners: %v", err)
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatalf("load provider registry: %v", err)
	}
	for _, instance := range registry.Instances {
		if instance.Scope == capability.ScopeApplication && instance.OwnerApplicationID == resolved.Manifest.ApplicationID {
			t.Fatalf("precondition: shared-only application retained an application-scoped owner %s", instance.ID)
		}
	}

	// The shared-module owner must never be reclaimed by the ownership seam.
	if err := checkRetainedApplicationProviderOwnership(context.Background(), resolved); err != nil {
		t.Fatalf("shared-only failed apply must not hit the retained-ownership refusal: %v", err)
	}
	after, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatalf("reload provider registry: %v", err)
	}
	shared := false
	for _, instance := range after.Instances {
		if instance.Scope == capability.ScopeShared {
			shared = true
		}
	}
	if !shared {
		t.Fatal("ownership reconciliation removed the shared provider infrastructure owner")
	}

	// The real CLI destroy still succeeds without any engine (nothing exists).
	var out, errOut bytes.Buffer
	if err := executeApplicationDestroyLifecycle(machineNoninteractiveContext(context.Background()), application.DefaultStore(), []string{"--yes"}, &out, &errOut); err != nil {
		t.Fatalf("destroy of the shared-only failed apply must return rc=0, got: %v\noutput:\n%s%s", err, out.String(), errOut.String())
	}
	if _, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity); !isMissingDeploymentRecord(err) {
		t.Fatalf("destroy left the failed deployment record in place: err=%v", err)
	}
}

func isMissingDeploymentRecord(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "is missing") {
		return true
	}
	state, ok := deployment.DeploymentRecordState(err)
	return ok && (state.Kind == "incomplete" || state.Kind == "missing")
}

func appDirExists(resolved resolvedApplication) bool {
	_, err := os.Stat(filepath.Join(resolved.Store.Root, resolved.Manifest.Name))
	return err == nil
}

// withdrawSharedBackends removes this application's bindings from the Target
// provider registry and reclaims the shared-module references it alone
// carried, so the fixture isolates the application-scoped retention seam.
// withoutSharedBackends deselects the shared SQL/cache modules an interrupted
// first apply never realized, so this fixture isolates the application-scoped
// retention seam (the broker and workload owners stay application-scoped).
func withoutSharedBackends(m application.Manifest) application.Manifest {
	m.Services.SQL = false
	m.Services.Cache = false
	m.Services.SQLInstances = nil
	m.Services.CacheInstances = nil
	return m
}

func withdrawSharedBackends(t *testing.T, targetStateRoot string, m application.Manifest) {
	t.Helper()
	store := capability.RegistryStore{Path: filepath.Join(targetStateRoot, "provider-registry.json")}
	err := store.Update(func(registry *capability.Registry) error {
		referenced := map[string]int{}
		for _, binding := range registry.Bindings {
			referenced[binding.ProviderInstanceID]++
		}
		registry.ReleaseApplicationDeployment(m.ApplicationID, m.Environment)
		var retained []capability.ProviderInstance
		for _, instance := range registry.Instances {
			if instance.Ownership != capability.OwnershipBaseHarbor {
				retained = append(retained, instance)
				continue
			}
			// Keep providers that are still in use and instances the
			// reconciliation never touched; reclaim only the shared-module
			// reference this fixture application alone carried.
			if referenced[instance.ID] == 0 || len(registry.BindingsForProviderInstance(instance.ID)) != 0 {
				retained = append(retained, instance)
				continue
			}
			if instance.Scope == capability.ScopeApplication && instance.OwnerApplicationID == m.ApplicationID {
				retained = append(retained, instance)
				continue
			}
		}
		registry.Instances = retained
		return registry.Validate()
	})
	if err != nil {
		t.Fatalf("withdraw shared-backend fixture references: %v", err)
	}
}
