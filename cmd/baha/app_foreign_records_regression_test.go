package main

import (
	"encoding/json"
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

// 0427-144: provider resolution on a Target that carries foreign deployment
// records (records of OTHER applications whose provider realization is no
// longer registered) must either tolerate them cleanly or report the concrete
// consumer conflict with its next step. Silent blocking is a defect.

// twoAppMixedBackground places one configured sql application with a desired
// preference naming a shared realization the Target registry no longer carries,
// alongside a second, http-only application being created on the same Target.
// The second app's own requirement projection is empty, so the stale edge is a
// pure foreign-record residue.
func twoAppMixedBackground(t *testing.T) (deployment.ResolvedTarget, string, application.Manifest, application.Manifest) {
	t.Helper()
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := application.WithWorkloadComponents(application.New("foreign-sql", "dev", true, false, false), "api")
	foreignRoot := t.TempDir()
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  testDeploymentID,
			ApplicationID: foreign.ApplicationID,
			Target:        target.Name,
			Application:   "foreign-sql",
			Environment:   "dev",
		},
		Source:  deployment.DeploymentSource{Kind: "repository", Repository: filepath.Join(foreignRoot, "gone"), Manifest: "baseharbor.yaml"},
		Applied: deployment.AppliedDeployment{Intent: mustMarshalTestIntent(t, foreign), RuntimeProvider: target.RuntimeProvider},
	}
	record.DesiredProviderPreferences = mustMarshalTestIntent(t, []application.ProviderPreference{
		{Requirement: capability.Requirement{Kind: capability.SQL, Name: "default"}, InstanceID: "postgresql/shared/environment-dev-91df0423", ProviderKind: capability.ProviderPostgreSQL},
	})
	record.Observed.State = "configured"
	record.Observed.Ready = false
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "provider-registry.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	httpApp := application.WithWorkloadComponents(application.New("http-only", "dev", false, false, false), "api")
	httpApp = application.WithHTTPExposure(httpApp, "default", "api", 8080, "http")
	return target, root, foreign, httpApp
}

