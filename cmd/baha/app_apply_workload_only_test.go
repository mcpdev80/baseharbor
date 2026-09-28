package main

import (
	"context"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	dockerprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/docker"
)

func TestStartManagedRuntimeSkipsWorkloadOnlyApplication(t *testing.T) {
	m := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "awc",
		Environment: "production",
		Workload: application.WorkloadConfig{
			Compose:  "docker-compose.yml",
			Services: []string{"coordinator", "docker-engine", "web"},
		},
	}
	if err := startManagedRuntime(context.Background(), io.Discard, dockerprovider.Provider{}, m, application.RuntimeFiles{}); err != nil {
		t.Fatalf("startManagedRuntime() error = %v", err)
	}
}
