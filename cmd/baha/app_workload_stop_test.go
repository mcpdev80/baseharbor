package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRepositoryWorkloadStopEnvironmentUsesSecretPlaceholders(t *testing.T) {
	resolved := resolvedApplication{
		Manifest: application.Manifest{
			Name:        "demo",
			Environment: "dev",
			Secrets: application.SecretRequirements{
				Required: []application.SecretRequirement{
					{Name: "SECRET_KEY"},
					{Name: "TOKEN_FILE"},
				},
			},
		},
	}
	env, err := repositoryWorkloadStopEnvironment(resolved, application.RuntimeFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if got := env["SECRET_KEY"]; got != "baseharbor-preflight-secret" {
		t.Fatalf("SECRET_KEY placeholder = %q", got)
	}
	if got := env["TOKEN_FILE"]; got != "/run/baseharbor/preflight/TOKEN_FILE" {
		t.Fatalf("TOKEN_FILE placeholder = %q", got)
	}
}


func TestWorkloadRuntimeCleanupResourcesPreservesVolumes(t *testing.T) {
	resources := []bhruntime.ProjectResource{
		{Kind: "container", Name: "demo-api"},
		{Kind: "network", Name: "demo_default"},
		{Kind: "volume", Name: "demo_data"},
	}
	got := workloadRuntimeCleanupResources(resources)
	if len(got) != 2 {
		t.Fatalf("cleanup resources = %#v, want container+network only", got)
	}
	for _, resource := range got {
		if resource.Kind == "volume" {
			t.Fatalf("repository-owned volume must be preserved: %#v", got)
		}
	}
}
