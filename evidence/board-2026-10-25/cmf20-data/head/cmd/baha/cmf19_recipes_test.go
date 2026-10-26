package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Acceptance regressions ported from the CMF19 independent review evidence
// (evidence/board-2026-10-25/codex-verdict-1077-cee2ef5b.md, findings F1-F5)
// and executed against the production path instead of an overlay.

func cmf19Fixture(t *testing.T) resolvedApplication {
	t.Helper()
	target := configureTestTarget(t)
	app := application.New("cmf19-neutral", "dev", true, false, false)
	app, err := application.WithBackendPlacementProjection(app, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedApplicationScopedPostgreSQLInstance(t, target, app, capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}); err != nil {
		t.Fatal(err)
	}
	registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())
	return resolvedApplication{Target: target, TargetStateRoot: root, Manifest: app, FromRepository: true, Store: application.Store{Root: filepath.Join(t.TempDir(), "state"), Namespace: target.Name}}
}

func cmf19WizardFixture(t *testing.T) resolvedApplication {
	t.Helper()
	resolved := cmf19Fixture(t)
	resolved.Store = application.Store{}
	resolved.DeploymentIdentity = deployment.DeploymentIdentity{}
	resolved.DeploymentRecord = nil
	if err := bindApplicationScopedPostgreSQLInstance(t, capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}, resolved.Manifest, application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "default"), "default"); err != nil {
		t.Fatal(err)
	}
	return resolved
}

func cmf19Registry(t *testing.T, resolved resolvedApplication) (capability.RegistryStore, capability.Registry) {
	t.Helper()
	store := capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return store, registry
}

func cmf19BindAll(t *testing.T, resolved resolvedApplication) {
	t.Helper()
	store, registry := cmf19Registry(t, resolved)
	for _, instance := range registry.Instances {
		name := "default"
		if instance.ID == application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "analytics") {
			name = "analytics"
		}
		if err := registry.BindDeployment(capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: name, Provider: capability.ProviderPostgreSQL}, resolved.Manifest.ApplicationID, "dev", instance.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
}

func cmf19MaterializeCanonicalFiles(t *testing.T, resolved resolvedApplication) application.RuntimeFiles {
	t.Helper()
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	compose, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	return files
}

// F1: a genuine runtime-detection failure with a valid canonical inventory and
// a real binding must stay unverifiable; it may never be converted into a
// measured stopped-owned fact.
func TestCMF19RuntimeDetectionFailureMustRemainUnverifiable(t *testing.T) {
	resolved := cmf19Fixture(t)
	cmf19BindAll(t, resolved)
	cmf19MaterializeCanonicalFiles(t, resolved)
	if _, _, _, err := nativeBackendInventory(resolved); err != nil {
		t.Fatalf("fixture inventory must be valid: %v", err)
	}
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	if _, err := detectRuntimeForTarget(context.Background(), resolved.Target); err == nil {
		t.Fatal("fixture must fail detection")
	}
	facts := map[string]application.ProviderFact{}
	if diagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, facts); diagnostic != nil {
		t.Fatalf("genuine detection failure must produce no materialization diagnostic: %v", diagnostic)
	}
	if len(facts) != 0 {
		t.Fatalf("runtime detection failure converted into measured absence: %+v", facts)
	}
}

// F1: an inspector failure after a successful detection also stays
// unverifiable; the registry may not substitute the failed measurement.
func TestCMF19InspectorFailureMustRemainUnverifiable(t *testing.T) {
	resolved := cmf19Fixture(t)
	cmf19BindAll(t, resolved)
	cmf19MaterializeCanonicalFiles(t, resolved)
	if _, _, _, err := nativeBackendInventory(resolved); err != nil {
		t.Fatalf("fixture inventory must be valid: %v", err)
	}
	original := detectRuntimeForTargetFunc
	t.Cleanup(func() { detectRuntimeForTargetFunc = original })
	detectRuntimeForTargetFunc = func(context.Context, deployment.ResolvedTarget) (bhruntime.RuntimeProvider, error) {
		return cmf19FailingInspector{}, nil
	}
	facts := map[string]application.ProviderFact{}
	if diagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, facts); diagnostic != nil {
		t.Fatalf("genuine inspection failure must produce no materialization diagnostic: %v", diagnostic)
	}
	if len(facts) != 0 {
		t.Fatalf("runtime inspection failure converted into measured absence: %+v", facts)
	}
}

