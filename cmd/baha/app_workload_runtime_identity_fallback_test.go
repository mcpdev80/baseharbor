package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// runtimeIdentityFallbackProvider is the minimal RuntimeProvider seam
// repositoryWorkloadBindingPlan touches: it renders a scripted Compose JSON model.
type runtimeIdentityFallbackProvider struct {
	bhruntime.RuntimeProvider
	rendered string
}

func (p runtimeIdentityFallbackProvider) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return p.rendered, nil
}

// TestRepositoryWorkloadBindingPlanRejectsLegacyBindingOnlyService pins H-V37: a
// workload service that reaches the application runtime broker only through a
// legacy BASEHARBOR_RUNTIME_TOKEN_FILE binding in its own Compose (with no
// explicit runtime permission) must NOT be admitted to RuntimeIdentityServices.
// Admitting it downgrades a scoped service identity to the shared legacy app
// token, which the fail-closed contract forbids.
func TestRepositoryWorkloadBindingPlanRejectsLegacyBindingOnlyService(t *testing.T) {
	// m requires the broker via secrets but declares no runtime permission.
	m := application.New("demo", "dev", true, false, true)

	// A repository-owned service "api" sets BASEHARBOR_RUNTIME_TOKEN_FILE but
	// has no explicit runtime permission.
	rendered := `{"services":{"api":{"image":"example/api","environment":{"BASEHARBOR_RUNTIME_TOKEN_FILE":"/run/secrets/baseharbor-runtime-token"}}}}`

	plan, err := repositoryWorkloadBindingPlan(
		t.Context(),
		runtimeIdentityFallbackProvider{rendered: rendered},
		resolvedApplication{Manifest: m},
		application.WorkloadFiles{Services: []string{"api"}},
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("repositoryWorkloadBindingPlan() error = %v", err)
	}

	if len(plan.RuntimeIdentityServices) != 0 {
		t.Fatalf("legacy-binding-only service must not be admitted to RuntimeIdentityServices, got %v", plan.RuntimeIdentityServices)
	}
}

// TestRepositoryWorkloadBindingPlanAdmitsExplicitPermissionService confirms the
// positive path: a service named in an explicit runtime permission is admitted,
// even though its Compose lacks the legacy binding.
func TestRepositoryWorkloadBindingPlanAdmitsExplicitPermissionService(t *testing.T) {
	m := application.WithRuntimePermission(
		application.New("demo", "dev", false, false, true),
		"object-storage.s3/v1",
		[]string{"api"},
		"runtime.create",
	)

	rendered := `{"services":{"api":{"image":"example/api"}}}`

	plan, err := repositoryWorkloadBindingPlan(
		t.Context(),
		runtimeIdentityFallbackProvider{rendered: rendered},
		resolvedApplication{Manifest: m},
		application.WorkloadFiles{Services: []string{"api"}},
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("repositoryWorkloadBindingPlan() error = %v", err)
	}

	if len(plan.RuntimeIdentityServices) != 1 || plan.RuntimeIdentityServices[0] != "api" {
		t.Fatalf("explicit permission service must be admitted to RuntimeIdentityServices, got %v", plan.RuntimeIdentityServices)
	}
}
