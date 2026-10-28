package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestWorkloadSecurityPreflightEnvironmentUsesDeclaredSecretPlaceholders(t *testing.T) {
	m := application.New("mailflow", "production", false, false, true)
	m.Secrets.Required = []application.SecretRequirement{
		{Name: "SECRET_KEY"},
		{Name: "TLS_KEY_FILE"},
	}

	got := workloadSecurityPreflightEnvironment(m)
	if got["SECRET_KEY"] != "baseharbor-preflight-secret" {
		t.Fatalf("SECRET_KEY placeholder = %q", got["SECRET_KEY"])
	}
	if got["TLS_KEY_FILE"] != "/run/baseharbor/preflight/TLS_KEY_FILE" {
		t.Fatalf("TLS_KEY_FILE placeholder = %q", got["TLS_KEY_FILE"])
	}
	if len(got) != 2 {
		t.Fatalf("placeholder environment = %#v", got)
	}
}

func TestWorkloadSecurityPreflightEnvironmentDoesNotInventUndeclaredVariables(t *testing.T) {
	m := application.New("demo", "dev", false, false, false)
	if got := workloadSecurityPreflightEnvironment(m); got != nil {
		t.Fatalf("expected nil environment without declared required secrets, got %#v", got)
	}
}
