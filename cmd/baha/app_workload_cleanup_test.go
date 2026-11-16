package main

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestVerifyRepositoryWorkloadCleanupAllowsPreservedNetworks(t *testing.T) {
	if err := verifyRepositoryWorkloadCleanup([]bhruntime.ProjectResource{
		{Kind: "network", Name: "observability"},
	}); err != nil {
		t.Fatalf("preserved network rejected: %v", err)
	}
}

func TestVerifyRepositoryWorkloadCleanupRejectsRemainingContainers(t *testing.T) {
	if err := verifyRepositoryWorkloadCleanup([]bhruntime.ProjectResource{
		{Kind: "container", Name: "demo-app"},
	}); err == nil {
		t.Fatal("remaining workload container was accepted")
	}
}
