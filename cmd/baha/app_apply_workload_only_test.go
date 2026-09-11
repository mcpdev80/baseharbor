package main

import (
	"context"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
	if err := startManagedRuntime(context.Background(), io.Discard, bhruntime.Compose{}, m, application.RuntimeFiles{}); err != nil {
		t.Fatalf("startManagedRuntime() error = %v", err)
	}
}
