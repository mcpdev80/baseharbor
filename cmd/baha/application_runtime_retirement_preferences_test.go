package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// This is a read-only production destruction-plan regression, not native
// execution evidence. No runtime or inventory authority is replaced.
func TestGuidedWithdrawalPreferencesPreserveCanonicalOwners(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	target := configureTestTarget(t)
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Targets["other-dev"] = cfg.Targets[target.Name]
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	other, err := cfg.ResolveTarget("other-dev", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	var selected resolvedApplication
	var preferred string
	for index, current := range []deployment.ResolvedTarget{target, other} {
		manifest := application.WithSQLInstances(application.New("retirement-preference", "dev", false, false, false), "primary")
		manifest.ApplicationID = application.MustNewApplicationID()
		// Missing source deliberately uses the actual protected applied intent.
		missing := filepath.Join(t.TempDir(), "unavailable")
		record := registerTestDeployment(t, current, manifest, missing, filepath.Join(missing, application.RepositoryManifestName))
		root, err := deployment.TargetStateRoot(current.Name)
		if err != nil {
			t.Fatal(err)
		}
		if err := application.ReconcileReferenceProviderRegistryAt(root, manifest); err != nil {
			t.Fatal(err)
		}
		registryPath := filepath.Join(root, "provider-registry.json")
		registry, err := (capability.RegistryStore{Path: registryPath}).Load()
		if err != nil {
			t.Fatal(err)
		}
		instance := registry.Bindings[0].ProviderInstanceID
		preferences := []application.ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "primary"}, InstanceID: instance, ProviderKind: capability.ProviderPostgreSQL}}
		if err := application.SaveDesiredProviderPreferencesAt(root, current.Name, manifest, preferences); err != nil {
			t.Fatal(err)
		}
		deploymentRoot, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			t.Fatal(err)
		}
		recovery := filepath.Join(deploymentRoot, "retained-recovery")
		if err := os.WriteFile(recovery, []byte("protected-recovery-marker"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Join(deploymentRoot, "deployment.json"), registryPath, recovery)
		if index == 0 {
			selected, err = resolveRegisteredApplication(context.Background(), current, root, manifest.Name, manifest.Environment, "destroy")
			if err != nil {
				t.Fatal(err)
			}
			preferred = instance
		}
	}
	before := map[string][]byte{}
	for _, path := range paths {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	// A genuine unrequested authored/preferred capability still fails closed.
	invalid := applicationRuntimeRetirementIntent(selected.Manifest)
	invalid = application.WithProviderPreferences(invalid, []application.ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "primary"}, InstanceID: preferred, ProviderKind: capability.ProviderPostgreSQL}})
	if _, err := application.PreviewReferenceProviderRegistryAt(selected.TargetStateRoot, invalid); err == nil || !strings.Contains(err.Error(), "unrequested capability") {
		t.Fatalf("unrequested preference bypassed original validation: %v", err)
	}
	withdrawn, retirement, err := buildApplicationRuntimeRetirementPlan(context.Background(), selected)
	if err != nil {
		t.Fatalf("actual withdrawal planner retained obsolete desired preference: %v", err)
	}
	preferences, err := application.DesiredProviderPreferencesAt(selected.TargetStateRoot, withdrawn.Manifest)
	if err != nil || len(preferences) != 0 {
		t.Fatalf("withdrawal still requests a provider: %v", err)
	}
	released := false
	for _, binding := range retirement.ReleasedBindings {
		released = released || binding.ProviderInstanceID == preferred
	}
	if !released {
		t.Fatal("withdrawal lost the actual preferred provider's release identity")
	}
	// Full installation destruction uses the same child retirement authority.
	impact, err := buildRuntimeDestroyImpact(context.Background(), []targetDestroyInventory{{Target: target}}, nil, true)
	if err != nil {
		t.Fatalf("full destroy child plan rejects saved preference: %v", err)
	}
	if len(impact.Applications) != 1 {
		t.Fatalf("full destroy selected unrelated Target applications: %d", len(impact.Applications))
	}
	if impact.Applications[0].Providers.Target != target.Name {
		t.Fatal("destruction child provider plan crossed Target boundary")
	}
	for _, scopedTarget := range impact.Applications[0].Scope.Targets {
		if scopedTarget.Target != target.Name {
			t.Fatal("destruction scope crossed Target boundary")
		}
	}
	for _, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before[path], after) {
			t.Fatalf("read-only withdrawal changed canonical owner, registry or recovery: %s: %v", path, err)
		}
	}
	record, err := deployment.LoadDeploymentRecord(selected.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	var preserved []application.ProviderPreference
	if err := json.Unmarshal(record.DesiredProviderPreferences, &preserved); err != nil || len(preserved) != 1 || preserved[0].InstanceID != preferred {
		t.Fatalf("withdrawal erased protected preference: %v", err)
	}
}