type cmf19FailingInspector struct {
	bhruntime.RuntimeProvider
}

func (cmf19FailingInspector) Kind() bhruntime.ProviderKind { return "cmf19-failing" }

func (cmf19FailingInspector) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return nil, errors.New("cmf19 injected inspector outage")
}

// F2: an instance without an exact registry binding stays unverifiable even
// when the registry owner tuple matches and the runtime is unavailable.
func TestCMF19UnboundInstanceMustRemainUnverifiable(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	_, registry := cmf19Registry(t, resolved)
	if len(registry.Bindings) != 0 {
		t.Fatal("fixture must be unbound")
	}
	facts := map[string]application.ProviderFact{}
	observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	if len(facts) != 0 {
		t.Errorf("unbound registry instance produced verified facts: %+v", facts)
	}
	if err := checkRetainedApplicationProviderOwnership(context.Background(), resolved); err == nil {
		t.Error("unbound retained ownership admitted without runtime verification")
	} else if !containsCauseCode(err, "retained_provider_ownership_unverified") {
		t.Errorf("retained ownership error lost its machine cause: %v", err)
	}
}

// F2: foreign bindings (different application UUID, environment or logical
// resource) never qualify the instance for the missing-inventory fallback.
func TestCMF19ForeignBindingsMustRemainUnverifiable(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	_, registry := cmf19Registry(t, resolved)
	instance := registry.Instances[0]
	own := capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}
	for name, foreign := range map[string]capability.ProviderBinding{
		"foreign application": {Resource: own, ApplicationID: "00000000-0000-4000-8000-000000000000", Environment: resolved.Manifest.Environment, ProviderInstanceID: instance.ID},
		"foreign environment": {Resource: own, ApplicationID: resolved.Manifest.ApplicationID, Environment: "prod", ProviderInstanceID: instance.ID},
		"foreign resource":    {Resource: capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: "foreign-resource", Provider: capability.ProviderPostgreSQL}, ApplicationID: resolved.Manifest.ApplicationID, Environment: resolved.Manifest.Environment, ProviderInstanceID: instance.ID},
		"foreign capability":  {Resource: capability.Resource{Application: resolved.Manifest.Name, Kind: capability.KeyValue, Name: "default", Provider: capability.ProviderValkey}, ApplicationID: resolved.Manifest.ApplicationID, Environment: resolved.Manifest.Environment, ProviderInstanceID: instance.ID},
		"empty binding set":   {},
	} {
		if _, ok := nativeBackendBindingProof(capability.Registry{Instances: registry.Instances, Bindings: []capability.ProviderBinding{foreign}}, instance, resolved.Manifest); ok {
			t.Fatalf("%s binding was accepted as an exact registry binding", name)
		}
	}
	if _, ok := nativeBackendBindingProof(capability.Registry{Instances: registry.Instances, Bindings: []capability.ProviderBinding{{Resource: own, ApplicationID: resolved.Manifest.ApplicationID, Environment: resolved.Manifest.Environment, ProviderInstanceID: instance.ID}}}, instance, resolved.Manifest); !ok {
		t.Fatal("the exact own binding must be accepted as proof")
	}
	empty, emptyRegistry := cmf19Registry(t, resolved)
	emptyRegistry.Bindings = nil
	if err := empty.Save(emptyRegistry); err != nil {
		t.Fatal(err)
	}
	facts := map[string]application.ProviderFact{}
	observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	if len(facts) != 0 {
		t.Fatalf("foreign bindings reached recordRegistryBoundStoppedOwned: %+v", facts)
	}
}

