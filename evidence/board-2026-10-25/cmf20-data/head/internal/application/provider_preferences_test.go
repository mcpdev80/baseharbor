package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func preferenceFixture(t *testing.T, target string) (Manifest, string, deployment.DeploymentRecord) {
	t.Helper()
	m := New("preference-owner", "dev", true, false, false)
	m.ApplicationID = "11111111-1111-4111-8111-111111111111"
	m = WithSQLInstances(m, "primary", "analytics")
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	id, err := deployment.NewDeploymentIdentity(target, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, _ := json.Marshal(m)
	record := deployment.DeploymentRecord{Identity: id, Applied: deployment.AppliedDeployment{Intent: intent}, Observed: deployment.ObservedDeployment{State: "configured"}}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	for _, key := range []string{"primary", "analytics"} {
		if err := registry.Register(capability.ProviderInstance{ID: "postgresql/shared/" + key, Provider: capability.PostgreSQL, Scope: capability.ScopeShared, SharingBoundary: "team:" + key, Ownership: capability.OwnershipBaseHarbor}); err != nil {
			t.Fatal(err)
		}
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, providerRegistryFile)}).Save(registry); err != nil {
		t.Fatal(err)
	}
	return m, root, record
}

func TestDesiredProviderPreferencesSurviveReconciliationAndReload(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, root, record := preferenceFixture(t, "preference-a")
	prefs := []ProviderPreference{
		{Requirement: capability.Requirement{Kind: capability.SQL, Name: "primary"}, InstanceID: "postgresql/shared/primary", ProviderKind: capability.ProviderPostgreSQL},
		{Requirement: capability.Requirement{Kind: capability.SQL, Name: "analytics"}, InstanceID: "postgresql/shared/analytics", ProviderKind: capability.ProviderPostgreSQL},
	}
	if err := SaveDesiredProviderPreferencesAt(root, "preference-a", m, prefs); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, providerRegistryFile))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		planned, err := PreviewReferenceProviderRegistryAt(root, m)
		if err != nil {
			t.Fatal(err)
		}
		for _, pref := range prefs {
			found := false
			for _, binding := range planned.Bindings {
				if binding.ApplicationID == m.ApplicationID && binding.Resource.Name == pref.Requirement.Name && binding.ProviderInstanceID == pref.InstanceID {
					found = true
				}
			}
			if !found {
				t.Fatalf("exact preference lost in preview: %+v", pref)
			}
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, providerRegistryFile))
	if string(before) != string(after) {
		t.Fatal("preview wrote registry")
	}
	for i := 0; i < 2; i++ {
		if err := ReconcileReferenceProviderRegistryAt(root, m); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := DesiredProviderPreferencesAt(root, m)
	if err != nil || !reflect.DeepEqual(loaded, prefs) {
		t.Fatalf("reload: %+v %v", loaded, err)
	}
	// Older caller record reconstruction must not erase the separately owned intent.
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	loaded, err = DesiredProviderPreferencesAt(root, m)
	if err != nil || !reflect.DeepEqual(loaded, prefs) {
		t.Fatal("legacy save erased desired preferences", err)
	}
	record, err = deployment.LoadDeploymentRecord(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if record.Observed.Ready || !record.Observed.VerifiedAt.IsZero() {
		t.Fatal("preference selection fabricated verified runtime")
	}
	if err := SaveDesiredProviderPreferencesAt(root, "preference-a", m, []ProviderPreference{}); err != nil {
		t.Fatal(err)
	}
	loaded, err = DesiredProviderPreferencesAt(root, m)
	if err != nil || len(loaded) != 0 {
		t.Fatal("automatic reset failed", err)
	}
}

func TestDesiredProviderPreferencesValidateIdentityIsolationAndStaleSelection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, root, _ := preferenceFixture(t, "preference-a")
	_, otherRoot, _ := preferenceFixture(t, "preference-b")
	foreignStore := capability.RegistryStore{Path: filepath.Join(root, providerRegistryFile)}
	foreignRegistry, err := foreignStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	foreignInstance := capability.ProviderInstance{ID: "postgresql/application/foreign", Provider: capability.PostgreSQL, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: "22222222-2222-4222-8222-222222222222", OwnerApplication: "foreign", OwnerEnvironment: "dev"}
	if err := foreignRegistry.Register(foreignInstance); err != nil {
		t.Fatal(err)
	}
	if err := foreignStore.Save(foreignRegistry); err != nil {
		t.Fatal(err)
	}
	base := ProviderPreference{Requirement: capability.Requirement{Kind: capability.SQL, Name: "primary"}, InstanceID: "postgresql/shared/primary"}
	for _, tc := range []struct {
		name  string
		prefs []ProviderPreference
	}{
		{"foreign", []ProviderPreference{{Requirement: base.Requirement, InstanceID: foreignInstance.ID}}},
		{"missing", []ProviderPreference{{Requirement: base.Requirement, InstanceID: "missing"}}},
		{"incompatible", []ProviderPreference{{Requirement: base.Requirement, InstanceID: base.InstanceID, ProviderKind: capability.ProviderValkey}}},
		{"unknown-name", []ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "unknown"}, InstanceID: base.InstanceID}}},
		{"duplicate", []ProviderPreference{base, base}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := SaveDesiredProviderPreferencesAt(root, "preference-a", m, tc.prefs); err == nil {
				t.Fatal("invalid preference accepted")
			}
		})
	}
	if err := SaveDesiredProviderPreferencesAt(root, "preference-b", m, []ProviderPreference{base}); err == nil {
		t.Fatal("foreign Target root accepted")
	}
	if err := SaveDesiredProviderPreferencesAt(root, "preference-a", m, []ProviderPreference{base}); err != nil {
		t.Fatal(err)
	}
	other, err := DesiredProviderPreferencesAt(otherRoot, m)
	if err != nil || len(other) != 0 {
		t.Fatal("preference crossed Targets", err)
	}
	foreign := m
	foreign.Environment = "test"
	other, err = DesiredProviderPreferencesAt(root, foreign)
	if err != nil || len(other) != 0 {
		t.Fatal("preference crossed environments", err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, providerRegistryFile)}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var kept []capability.ProviderInstance
	for _, instance := range registry.Instances {
		if instance.ID != base.InstanceID {
			kept = append(kept, instance)
		}
	}
	registry.Instances = kept
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	if _, err := PreviewReferenceProviderRegistryAt(root, m); err == nil {
		t.Fatal("stale preference silently fell back to another provider")
	}
}
