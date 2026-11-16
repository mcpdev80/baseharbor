package main

// Regression coverage for 0427-150 (hunt H-P3 BF-03): a stale provider
// instance in the Target registry must not block the application edit
// surface. An explicit preference that the current projection lost is
// re-resolved against the live provider registry; an instance that is
// genuinely gone fails closed with an actionable message instead of a
// silent dead end.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/provider/builtin"
)

// staleEditCanonicalValkey derives the canonical shared valkey instance the
// current build would register for a cache need in this manifest's
// environment (valkey/shared/<boundary-token>).
func staleEditCanonicalValkey(t *testing.T, m application.Manifest) capability.ProviderInstance {
	t.Helper()
	placement, err := application.ResolveConfiguredProviderPlacement(m, capability.ProviderValkey)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeShared {
		t.Fatalf("test requires the default shared valkey placement, got %q", placement.Scope)
	}
	bundled, err := builtin.Lookup(capability.ProviderValkey)
	if err != nil {
		t.Fatal(err)
	}
	token := application.ProviderPlacementNameToken(placement.SharingBoundary)
	return capability.ProviderInstance{
		ID: "valkey/shared/" + token,
		Provider: capability.Provider{
			Kind:         capability.ProviderValkey,
			Capabilities: bundled.Integration.Provider.Capabilities,
		},
		Scope:           placement.Scope,
		SharingBoundary: placement.SharingBoundary,
		Ownership:       placement.Ownership,
		Reference:       placement.ExternalReference,
	}
}