// F2: neither a missing instance nor a manifest intent without the named
// logical resource may produce a fallback fact.
func TestCMF19MissingInstanceOrIntentMustRemainUnverifiable(t *testing.T) {
	t.Run("missing instance", func(t *testing.T) {
		resolved := cmf19Fixture(t)
		resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
		store, registry := cmf19Registry(t, resolved)
		registry.Instances = nil
		if err := store.Save(registry); err != nil {
			t.Fatal(err)
		}
		facts := map[string]application.ProviderFact{}
		observeNativeBackendProviderFacts(context.Background(), resolved, facts)
		if len(facts) != 0 {
			t.Fatalf("fact recorded for a removed registry instance: %+v", facts)
		}
	})
	t.Run("missing intent", func(t *testing.T) {
		resolved := cmf19Fixture(t)
		resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
		store, registry := cmf19Registry(t, resolved)
		instance := registry.Instances[0]
		plain := application.New("cmf19-unintent", "dev", false, false, false)
		plain.ApplicationID = resolved.Manifest.ApplicationID
		plain.Name = resolved.Manifest.Name
		instance.OwnerApplicationID = plain.ApplicationID
		instance.OwnerApplication = plain.Name
		registry.Instances = []capability.ProviderInstance{instance}
		if err := registry.BindDeployment(capability.Resource{Application: plain.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, plain.ApplicationID, "dev", instance.ID); err != nil {
			t.Fatal(err)
		}
		if err := store.Save(registry); err != nil {
			t.Fatal(err)
		}
		resolved.Manifest = plain
		facts := map[string]application.ProviderFact{}
		observeNativeBackendProviderFacts(context.Background(), resolved, facts)
		for id, fact := range facts {
			if fact.State == application.ProviderStoppedOwned {
				t.Fatalf("manifest without SQL intent received a stopped-owned fallback fact for %q", id)
			}
		}
	})
}

// F3: with the canonical runtime directory materialized, a missing
// runtime.env must not be misread as directory absence.
func TestCMF19MaterializedDirectoryMissingEnvMustNotProveAbsence(t *testing.T) {
	resolved := cmf19Fixture(t)
	cmf19BindAll(t, resolved)
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	compose, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	// runtime.env is missing: the runtime directory already exists.
	if _, _, _, err := nativeBackendInventory(resolved); !os.IsNotExist(err) {
		t.Fatalf("expected missing env, got %v", err)
	}
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	facts := map[string]application.ProviderFact{}
	diagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	if len(facts) != 0 {
		t.Fatalf("missing env misclassified as missing directory and measured absence: %+v", facts)
	}
	if diagnostic != nil {
		t.Fatalf("missing env must not surface a materialization diagnostic: %v", diagnostic)
	}
}

// F3: the wizard preview performs no CWD-relative lookup; observing the same
// canonical owner from two different working directories, one containing an
// unrelated matching relative state path, yields identical results.
func TestCMF19WizardPreviewObservationIsCWDIndependent(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	clean := t.TempDir()
	poisoned := t.TempDir()
	t.Chdir(poisoned)
	files := application.RuntimeFilesFor(application.Store{}, resolved.Manifest)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	compose, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	poisonedFacts := map[string]application.ProviderFact{}
	poisonedDiagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, poisonedFacts)
	t.Chdir(clean)
	cleanFacts := map[string]application.ProviderFact{}
	cleanDiagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, cleanFacts)
	if !reflect.DeepEqual(poisonedFacts, cleanFacts) {
		t.Fatalf("CWD changed wizard facts: poisoned=%+v clean=%+v", poisonedFacts, cleanFacts)
	}
	if (poisonedDiagnostic == nil) != (cleanDiagnostic == nil) {
		t.Fatalf("CWD changed wizard diagnostics: poisoned=%v clean=%v", poisonedDiagnostic, cleanDiagnostic)
	}
	if len(cleanFacts) == 0 {
		t.Fatal("wizard preview must still record the bound stopped-owned fact in both CWDs")
	}
}

