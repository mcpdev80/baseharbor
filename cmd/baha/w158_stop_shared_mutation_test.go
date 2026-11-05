package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// runningWorkloadRuntime is a fake RuntimeProvider that reports a single owned
// workload container as running, so the shared stopRepositoryWorkload can be
// driven to completion past the ownership gate and its stop result measured.
type runningWorkloadRuntime struct {
	bhruntime.RuntimeProvider
	kind       bhruntime.ProviderKind
	containers []bhruntime.RuntimeContainer
	stopped    []string
}

func (r *runningWorkloadRuntime) Kind() bhruntime.ProviderKind { return r.kind }
func (r *runningWorkloadRuntime) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return `{"services":{"api":{}}}`, nil
}
func (r *runningWorkloadRuntime) InspectProjectResources(_ context.Context, project string, expected []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
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
func (r *runningWorkloadRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return append([]bhruntime.RuntimeContainer(nil), r.containers...), nil
}
func (r *runningWorkloadRuntime) StopProjectFilesSelected(_ context.Context, project, workdir string, _ map[string]string, services []string, composeFiles ...string) error {
	r.stopped = append([]string(nil), services...)
	for i := range r.containers {
		for _, s := range services {
			if r.containers[i].Project == project && r.containers[i].Service == s {
				r.containers[i].Running, r.containers[i].State = false, "exited"
			}
		}
	}
	return nil
}

// resolveRunningWorkloadFixture builds a resolved repository application whose
// runtime directory is present and (optionally) carries an owned runtime.env,
// returning the RuntimeFiles so the caller can toggle runtime.env's presence.
func resolveRunningWorkloadFixture(t *testing.T, withRuntimeEnv bool) (resolvedApplication, application.RuntimeFiles) {
	t.Helper()
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithHTTPExposure(m, "demo-app", "demo-app", 8080, "https")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for apply: %v", err)
	}

	manifestBytes, _ := json.Marshal(resolved.Manifest)
	pending := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: resolved.repositoryRoot(), Manifest: resolved.ManifestPath, Digest: "mut-revision"},
		Applied: deployment.AppliedDeployment{
			Intent:          json.RawMessage(manifestBytes),
			RuntimeProvider: resolved.Target.RuntimeProvider,
			LastAppliedRef:  "mut-revision",
			GeneratedState:  map[string]string{"deployment": resolved.DeploymentStateRoot, "state": resolved.DeploymentStateRoot + "/state"},
		},
		Observed: deployment.ObservedDeployment{State: "applying", Ready: false},
	}
	if err := deployment.SaveDeploymentRecord(pending); err != nil {
		t.Fatal(err)
	}

	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if withRuntimeEnv {
		if err := os.WriteFile(files.Env, []byte("KEY=value\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, statErr := os.Stat(files.Env); !os.IsNotExist(statErr) {
			t.Fatalf("precondition: runtime.env must be absent")
		}
	}
	return resolved, files
}

// TestStopRepositoryWorkloadRunningEnvMissing measures the shared stop contract
// for the review's degenerate mutation: a workload whose runtime directory is
// present but whose runtime.env is absent while an owned container is actually
// running. Under normal develop states a missing runtime.env means the
// workload was never materialized; this fixture forces the foreign
// "running but env-absent" state and measures the return. Per review-1382b the
// SHARED path must NOT no-op here: it fails closed with an inventory error so
// backup/TLS/restore/ordinary-stop never infer "workload stopped" from a
// missing owner file while a workload may be running.
func TestStopRepositoryWorkloadRunningEnvMissing(t *testing.T) {
	resolved, files := resolveRunningWorkloadFixture(t, false)
	r := &runningWorkloadRuntime{kind: bhruntime.ProviderDocker}
	r.containers = append(r.containers, bhruntime.RuntimeContainer{
		ID: files.Project + "-demo-app-1", Name: files.Project + "-demo-app-1",
		Project: files.Project, Service: "demo-app", Running: true, State: "running",
	})
	stopped, err := stopRepositoryWorkload(context.Background(), r, resolved, files)
	t.Logf("shared stopRepositoryWorkload(running workload, runtime.env ABSENT) = stopped=%v err=%v stoppedServices=%v", stopped, err, r.stopped)
	if err == nil {
		t.Fatalf("shared stop MUST fail closed on degenerate state (missing runtime.env while running); got no error")
	}
	if stopped {
		t.Fatalf("shared stop must not report stopped on the degenerate state, got stopped=true")
	}
	if len(r.stopped) != 0 {
		t.Fatalf("shared stop must NOT mutate a running workload on the degenerate state (stopped %v)", r.stopped)
	}
	t.Logf("shared stop failed closed: stopped=%v err=%q; backup/TLS/restore cannot no-op a running workload", stopped, err.Error())
}

// TestStopRepositoryWorkloadRunningEnvPresent is the healthy contrast: an owned
// runtime.env present while the container runs => the shared stop actually
// stops it (stopped=true) and surfaces the owned materialization.
func TestStopRepositoryWorkloadRunningEnvPresent(t *testing.T) {
	resolved, files := resolveRunningWorkloadFixture(t, true)
	r := &runningWorkloadRuntime{kind: bhruntime.ProviderDocker}
	r.containers = append(r.containers, bhruntime.RuntimeContainer{
		ID: files.Project + "-demo-app-1", Name: files.Project + "-demo-app-1",
		Project: files.Project, Service: "demo-app", Running: true, State: "running",
	})
	stopped, err := stopRepositoryWorkload(context.Background(), r, resolved, files)
	if err != nil {
		t.Fatalf("healthy running workload must not error: %v", err)
	}
	if !stopped {
		t.Fatalf("healthy running workload must be stopped (stopped=true), got stopped=%v", stopped)
	}
	if len(r.stopped) == 0 {
		t.Fatalf("healthy running workload was not stopped")
	}
	t.Logf("shared stop returned stopped=true, stoppedServices=%v (healthy owned workload cell)", r.stopped)
}

// TestDestroyRepositoryWorkloadStopRetiresDegenerateState is the third recipe
// cell: the destroy/retirement path MUST still retire the stuck-apply state
// (runtime directory present, runtime.env absent) with rc=0. This is the
// destroy-only behavior moved into destroyRepositoryWorkloadStop; the shared
// path fails closed instead.
func TestDestroyRepositoryWorkloadStopRetiresDegenerateState(t *testing.T) {
	resolved, files := resolveRunningWorkloadFixture(t, false)
	r := &runningWorkloadRuntime{kind: bhruntime.ProviderDocker}
	r.containers = append(r.containers, bhruntime.RuntimeContainer{
		ID: files.Project + "-demo-app-1", Name: files.Project + "-demo-app-1",
		Project: files.Project, Service: "demo-app", Running: true, State: "running",
	})
	stopped, err := destroyRepositoryWorkloadStop(context.Background(), r, resolved, files)
	t.Logf("destroy stopRepositoryWorkload(running workload, runtime.env ABSENT) = stopped=%v err=%v", stopped, err)
	if err != nil {
		t.Fatalf("destroy/retirement must retire the stuck-apply state with rc=0, got error: %v", err)
	}
	if stopped {
		t.Fatalf("destroy retire of stuck-apply must not claim it stopped a workload, got stopped=true")
	}
	t.Log("destroy path retires stuck-apply (dir present, env absent) with rc=0; shared path fails closed")
}

var _ = errors.New