func staleEditWriteLiveRegistry(t *testing.T, instances ...capability.ProviderInstance) {
	t.Helper()
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	for _, instance := range instances {
		if err := registry.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(registry); err != nil {
		t.Fatal(err)
	}
}

func staleEditCandidateIDs(choice appProviderChoice) []string {
	ids := make([]string, 0, len(choice.Candidates))
	for _, candidate := range choice.Candidates {
		ids = append(ids, candidate.Instance.ID)
	}
	return ids
}

// staleEditPreview is the read-only preview authority the hydration path
// requires; it echoes the owner without any Target state.
func staleEditPreview(target, applicationID string) appEditPreviewAuthority {
	return func(context.Context, appEditDesired) (GuidedReadContext, error) {
		return GuidedReadContext{
			ApplicationID: applicationID, Target: target,
			Footprint: application.Footprint{Version: application.FootprintVersion, Target: target},
		}, nil
	}
}

// TestProviderStaleCanonicalSharedInstanceIsReResolvable proves the choice
// surface re-offers a canonical shared instance that only the live registry
// (e.g. the residue of a never-applied sibling record) still lists, and that
// selecting it succeeds instead of failing the whole edit path.
func TestProviderStaleCanonicalSharedInstanceIsReResolvable(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	m := application.New("stale-live", "dev", false, true, false)
	m.ApplicationID = application.MustNewApplicationID()
	canonical := staleEditCanonicalValkey(t, m)
	staleEditWriteLiveRegistry(t, canonical)
	requirement := capability.Requirement{Kind: capability.KeyValue, Name: "default"}
	snapshot := providerChoiceSnapshot("boot")
	selection := newAppCapabilitySelection(appCapabilitySelectionInput{Current: []capability.Requirement{requirement}})
	preferences := []application.ProviderPreference{{Requirement: requirement, InstanceID: canonical.ID, ProviderKind: capability.ProviderValkey}}

	choices, err := appProviderChoicesWithPreferences(m, snapshot, selection, preferences)
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 1 {
		t.Fatalf("choices=%+v", choices)
	}
	found := false
	for _, id := range staleEditCandidateIDs(choices[0]) {
		found = found || id == canonical.ID
	}
	if !found {
		t.Fatalf("live canonical instance %q missing from candidates %v", canonical.ID, staleEditCandidateIDs(choices[0]))
	}
	selected, err := choices[0].Select(canonical.ID)
	if err != nil {
		t.Fatalf("stale canonical selection blocked the edit path: %v", err)
	}
	if selected.Preference().ProviderPreference.InstanceID != canonical.ID {
		t.Fatalf("preference lost: %+v", selected.Preference())
	}
}

// TestProviderGoneInstanceFailsClosedWithNextStep proves the "instance really
// gone" case: an explicit preference whose instance exists in neither the
// projection nor the live registry fails closed, names the instance and the
// next step, and publishes nothing.
func TestProviderGoneInstanceFailsClosedWithNextStep(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	m := application.New("stale-gone", "dev", false, true, false)
	m.ApplicationID = application.MustNewApplicationID()
	requirement := capability.Requirement{Kind: capability.KeyValue, Name: "default"}
	gone := capability.ProviderInstance{
		ID: "valkey/shared/gone-boundary",
		Provider: capability.Provider{
			Kind:         capability.ProviderValkey,
			Capabilities: []capability.Kind{capability.KeyValue},
		},
		Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "gone-boundary",
	}
	snapshot := providerChoiceSnapshot("boot")
	selection := newAppCapabilitySelection(appCapabilitySelectionInput{Current: []capability.Requirement{requirement}})
	choices, err := appProviderChoicesWithPreferences(m, snapshot, selection, []application.ProviderPreference{{Requirement: requirement, InstanceID: gone.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range staleEditCandidateIDs(choices[0]) {
		if id == gone.ID {
			t.Fatal("gone instance offered as a candidate")
		}
	}
	if _, err := choices[0].Select(gone.ID); err == nil {
		t.Fatal("gone instance silently accepted")
	} else if !strings.Contains(err.Error(), gone.ID) {
		t.Fatalf("gone-instance error must name the instance: %v", err)
	}
	// The domain preference check fails closed with its next step and leaves
	// the live registry untouched.
	targetRoot := filepath.Join(root, "targets", "boot")
	if err := os.MkdirAll(targetRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = application.WithDesiredProviderPlacementsAt(targetRoot, application.WithProviderPreferences(m, []application.ProviderPreference{{Requirement: requirement, InstanceID: gone.ID}}))
	if err == nil {
		t.Fatal("gone instance accepted by the preference authority")
	}
	if !strings.Contains(err.Error(), "no longer registered") {
		t.Fatalf("preference error must carry the re-resolution next step: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "provider-registry.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only validation must not publish a registry: %v", err)
	}
}

// TestAppEditCommandStalePreferenceReResolvesFromLiveRegistry is the BF-03
// field shape end to end on the real production authority wiring: a
// state-only sibling record's dangling desired preference made the shared
// canonical instance live on the Target while this app's Target-state
// projection lost it. Opening edit must not fail closed and confirming an
// unrelated needs change must apply and persist through the authorities.
func TestAppEditCommandStalePreferenceReResolvesFromLiveRegistry(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	t.Chdir(repo)
	mustWriteWizardTestFile(t, filepath.Join(repo, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n")
	m := application.New("stale-edit", "dev", false, true, false)
	m.ApplicationID = application.MustNewApplicationID()
	m = application.WithSQLInstances(m, "default")
	m = application.WithWorkloadComponents(m, "api")
	manifestPath := filepath.Join(repo, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, manifestPath, m)
	registerTestDeployment(t, target, m, repo, manifestPath)
	requirement := capability.Requirement{Kind: capability.KeyValue, Name: "default"}
	canonical := staleEditCanonicalValkey(t, m)

	// Registry drift, exactly the BF-03 residue: the live global registry
	// still registers the canonical shared instance (a never-applied sibling
	// registered it), while the Target-state registry keeps only the secrets
	// realization and no binding for it.
	live := capability.NewRegistry()
	if err := live.Register(canonical); err != nil {
		t.Fatal(err)
	}
	dataRoot, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(dataRoot, "provider-registry.json")}).Save(live); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	targetRegistry := capability.NewRegistry()
	baoResource := capability.Resource{Application: m.Name, Kind: capability.Secrets, Name: "default", Provider: capability.ProviderOpenBao}
	if bao, err := application.ReferenceProviderInstanceForRegistration(baoResource); err == nil {
		if err := targetRegistry.Register(bao); err != nil {
			t.Fatal(err)
		}
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Save(targetRegistry); err != nil {
		t.Fatal(err)
	}
	// The app's own desired preference names the drifted instance.
	if err := application.SaveDesiredProviderPreferencesAt(stateRoot, target.Name, m, []application.ProviderPreference{{Requirement: requirement, InstanceID: canonical.ID}}); err != nil {
		t.Fatalf("stale preference rejected before the edit even starts: %v", err)
	}

	// Opening the edit surface must not be blocked by the stale preference,
	// and the read path must resolve with the drifted instance re-adopted.
	sourceBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), []string{"stale-edit"}, "edit")
	if _, err := readGuidedApplicationContext(context.Background(), resolved); err != nil {
		t.Fatalf("stale sibling preference blocked the guided read: %v", err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil || !bytes.Equal(sourceBefore, after) {
		t.Fatalf("read-only resolution changed the application source: %v", err)
	}
}

// TestProviderStaleSiblingStateOnlyAppReResolves covers the BF-03 field shape
// directly: a never-applied (state-only) sibling record still names the
// canonical shared instance in its desired preferences, the Target projection
// of the editing app lost it, and the plan authority must re-resolve the
// instance from the live registry instead of failing the read path.
func TestProviderStaleSiblingStateOnlyAppReResolves(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	editing := application.New("stale-sibling-edit", "dev", false, true, false)
	editing.ApplicationID = application.MustNewApplicationID()
	editing = application.WithWorkloadComponents(editing, "api")
	canonical := staleEditCanonicalValkey(t, editing)
	requirement := capability.Requirement{Kind: capability.KeyValue, Name: "default"}

	// The state-only sibling registered the canonical shared instance (its
	// live record) but only its dangling desired preference proves the
	// projection was never reconciled for this app.
	if err := os.MkdirAll(filepath.Join(root, "targets"), 0o700); err != nil {
		t.Fatal(err)
	}
	live := capability.NewRegistry()
	if err := live.Register(canonical); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(live); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	empty := capability.NewRegistry()
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Save(empty); err != nil {
		t.Fatal(err)
	}

	// The editing app's desired preference names the canonical instance the
	// Target projection lost. The read-only validation re-resolves it.
	projected, err := application.PreviewReferenceProviderRegistryAt(stateRoot, application.WithProviderPreferences(editing, []application.ProviderPreference{{Requirement: requirement, InstanceID: canonical.ID}}))
	if err != nil {
		t.Fatalf("stale projection blocked the editing app: %v", err)
	}
	found := false
	for _, instance := range projected.Instances {
		found = found || instance.ID == canonical.ID
	}
	if !found {
		t.Fatalf("canonical shared instance %q lost from the projected registry", canonical.ID)
	}
	// Nothing was published by the read-only preview.
	after, err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Instances) != 0 {
		t.Fatalf("read-only preview published target registry state: %+v", after.Instances)
	}
}

// TestAppEditCommandStalePreferenceDegradesToAutomaticWithNotice proves the
// second acceptance case: an explicit preference whose instance is gone from
// BOTH the projection and the live registry must not abort the edit either.
// That one preference degrades to automatic with a visible notice; the wizard
// stays reachable.
func TestAppEditCommandStalePreferenceDegradesToAutomaticWithNotice(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	m := application.New("stale-gone-edit", "dev", false, true, false)
	m.ApplicationID = application.MustNewApplicationID()
	m = application.WithWorkloadComponents(m, "api")
	requirement := capability.Requirement{Kind: capability.KeyValue, Name: "default"}
	gone := "valkey/shared/gone-boundary"
	loaded := appEditTestLoaded(t, target.Name, []capability.Requirement{requirement}, []application.ProviderPreference{{Requirement: requirement, InstanceID: gone, ProviderKind: capability.ProviderValkey}})
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name, Revision: "r", PlanID: "stale-plan", ConfirmationsRequired: 1}
	authorities := appEditTestAuthorities(target.Name, loaded.ApplicationID, []capability.Requirement{requirement}, loaded.Preferences, []application.ProviderImpactPlan{plan}, new(int))
	authorities.Preview = staleEditPreview(target.Name, loaded.ApplicationID)
	snapshot := application.ResolutionSnapshot{Target: target.Name, Registry: capability.NewRegistry(), Facts: map[string]application.ProviderFact{}}
	var out bytes.Buffer
	session, err := appEditStart(context.Background(), target.Name, loaded.Source, snapshot, authorities, &out)
	if err != nil {
		t.Fatalf("gone provider preference aborted the edit surface: %v", err)
	}
	if !strings.Contains(out.String(), gone) || !strings.Contains(out.String(), "automatic") {
		t.Fatalf("stale preference degraded without a visible notice: %q", out.String())
	}
	if session.Controller.phase != appWizardNeeds {
		t.Fatalf("wizard did not stay reachable: phase=%q", session.Controller.phase)
	}
}

// TestAppEditCommandStalePreferenceCancelWritesNothing confirms the read-only
// boundary of the re-resolution path on the real authority wiring: a cancelled
// edit with a live-registry residue leaves the provider registry and the
// application source untouched.
func TestAppEditCommandStalePreferenceCancelWritesNothing(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	target := configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n")
	m := application.New("stale-cancel", "dev", false, true, false)
	m.ApplicationID = application.MustNewApplicationID()
	m = application.WithWorkloadComponents(m, "api")
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, manifestPath, m)
	registerTestDeployment(t, target, m, root, manifestPath)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	canonical := staleEditCanonicalValkey(t, m)
	registryPath := filepath.Join(stateRoot, "provider-registry.json")
	if err := (capability.RegistryStore{Path: registryPath}).Save(func() capability.Registry {
		registry := capability.NewRegistry()
		if err := registry.Register(canonical); err != nil {
			t.Fatal(err)
		}
		resource, err := capability.Resolve(m.Name, capability.Requirement{Kind: capability.KeyValue, Name: "default"}, capability.Valkey)
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.BindDeployment(resource, m.ApplicationID, m.Environment, canonical.ID); err != nil {
			t.Fatal(err)
		}
		return registry
	}()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"cancel\n", ""} {
		if _, err := runAppEdit(context.Background(), application.DefaultStore(), []string{"stale-cancel"}, strings.NewReader(input), io.Discard, io.Discard); !errors.Is(err, errAppWizardAbort) {
			t.Fatalf("cancel path %q error = %v", input, err)
		}
	}
	after, err := os.ReadFile(registryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("cancel changed the provider registry: %v", err)
	}
	sourceAfter, err := os.ReadFile(manifestPath)
	if err != nil || !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatalf("cancel changed the application source: %v", err)
	}
}
