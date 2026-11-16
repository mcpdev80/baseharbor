package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRemoteApplicationNeverResolvesALocalRuntime(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, access := range []string{"baseharbor-node-connector", "native-api", "vendor/remote"} {
		for _, runtime := range []string{"docker", "podman"} {
			resolved := resolvedApplication{Target: deployment.ResolvedTarget{Name: "remote", RuntimeProvider: runtime, AccessProvider: access, AccessReference: "node-a"}}
			provider, err := detectRuntimeForApplication(context.Background(), resolved, bhruntime.CapabilityWorkloadLifecycle)
			var problem *machine.Error
			if provider != nil || !errors.As(err, &problem) || problem.Code != machine.ErrorCapabilityMissing {
				t.Fatalf("remote application reached local runtime resolution: %s/%s: %v", access, runtime, err)
			}
		}
	}
}

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
