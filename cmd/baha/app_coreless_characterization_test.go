package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// These tests characterize the existing workload-only realization before the
// Core prerequisite is changed. They deliberately bypass neither ownership nor
// provider readiness and do not use a live daemon.
func TestCorelessRuntimeFilesHaveNoBackendResources(t *testing.T) {
	m := application.New("workload", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "api")
	store := application.Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "isolated"}
	files, err := application.EnsureRuntime(context.Background(), nil, store, m)
	if err != nil {
		t.Fatal(err)
	}
	if got := application.ExpectedRuntimeResourcesForIdentity(m, files.Project, files.ResourceProject); len(got) != 0 {
		t.Fatalf("workload-only runtime acquired backend resources: %+v", got)
	}
	if application.RequiresRuntimeBroker(m) {
		t.Fatal("workload-only runtime requires a broker")
	}
	if err := application.CheckManagedRuntimeDefinition(files, m); err != nil {
		t.Fatal(err)
	}
	if _, err := application.ExistingRuntimeFiles(store, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte("services:\n  foreign:\n    image: foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := application.CheckManagedRuntimeDefinition(files, m); !errors.Is(err, application.ErrRuntimeDefinitionChanged) {
		t.Fatalf("foreign definition accepted: %v", err)
	}
}

func TestCorelessIntentWithoutWorkloadCannotMaterializeRuntime(t *testing.T) {
	m := application.New("empty", "dev", false, false, false)
	m.Services.SQL = false
	store := application.Store{Root: filepath.Join(t.TempDir(), "apps")}
	if _, err := application.EnsureRuntime(context.Background(), nil, store, m); err == nil {
		t.Fatal("empty intent materialized a fictitious runtime")
	}
	if _, err := os.Stat(application.RuntimeFilesFor(store, m).Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid intent mutated runtime state: %v", err)
	}
}

func TestCorelessUpSkipsCorePreflight(t *testing.T) {
	m := application.New("workload", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "api")
	e := applicationUpExecution{manifest: m}
	for _, check := range e.preflightChecks() {
		if check.Name == "BaseHarbor control-plane runtime" || check.Name == "managed service PKI" {
			if err := check.Run(context.Background()); err != nil {
				t.Fatalf("workload-only preflight required Core: %v", err)
			}
		}
	}
	if requiresDevelopmentGateway(m) || requiresManagedServiceIssuer(m) {
		t.Fatal("plain workload acquired implicit development gateway dependency")
	}
}
