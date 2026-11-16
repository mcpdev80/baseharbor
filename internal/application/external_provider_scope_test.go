package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

func externalScopeFixture(t *testing.T) (string, string) {
	t.Helper()
	root := protectedTempDir(t)
	reg := externalprovider.Registration{ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app", CredentialRef: "file:///run/secrets/db", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}
	if err := RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(root, providerRegistryFile)}).Load()
	if err != nil {
		t.Fatal(err)
	}
	id := registry.Instances[0].ID
	for _, target := range []string{"dev", "prod"} {
		store := capability.RegistryStore{Path: filepath.Join(root, "targets", target, providerRegistryFile)}
		if err := store.Update(func(r *capability.Registry) error {
			if err := r.Register(registry.Instances[0]); err != nil {
				return err
			}
			return r.BindDeployment(capability.Resource{Application: "web", Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, MustNewApplicationID(), target, id)
		}); err != nil {
			t.Fatal(err)
		}
	}
	return root, id
}

func TestExternalRemovalScopeRequiresAllTargetsAndTwoStages(t *testing.T) {
	root, _ := externalScopeFixture(t)
	plan, err := PlanExternalProviderRemovalAt(root, "company-db")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Scope.Targets) != 2 || plan.Scope.Complete || plan.Scope.ConfirmationsRequired != 2 {
		t.Fatalf("incorrect scope: %+v", plan.Scope)
	}
	for _, target := range plan.Scope.Targets {
		if len(target.Impacts) != 1 || len(target.Impacts[0].Functions) != 1 || len(target.Impacts[0].Effects) < 2 {
			t.Fatalf("missing actual binding consequences: %+v", target)
		}
	}
	_, err = ApplyExternalProviderRemovalAt(root, "company-db", ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true})
	if err == nil {
		t.Fatal("ordinary/one-stage approval removed shared bindings")
	}
	result, err := ApplyExternalProviderRemovalAt(root, "company-db", ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Removed || result.ForeignMutated || result.Scope.Status != "partial" {
		t.Fatalf("registration removal fabricated runtime continuity: %+v", result)
	}
	for _, target := range []string{"dev", "prod"} {
		r, err := (capability.RegistryStore{Path: filepath.Join(root, "targets", target, providerRegistryFile)}).Load()
		if err != nil || len(r.Bindings) != 0 || len(r.Instances) != 0 {
			t.Fatalf("target %s not unregistered: %+v %v", target, r, err)
		}
	}
}

func TestExternalRemovalConsentInvalidatedByNewTarget(t *testing.T) {
	root, _ := externalScopeFixture(t)
	plan, err := PlanExternalProviderRemovalAt(root, "company-db")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "targets", "later"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err = ApplyExternalProviderRemovalAt(root, "company-db", ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true})
	var changed *ProviderChangeError
	if !errors.As(err, &changed) || changed.Code != "stale-impact-consent" {
		t.Fatalf("new target did not invalidate consent: %v", err)
	}
	store, _ := externalProviderStoreAt(root)
	if _, err := store.Inspect("company-db"); err != nil {
		t.Fatal("stale consent mutated registration")
	}
}

func TestExternalRemovalRejectsUnattributedAndRedirectedOwners(t *testing.T) {
	root, id := externalScopeFixture(t)
	store := capability.RegistryStore{Path: filepath.Join(root, providerRegistryFile)}
	if err := store.Update(func(r *capability.Registry) error {
		return r.BindDeployment(capability.Resource{Application: "legacy", Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, MustNewApplicationID(), "dev", id)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanExternalProviderRemovalAt(root, "company-db"); err == nil {
		t.Fatal("unattributed owner treated as runtime Target")
	}
	root, _ = externalScopeFixture(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "targets", "redirected")); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanExternalProviderRemovalAt(root, "company-db"); err == nil {
		t.Fatal("redirected Target owner accepted")
	}
}

func TestExternalRemovalPlanningIsReadOnlyAndRejectsUnprotectedRegistry(t *testing.T) {
	root, _ := externalScopeFixture(t)
	path := filepath.Join(root, "targets", "dev", providerRegistryFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanExternalProviderRemovalAt(root, "company-db")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("planning changed protected owner")
	}
	material := plan.Scope.Targets[0].Impacts[0].Materials
	if len(material) != 1 || material[0].Before.Location.Reference != "file:///run/secrets/db" || material[0].Before.Verified {
		t.Fatalf("material location/unknown validity missing: %+v", material)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanExternalProviderRemovalAt(root, "company-db"); err == nil {
		t.Fatal("unprotected registry granted resource authority")
	}
}
