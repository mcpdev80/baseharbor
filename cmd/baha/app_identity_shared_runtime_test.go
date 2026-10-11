package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Extend the already isolated selected-Identity gate on its same Target. The
// second app consumes the same actual provider, not a fabricated registry row.
func qualifySharedIdentityConsumers(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, first resolvedApplication, originalEnv [32]byte, volumes []bhruntime.ProjectResource) {
	t.Helper()
	firstRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	siblingRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(siblingRoot, "compose.yaml"), []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	writeSelectiveIdentityIntent(t, siblingRoot, "selective-sibling", true)
	var sibling resolvedApplication
	inSelectiveIdentityRepository(t, siblingRoot, func() {
		var output bytes.Buffer
		if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
			t.Fatalf("initialize sibling: %v\n%s", err, output.String())
		}
		sibling = applySelectiveIdentityIntent(t, ctx)
	})
	t.Cleanup(func() {
		inSelectiveIdentityRepository(t, siblingRoot, func() {
			if err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
				t.Errorf("isolated sibling cleanup: %v", err)
			}
		})
	})
	if sibling.Manifest.ApplicationID == first.Manifest.ApplicationID {
		t.Fatal("two independently initialized apps have the same system identity")
	}
	writeSelectiveIdentityIntent(t, firstRoot, first.Manifest.Name, false)
	applySelectiveIdentityIntent(t, ctx)
	inSelectiveIdentityRepository(t, siblingRoot, func() {
		current, err := resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready {
			t.Fatalf("removal at first app broke sibling Identity readiness: %+v %v", status, err)
		}
	})
	files, expected, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(first.Manifest, capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}, first.TargetStateRoot, first.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	assertSharedIdentityNativeState(t, ctx, provider, files, expected, volumes, true)
	t.Log("two apps: first removal preserves sibling OIDC/provider readiness and the shared native Identity module")
	writeSelectiveIdentityIntent(t, siblingRoot, sibling.Manifest.Name, false)
	inSelectiveIdentityRepository(t, siblingRoot, func() { applySelectiveIdentityIntent(t, ctx) })
	assertSharedIdentityNativeState(t, ctx, provider, files, expected, volumes, false)
	if data, err := os.ReadFile(files.Env); err != nil || sha256.Sum256(data) != originalEnv {
		t.Fatal("last-consumer removal replaced retained Identity credential/configuration material")
	}
	t.Logf("two apps: last removal converged selected Identity runtime to zero; retained owned data volumes=%d", len(volumes))
}

func writeSelectiveIdentityIntent(t *testing.T, root, name string, enabled bool) {
	t.Helper()
	content := fmt.Sprintf("version: 1\napp:\n  name: %s\n  environment: dev\nservices:\n  identity: %t\n", name, enabled)
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func inSelectiveIdentityRepository(t *testing.T, root string, action func()) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore isolated repository: %v", err)
		}
	}()
	action()
}

func applySelectiveIdentityIntent(t *testing.T, ctx context.Context) resolvedApplication {
	t.Helper()
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if impact := plan.ProviderPlan.Impact; impact != nil {
		ctx = withProviderChangeConsent(ctx, &application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true})
	}
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply", "--skip-memory-preflight"}, &output, &output); err != nil {
		t.Logf("shared Identity failed common union: complete=%t changes=%+v", plan.ProviderPlan.Complete, plan.ProviderPlan.Changes)
		t.Fatalf("shared Identity apply: %v\n%s", err, output.String())
	}
	return resolved
}

func assertSharedIdentityNativeState(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, files identityprovider.KeycloakFiles, expected, volumes []bhruntime.ProjectResource, needed bool) {
	t.Helper()
	actual, err := provider.InspectProjectResources(ctx, files.Project, expected)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, resource := range actual {
		if resource.Kind == "container" {
			count++
		}
	}
	if (needed && count == 0) || (!needed && count != 0) {
		t.Fatalf("shared provider required=%t actual selected containers=%d", needed, count)
	}
	for _, volume := range volumes {
		if !application.ResourceNamedExists(actual, volume) {
			t.Fatalf("shared removal lost retained owned volume %s", volume.Name)
		}
	}
}