// F4: every independently proven instance receives its own fact; the first
// match must not short-circuit the loop.
func TestCMF19TwoInstancesMustBothReceiveFacts(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Manifest = application.WithSQLInstances(resolved.Manifest, "default", "analytics")
	store, registry := cmf19Registry(t, resolved)
	second := registry.Instances[0]
	second.ID = application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "analytics")
	if err := registry.Register(second); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	cmf19BindAll(t, resolved)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	facts := map[string]application.ProviderFact{}
	observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	if len(facts) != 2 {
		t.Fatalf("only %d/2 projected instances observed: %+v", len(facts), facts)
	}
	for _, name := range []string{"default", "analytics"} {
		id := application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", name)
		if fact, ok := facts[id]; !ok || fact.State != application.ProviderStoppedOwned {
			t.Fatalf("instance %q (name %s) missing a stopped-owned fact: %+v ok=%v", id, name, fact, ok)
		}
	}
}

// F4: PostgreSQL and Valkey instances are handled through their own capability
// bindings and both receive independent facts.
func TestCMF19SQLAndValkeyInstancesMustBothReceiveFacts(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Manifest.Services.Cache = true
	store, registry := cmf19Registry(t, resolved)
	sqlInstance := registry.Instances[0]
	if err := registry.Register(capability.ProviderInstance{
		ID:                 application.ApplicationProviderInstanceIDForScopedValkey(resolved.Manifest.ApplicationID, "dev", "default"),
		Provider:           capability.Provider{Kind: capability.ProviderValkey, Capabilities: []capability.Kind{capability.KeyValue, capability.DurableKeyValue}},
		Scope:              capability.ScopeApplication,
		Ownership:          capability.OwnershipBaseHarbor,
		OwnerApplicationID: resolved.Manifest.ApplicationID,
		OwnerApplication:   resolved.Manifest.Name,
		OwnerEnvironment:   "dev",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, resolved.Manifest.ApplicationID, "dev", sqlInstance.ID); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: resolved.Manifest.Name, Kind: capability.KeyValue, Name: "default", Provider: capability.ProviderValkey}, resolved.Manifest.ApplicationID, "dev", application.ApplicationProviderInstanceIDForScopedValkey(resolved.Manifest.ApplicationID, "dev", "default")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	facts := map[string]application.ProviderFact{}
	diagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	if len(facts) != 2 {
		t.Fatalf("SQL and Valkey instances must both receive facts: %+v", facts)
	}
	if facts[sqlInstance.ID].State != application.ProviderStoppedOwned || facts[application.ApplicationProviderInstanceIDForScopedValkey(resolved.Manifest.ApplicationID, "dev", "default")].State != application.ProviderStoppedOwned {
		t.Fatalf("mixed provider facts lost their stopped-owned state: %+v", facts)
	}
	for _, instance := range []string{sqlInstance.ID, application.ApplicationProviderInstanceIDForScopedValkey(resolved.Manifest.ApplicationID, "dev", "default")} {
		if !containsMachineResource(diagnostic, instance) {
			t.Fatalf("diagnostic does not cover proven instance %q: %v", instance, diagnostic)
		}
	}
}

// F4: a mix of eligible and ineligible peers yields facts only for the proven
// instance; proof is never transferred.
func TestCMF19MixedEligibilityMustNotTransferProof(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Manifest = application.WithSQLInstances(resolved.Manifest, "default", "analytics")
	store, registry := cmf19Registry(t, resolved)
	second := registry.Instances[0]
	second.ID = application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "analytics")
	if err := registry.Register(second); err != nil {
		t.Fatal(err)
	}
	bound := application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "default")
	if err := registry.BindDeployment(capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, resolved.Manifest.ApplicationID, "dev", bound); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	facts := map[string]application.ProviderFact{}
	diagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	if len(facts) != 1 || facts[bound].State != application.ProviderStoppedOwned {
		t.Fatalf("unproven peer received a transferred fact: %+v", facts)
	}
	if !containsMachineResource(diagnostic, bound) {
		t.Fatalf("diagnostic missing for the proven instance: %v", diagnostic)
	}
	if containsMachineResource(diagnostic, second.ID) {
		t.Fatalf("diagnostic recorded for an unproven peer: %v", diagnostic)
	}
}

