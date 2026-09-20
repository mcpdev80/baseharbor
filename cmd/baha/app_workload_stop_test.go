package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
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
