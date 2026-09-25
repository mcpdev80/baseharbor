package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRuntimeProviderKindForApplicationUsesTargetProvider(t *testing.T) {
	tests := map[string]bhruntime.ProviderKind{
		"docker":     bhruntime.ProviderDocker,
		"podman":     bhruntime.ProviderPodman,
		"kubernetes": bhruntime.ProviderKubernetes,
		"openshift":  bhruntime.ProviderOpenShift,
	}
	for provider, want := range tests {
		resolved := resolvedApplication{
			Target: deployment.ResolvedTarget{Name: provider + "-dev", RuntimeProvider: provider},
		}
		got, err := runtimeProviderKindForApplication(resolved)
		if err != nil {
			t.Fatalf("%s: %v", provider, err)
		}
		if got != want {
			t.Fatalf("%s: provider = %q, want %q", provider, got, want)
		}
	}
}

func TestRuntimeProviderKindForApplicationRejectsMissingTargetProvider(t *testing.T) {
	_, err := runtimeProviderKindForApplication(resolvedApplication{
		Target: deployment.ResolvedTarget{Name: "broken"},
	})
	if err == nil {
		t.Fatal("missing target runtime provider unexpectedly accepted")
	}
}
