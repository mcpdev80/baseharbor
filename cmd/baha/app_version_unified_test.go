package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

// TestAppVersionUnifiedRowsAndSelectorCallable proves the unified view is
// callable end to end (card 0428-062): a repository deployment with a
// delivered portable source model renders the separated cells and the source
// provenance rows through `app version -o json`, and -c narrows or fails
// closed without ever rendering a parallel version story.
func TestAppVersionUnifiedRowsAndSelectorCallable(t *testing.T) {
	configureTestTarget(t)
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, application.RepositoryManifestName)
	parsed := application.WithWorkloadComponents(application.New("unified", "dev", false, false, false), "api")
	if err := os.WriteFile(manifestPath, []byte(parsed.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceDir := filepath.Join(repo, "src")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion,
		Application:   parsed.Name,
		Sources:       []development.SourceDefinition{{ID: "main", Type: development.SourceRepository, Repository: "https://example.invalid/unified"}},
		Components:    []development.ComponentSource{{Component: "api", Source: "main"}},
	}
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	if _, err := development.SaveWorkspaceMapping(manifestPath, development.WorkspaceMapping{
		SchemaVersion: development.WorkspaceMappingVersion, Application: parsed.Name, Manifest: manifestPath,
		Sources: map[string]string{"main": sourceDir},
	}); err != nil {
		t.Fatal(err)
	}
	identity, err := deployment.NewDeploymentIdentity("docker-dev", parsed.ApplicationID, parsed.Name, parsed.Environment)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Intent: mustIntentSnapshotJSON(t, parsed)},
		References: map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:intended"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:applied"),
			deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "sha256:applied"),
		},
		Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}

	t.Chdir(repo)
	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "version", "-o", "json"}, &out, &errOut); err != nil {
		t.Fatalf("app version must stay callable: %v\n%s", err, errOut.String())
	}
	var payload struct {
		View struct {
			Stages struct {
				Wanted   map[string]any `json:"wanted"`
				Applied  map[string]any `json:"applied"`
				Observed map[string]any `json:"observed"`
			} `json:"stages"`
		} `json:"view"`
		Components []machinereadmodels.ComponentVersion `json:"components"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if ref, _ := payload.View.Stages.Wanted["reference"].(string); ref != "sha256:intended" {
		t.Fatalf("the wanted cell must keep the recorded intended dimension: %s", out.String())
	}
	if ref, _ := payload.View.Stages.Observed["reference"].(string); ref != "sha256:applied" {
		t.Fatalf("the observed cell must keep its own recorded dimension: %s", out.String())
	}
	api := componentRowIndex(payload.Components, "api")
	postgres := componentRowIndex(payload.Components, "postgres")
	_ = postgres
	if api < 0 {
		t.Fatalf("declared components must be present: %s", out.String())
	}
	if payload.Components[api].Source != "main" || payload.Components[api].SourceIdentity != sourceDir {
		t.Fatalf("the source-model provenance must flow into the row: %+v", payload.Components[api])
	}
	if payload.Components[api].Image != "" || !strings.Contains(payload.Components[api].Observation, "workload") {
		t.Fatalf("BaseHarbor must not invent an application-owned image version: %+v", payload.Components[api])
	}

	// -c narrows to the selected component rows only.
	out.Reset()
	errOut.Reset()
	if err := runWithIO(context.Background(), []string{"app", "version", "-c", "api", "-o", "json"}, &out, &errOut); err != nil {
		t.Fatalf("-c must stay callable: %v\n%s", err, errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Components) != 1 || payload.Components[0].Service != "api" {
		t.Fatalf("-c must narrow the rows exactly: %s", out.String())
	}

	// An unknown -c selection fails closed on the version face too.
	out.Reset()
	errOut.Reset()
	if err := runWithIO(context.Background(), []string{"app", "version", "-c", "ghost"}, &out, &errOut); err == nil {
		t.Fatalf("an unknown -c selection must fail closed:\n%s", out.String())
	}
}

// TestManifestSQLFlagRoundTripsThroughAuthoredYAML guards the assumption the
// unified component rows depend on: the authored YAML must round-trip the
// declared SQL service, or every view would silently lose the backend row.
func TestManifestSQLFlagRoundTripsThroughAuthoredYAML(t *testing.T) {
	m := application.New("roundtrip", "dev", true, false, false)
	m = application.WithWorkloadComponents(m, "api")
	path := filepath.Join(t.TempDir(), application.RepositoryManifestName)
	if err := os.WriteFile(path, []byte(m.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := application.LoadManifestFile(path)
	if err != nil {
		t.Fatalf("authored YAML must reload: %v\n%s", err, m.AuthoredYAML())
	}
	if !parsed.Services.SQL {
		t.Fatalf("the authored SQL declaration must round-trip:\n%s", m.AuthoredYAML())
	}
	if len(parsed.Workload.Components) != 1 || parsed.Workload.Components[0] != "api" {
		t.Fatalf("workload components must round-trip:\n%s", m.AuthoredYAML())
	}
}