// F5: the materialization condition is a typed machine error with a stable
// cause/code and instance/target context; it decodes through machine JSON.
func TestCMF19MaterializationDiagnosticIsTypedMachineError(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	facts := map[string]application.ProviderFact{}
	diagnostic := observeNativeBackendProviderFacts(context.Background(), resolved, facts)
	var machineErr *machine.Error
	if !errors.As(diagnostic, &machineErr) {
		t.Fatalf("diagnostic is not a typed machine error: %v (%T)", diagnostic, diagnostic)
	}
	if machineErr.CauseCode != causeRuntimeInventoryNotYetMaterialized {
		t.Fatalf("unstable machine cause: %q", machineErr.CauseCode)
	}
	if machineErr.Code != machine.ErrorRuntimeUnavailable {
		t.Fatalf("unexpected machine code: %q", machineErr.Code)
	}
	if machineErr.Resource == "" {
		t.Fatal("diagnostic lost its instance context")
	}
	details, ok := machineErr.Details.(map[string]any)
	if !ok || details["target"] != resolved.Target.Name || details["instance"] != machineErr.Resource || details["application_id"] != resolved.Manifest.ApplicationID {
		t.Fatalf("diagnostic lost instance/target context: %+v", machineErr.Details)
	}
	raw, err := json.Marshal(machineErr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Code     string `json:"code"`
		Cause    string `json:"cause"`
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Cause != causeRuntimeInventoryNotYetMaterialized || decoded.Code != string(machine.ErrorRuntimeUnavailable) || decoded.Resource != machineErr.Resource {
		t.Fatalf("machine JSON lost the stable cause/code: %+v", decoded)
	}
}

// F5: the observation caller propagates the diagnostic through the structured
// error path; retained ownership is never authorized from it alone.
func TestCMF19RetainedOwnershipCannotBeAuthorizedByDiagnosticAlone(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	store, registry := cmf19Registry(t, resolved)
	bound := registry.Instances[0]
	retained := capability.ProviderInstance{
		ID:                 application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "retained"),
		Provider:           capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Scope:              capability.ScopeApplication,
		Ownership:          capability.OwnershipBaseHarbor,
		OwnerApplicationID: resolved.Manifest.ApplicationID,
		OwnerApplication:   resolved.Manifest.Name,
		OwnerEnvironment:   "dev",
	}
	if err := registry.Register(retained); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, resolved.Manifest.ApplicationID, "dev", bound.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	err := checkRetainedApplicationProviderOwnership(context.Background(), resolved)
	if err == nil {
		t.Fatal("retained unbound instance authorized although only a preview diagnostic exists")
	}
	if !containsCauseCode(err, "retained_provider_ownership_unverified") {
		t.Fatalf("ownership error lost its machine cause: %v", err)
	}
	if !containsCauseCode(err, causeRuntimeInventoryNotYetMaterialized) {
		t.Fatalf("caller lost the propagated materialization diagnostic: %v", err)
	}
}

func containsCauseCode(err error, cause string) bool {
	if err == nil {
		return false
	}
	var machineErr *machine.Error
	if errors.As(err, &machineErr) && machineErr != nil && machineErr.CauseCode == cause {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, element := range joined.Unwrap() {
			if containsCauseCode(element, cause) {
				return true
			}
		}
		return false
	}
	return containsCauseCode(errors.Unwrap(err), cause)
}

func containsMachineResource(err error, resource string) bool {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		var machineErr *machine.Error
		return errors.As(err, &machineErr) && machineErr.Resource == resource
	}
	for _, element := range joined.Unwrap() {
		if containsMachineResource(element, resource) {
			return true
		}
	}
	return false
}
