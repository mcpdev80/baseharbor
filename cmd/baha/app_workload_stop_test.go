package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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

func TestWorkloadRuntimeCleanupResourcesPreservesVolumes(t *testing.T) {
	resources := []bhruntime.ProjectResource{
		{Kind: "container", Name: "demo-api"},
		{Kind: "network", Name: "demo_default"},
		{Kind: "volume", Name: "demo_data"},
	}
	got := workloadRuntimeCleanupResources(resources)
	if len(got) != 2 {
		t.Fatalf("cleanup resources = %#v, want container+network only", got)
	}
	for _, resource := range got {
		if resource.Kind == "volume" {
			t.Fatalf("repository-owned volume must be preserved: %#v", got)
		}
	}
}

type backupWorkloadStopRuntime struct {
	bhruntime.RuntimeProvider
	kind                   bhruntime.ProviderKind
	containers             []bhruntime.RuntimeContainer
	selected, composeFiles []string
	project, workdir       string
	inspectErr, stopErr    error
	legacyStops, lists     int
	partialStop            bool
}

func (r *backupWorkloadStopRuntime) Kind() bhruntime.ProviderKind { return r.kind }
func (r *backupWorkloadStopRuntime) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return `{"services":{"api":{},"worker":{}}}`, nil
}
func (r *backupWorkloadStopRuntime) StopOwnedProjectContainers(context.Context, string) error {
	r.legacyStops++
	return errors.New("project-wide stop must never be used for a repository module")
}
func (r *backupWorkloadStopRuntime) InspectProjectResources(_ context.Context, project string, expected []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	if r.inspectErr != nil {
		return nil, r.inspectErr
	}
	var found []bhruntime.ProjectResource
	for _, resource := range expected {
		for _, c := range r.containers {
			if bhruntime.ContainerMatchesProjectResource(r.kind, c, project, resource) {
				found = append(found, resource)
			}
		}
	}
	return found, nil
}
func (r *backupWorkloadStopRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	r.lists++
	return append([]bhruntime.RuntimeContainer(nil), r.containers...), nil
}
func (r *backupWorkloadStopRuntime) StopProjectFilesSelected(_ context.Context, project, workdir string, _ map[string]string, services []string, composeFiles ...string) error {
	r.project, r.workdir = project, workdir
	r.selected, r.composeFiles = append([]string(nil), services...), append([]string(nil), composeFiles...)
	if r.stopErr != nil && !r.partialStop {
		return r.stopErr
	}
	var remaining []bhruntime.RuntimeContainer
	for _, c := range r.containers {
		selected := false
		for _, service := range services {
			selected = selected || c.Project == project && c.Service == service
		}
		if selected && (!r.partialStop || c.Service == "api") {
			if r.kind == bhruntime.ProviderPodman {
				continue
			}
			c.Running, c.State = false, "exited"
		}
		remaining = append(remaining, c)
	}
	r.containers = remaining
	return r.stopErr
}

func TestGuidedRepositoryWorkloadBackupStopPreservesCanonicalBackendAndOtherTargetOwners(t *testing.T) {
	for _, kind := range []bhruntime.ProviderKind{bhruntime.ProviderDocker, bhruntime.ProviderPodman} {
		for _, mode := range []string{"no-workload", "selected-workload", "foreign-owner", "stop-failure", "partial-stop-failure"} {
			t.Run(string(kind)+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("BASEHARBOR_TEST_RUNTIME", string(kind))
				configureAppBackupRuntimeTarget(t, filepath.Join(root, "config"), filepath.Join(root, "data"))
				target, err := effectiveTarget(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				repo := filepath.Join(root, "repository")
				if err := os.MkdirAll(repo, 0700); err != nil {
					t.Fatal(err)
				}
				t.Chdir(repo)
				m := application.New("backup-owner", "dev", false, false, false)
				if mode != "no-workload" {
					m = application.WithWorkloadComponents(m, "api", "worker")
				} else {
					m.Services.SQL = true
				}
				source := filepath.Join(repo, application.RepositoryManifestName)
				mustWriteInitializedAuthoredManifest(t, source, m)
				record := registerTestDeployment(t, target, m, repo, source)
				resolved, err := resolveApplication(t.Context(), application.DefaultStore(), nil, "backup")
				if err != nil {
					t.Fatal(err)
				}
				if resolved.Manifest.ApplicationID != m.ApplicationID || resolved.DeploymentIdentity != record.Identity || resolved.Store.Namespace != target.Name {
					t.Fatal("lost canonical repository deployment owner")
				}
				files := application.RuntimeFilesFor(resolved.Store, m)
				if err := os.MkdirAll(files.Dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(files.Env, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n  worker:\n    image: example/worker:1\n"), 0600); err != nil {
					t.Fatal(err)
				}
				r := &backupWorkloadStopRuntime{kind: kind}
				for _, service := range []string{"postgres", "broker", "api", "worker"} {
					name := files.Project + "-" + service
					if kind == bhruntime.ProviderDocker {
						name += "-1"
					}
					r.containers = append(r.containers, bhruntime.RuntimeContainer{ID: "retained-" + service, Name: name, Project: files.Project, Service: service, Running: true, State: "running"})
				}
				r.containers = append(r.containers, bhruntime.RuntimeContainer{ID: "other-target", Name: "other-target-api", Project: "bh-other-target-backup-owner-dev", Service: "api", Running: true, State: "running"})
				before := append([]bhruntime.RuntimeContainer(nil), r.containers...)
				if mode == "foreign-owner" {
					r.inspectErr = bhruntime.ErrResourceOwnership
				}
				if mode == "stop-failure" {
					r.stopErr = errors.New("native selected stop interrupted")
				}
				if mode == "partial-stop-failure" {
					r.stopErr = errors.New("partial selected stop interrupted")
					r.partialStop = true
				}
				stopped, err := stopRepositoryWorkload(t.Context(), r, resolved, files)
				if mode == "selected-workload" {
					if err != nil || !stopped || !reflect.DeepEqual(r.selected, []string{"api", "worker"}) || r.project != files.Project || r.workdir != repo || len(r.composeFiles) < 2 || r.composeFiles[0] != filepath.Join(repo, "compose.yaml") {
						t.Fatalf("wrong selected module transport: stopped=%t err=%v services=%v files=%v", stopped, err, r.selected, r.composeFiles)
					}
				} else if mode == "no-workload" {
					if err != nil || stopped || len(r.selected) != 0 || r.lists != 0 {
						t.Fatalf("absent workload touched a runtime: %v", err)
					}
				} else if mode == "partial-stop-failure" {
					if err == nil || !stopped {
						t.Fatal("partial stop lost the recovery requirement", err)
					}
				} else if err == nil || stopped {
					t.Fatal("unverified stop was accepted")
				}
				if r.legacyStops != 0 {
					t.Fatal("used project-wide stop")
				}
				for _, prior := range []bhruntime.RuntimeContainer{before[0], before[1], before[4]} {
					found := false
					for _, current := range r.containers {
						if current.ID == prior.ID {
							found = current == prior
						}
					}
					if !found {
						t.Fatalf("changed backend, broker or other Target owner %s", prior.Service)
					}
				}
				if mode == "foreign-owner" && len(r.selected) != 0 {
					t.Fatal("foreign observation triggered mutation")
				}
			})
		}
	}
}