func mustMarshalTestIntent(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func foreignRecordResolvedApplication(t *testing.T, target deployment.ResolvedTarget, root string, m application.Manifest) resolvedApplication {
	t.Helper()
	return resolvedApplication{Target: target, Manifest: m, TargetStateRoot: root, SourceAvailable: true, ManifestPath: filepath.Join(t.TempDir(), "baseharbor.yaml")}
}

// TestForeignRecordDoesNotBlockHttpAppSelection proves clean tolerance on the
// wizard/edit selection surface: the http-only app's provider choices stay
// available (no silent stall) while a foreign record's stale SQL reference is
// reported per requirement.
func TestForeignRecordDoesNotBlockHttpAppSelection(t *testing.T) {
	target, root, _, httpApp := twoAppMixedBackground(t)
	resolved := foreignRecordResolvedApplication(t, target, root, httpApp)
	snapshot, _, err := applicationLifecycleSnapshot(resolved, nil)
	if err != nil {
		t.Fatalf("http-only app blocked by foreign record before selection: %v", err)
	}
	contract, err := application.PortableContractFromManifest(httpApp)
	if err != nil {
		t.Fatal(err)
	}
	choices, err := appProviderChoicesWithView(httpApp, snapshot, newAppCapabilitySelection(appCapabilitySelectionInput{Current: contract.Capabilities}), nil, true)
	if err != nil {
		t.Fatalf("wizard selection aborted on foreign record residue: %v", err)
	}
	if len(choices) != 1 || choices[0].Requirement.Kind != capability.ExposureHTTP {
		t.Fatalf("unexpected choices: %+v", choices)
	}
	if choices[0].ProposedInstanceID == "" {
		t.Fatalf("http exposure lost its own resolution under foreign residue: %+v", choices[0])
	}
}

// TestForeignRecordUnionReportsConcreteConflictWithNextStep proves the second
// arm of the acceptance disjunction: where the union plan cannot tolerate the
// residue, it fails closed naming the exact foreign consumer and the recovery
// step — never a bare "reconcile the inventory" hint.
func TestForeignRecordUnionReportsConcreteConflictWithNextStep(t *testing.T) {
	target, root, foreign, httpApp := twoAppMixedBackground(t)
	resolved := foreignRecordResolvedApplication(t, target, root, httpApp)
	_, err := resolveApplicationProviderPlan(resolved, nil)
	if err == nil {
		t.Fatal("stale foreign dependency edge accepted silently")
	}
	failure := machine.Classify(err)
	if failure.Code != machine.ErrorNotFound || failure.CauseCode != "provider_stale_consumer_dependency" {
		t.Fatalf("foreign residue lost its concrete attribution: %+v", failure)
	}
	if failure.Resource != "deployment/"+testDeploymentID {
		t.Fatalf("conflict did not name the foreign consumer record: %+v", failure)
	}
	if !strings.Contains(failure.Next, "postgresql/shared/environment-dev-91df0423") || !strings.Contains(failure.Next, "lifecycle owner") {
		t.Fatalf("next step is not actionable: %q", failure.Next)
	}
	var resolution *application.ResolutionError
	if !errors.As(err, &resolution) || resolution.Consumer != "deployment/"+testDeploymentID {
		t.Fatalf("typed consumer attribution lost: %v", err)
	}
	// The foreign consumer's OWN surface fails closed where its stored
	// preference is read through the canonical deployment owner: the live
	// Target registry does not carry that realization, so the selection is
	// rejected explicitly. (When its canonical placement happens to derive the
	// same instance id, the projection re-materializes the offer instead — the
	// resolution then fails closed at plan time as above.)
	record, found, err := deployment.FindDeployment(target.Name, foreign.ApplicationID, foreign.Environment)
	if err != nil || !found {
		t.Fatalf("foreign record: %v %v", found, err)
	}
	var prefs []application.ProviderPreference
	if err := json.Unmarshal(record.DesiredProviderPreferences, &prefs); err != nil {
		t.Fatal(err)
	}
	if len(prefs) != 1 || prefs[0].InstanceID != "postgresql/shared/environment-dev-91df0423" {
		t.Fatalf("foreign stored selection lost: %+v", prefs)
	}
	// The foreign app's own registry projection (its canonical placement owner
	// re-materializes its declared shared realization) must carry the
	// realization; the residue above never reaches a live Target where the
	// foreign app itself is reconciled.
	preview, previewErr := application.PreviewReferenceProviderRegistryAt(root, foreign)
	if previewErr != nil {
		t.Fatalf("foreign preview projection: %v", previewErr)
	}
	carried := false
	for _, instance := range preview.Instances {
		if instance.ID == prefs[0].InstanceID {
			carried = true
		}
	}
	if !carried {
		t.Fatalf("foreign app's own projection lost its declared shared realization: %+v", preview.Instances)
	}
}

// TestForeignRecordStaleReferenceDoesNotPoisonUnrelatedUnionPlan proves the
// second app's union planning still succeeds where the referenced realization
// is not required by anyone but is merely present as residue.
func TestForeignRecordStaleReferenceDoesNotPoisonUnrelatedUnionPlan(t *testing.T) {
	target, root, _, httpApp := twoAppMixedBackground(t)
	// The registry now DOES carry the realization the foreign record references
	// (nothing is actually missing): the residue must stay inert for the
	// unrelated http-only app and must not stall its plan.
	foreign := application.New("foreign-sql", "dev", true, false, false)
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("fixture records: %+v", records)
	}
	var prefs []application.ProviderPreference
	if err := json.Unmarshal(records[0].DesiredProviderPreferences, &prefs); err != nil {
		t.Fatal(err)
	}
	instance := capability.ProviderInstance{ID: prefs[0].InstanceID, Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "environment:dev"}
	registry := capability.NewRegistry()
	if err := registry.Register(instance); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "provider-registry.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	resolved := foreignRecordResolvedApplication(t, target, root, httpApp)
	plan, err := resolveApplicationProviderPlan(resolved, nil)
	if err != nil {
		t.Fatalf("registered-but-unconsumed foreign realization blocked the http-only plan: %v", err)
	}
	for _, change := range plan.Changes {
		if change.InstanceID == instance.ID && change.Action != "preserve" && !change.PreserveData {
			t.Fatalf("foreign residue authorized destructive action: %+v", change)
		}
	}
	_ = foreign
}
