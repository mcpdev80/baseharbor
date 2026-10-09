package main

import (
	"bytes"
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryVolumePlanNamesEachPreservedResource(t *testing.T) {
	var out bytes.Buffer
	volumes := []bhruntime.RepositoryVolume{
		{Name: "bh-local-demo-dev_demo-app-state", Project: "bh-local-demo-dev", Owner: "not recorded", Reason: "persistent application data"},
		{Name: "shared", Project: "bh-local-demo-dev", Owner: "not recorded", Reason: "shared with another project", Shared: true},
		{Name: "foreign", Reason: "unverified ownership"},
	}
	renderRepositoryVolumePreservation(&out, volumes, "docker")
	for _, volume := range volumes {
		if !strings.Contains(out.String(), "PRESERVED volume "+volume.Name) || !strings.Contains(out.String(), volume.Reason) {
			t.Fatalf("missing precise preservation: %s", out.String())
		}
	}
	if strings.Count(out.String(), "docker volume rm") != 1 || strings.Contains(out.String(), "down -v") {
		t.Fatalf("unsafe cleanup advice: %s", out.String())
	}
}

func TestRepositoryVolumeObservationOutlivesAppStateAndRequiresConsent(t *testing.T) {
	root := t.TempDir()
	e := applicationDestroyExecution{resolved: resolvedApplication{TargetStateRoot: root}, files: application.RuntimeFiles{Project: "bh-test-app"}, repositoryVolumes: []bhruntime.RepositoryVolume{{Name: "external-data"}}}
	if err := e.destroyRuntimeResources(context.Background()); err == nil {
		t.Fatal("missing explicit consent")
	}
	if _, err := os.Stat(e.repositoryVolumeObservationPath()); !os.IsNotExist(err) {
		t.Fatal("unapproved destroy wrote observation")
	}
	if err := e.saveRepositoryVolumeObservation(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "applications")); err != nil {
		t.Fatal(err)
	}
	names, err := e.previousRepositoryVolumeNames()
	if err != nil || len(names) != 1 || names[0] != "external-data" {
		t.Fatalf("lost observation: %v %v", names, err)
	}
}
