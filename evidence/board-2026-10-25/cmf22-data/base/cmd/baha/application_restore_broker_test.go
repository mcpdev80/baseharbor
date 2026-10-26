package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

type restoreBrokerStopFixture struct {
	bhruntime.RuntimeProvider
	resources           []bhruntime.ProjectResource
	containers          []bhruntime.RuntimeContainer
	selected            []string
	inspectErr, stopErr error
	legacyStops         int
	destroyErr          error
}

func (f *restoreBrokerStopFixture) StopProject(context.Context, string, string, string) error {
	f.legacyStops++
	return errors.New("retired broker unit is not loaded")
}
func (f *restoreBrokerStopFixture) DestroyProject(context.Context, string, string, string) error {
	return f.destroyErr
}

func TestRestoreResetUsesObservedBrokerAbsenceBeforeDestructivePhase(t *testing.T) {
	m := application.New("restore-reset-broker", "dev", false, false, true)
	store := application.Store{Root: t.TempDir(), Namespace: "isolated"}
	files := application.RuntimeFilesFor(store, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{files.Compose: "services: {}\n", files.Env: "FIXTURE=retained\n", filepath.Join(files.Dir, "broker.compose.yaml"): "services: {}\n", filepath.Join(files.Dir, "broker-image"): "isolated-image\n"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	id, err := deployment.NewDeploymentIdentity("isolated", m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Manifest: m, Store: store, DeploymentIdentity: id}
	ctx := context.WithValue(t.Context(), verifiedRestoreScopeKey{}, verifiedRestoreScope{Identity: id, Intent: m.YAML(), PlanID: "current-fixture-owner-plan"})
	marker := errors.New("reached destructive reset only after broker absence verification")
	native := &restoreBrokerStopFixture{destroyErr: marker}
	err = resetRestoreTarget(ctx, native, bhruntime.Files{}, resolved)
	if !errors.Is(err, marker) || native.legacyStops != 0 || len(native.selected) != 0 {
		t.Fatal("actual restore reset did not use verified retired-broker absence", err)
	}
}

func (f *restoreBrokerStopFixture) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }
func (f *restoreBrokerStopFixture) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return f.resources, f.inspectErr
}
func (f *restoreBrokerStopFixture) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return f.containers, nil
}
func (f *restoreBrokerStopFixture) StopProjectFilesSelected(_ context.Context, _ string, _ string, _ map[string]string, selected []string, _ ...string) error {
	f.selected = append(f.selected, selected...)
	if f.stopErr != nil {
		return f.stopErr
	}
	for i := range f.containers {
		if f.containers[i].Service == runtimebroker.ServiceName {
			f.containers[i].Running = false
			f.containers[i].State = "exited"
		}
	}
	return nil
}

func TestRestoreBrokerStopVerifiesRetiredAbsenceAndPreservesSiblings(t *testing.T) {
	for _, mode := range []string{"retired", "running", "foreign", "replica", "stop-failure", "public-owner"} {
		t.Run(mode, func(t *testing.T) {
			m := application.New("restore-broker", "dev", false, false, true)
			files := application.RuntimeFilesFor(application.Store{Root: t.TempDir(), Namespace: "isolated"}, m)
			if err := os.MkdirAll(files.Dir, 0700); err != nil {
				t.Fatal(err)
			}
			for path, data := range map[string]string{files.Env: "BROKER_FIXTURE=retained\n", filepath.Join(files.Dir, "broker.compose.yaml"): "services: {}\n", filepath.Join(files.Dir, "broker-image"): "isolated-image\n"} {
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			project := runtimebroker.ProjectNameForRuntime(m, files)
			sibling := bhruntime.RuntimeContainer{ID: "sibling", Name: project + "-api-1", Project: project, Service: "api", Running: true, State: "running"}
			f := &restoreBrokerStopFixture{containers: []bhruntime.RuntimeContainer{sibling}}
			if mode != "retired" && mode != "public-owner" {
				f.resources = []bhruntime.ProjectResource{{Kind: "container", Name: project + "-broker-1"}}
				f.containers = append(f.containers, bhruntime.RuntimeContainer{ID: "broker", Name: project + "-broker-1", Project: project, Service: "broker", Running: true, State: "running"})
			}
			switch mode {
			case "foreign":
				f.inspectErr = bhruntime.ErrResourceOwnership
			case "replica":
				f.containers[1].Name = project + "-broker-2"
			case "stop-failure":
				f.stopErr = errors.New("native stop failed")
			case "public-owner":
				if err := os.Chmod(filepath.Join(files.Dir, "broker.compose.yaml"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			err := stopRestoreRuntimeBroker(t.Context(), f, m, files)
			if mode == "retired" || mode == "running" {
				if err != nil {
					t.Fatal(err)
				}
				if mode == "retired" && len(f.selected) != 0 {
					t.Fatal("absent broker triggered native stop")
				}
				if mode == "running" && !reflect.DeepEqual(f.selected, []string{"broker"}) {
					t.Fatal("selected another service")
				}
			} else if err == nil {
				t.Fatal("unverified broker reset was accepted")
			}
			if f.containers[0] != sibling {
				t.Fatal("restore broker stop changed sibling")
			}
			if mode != "running" && mode != "stop-failure" && len(f.selected) != 0 {
				t.Fatal("unsafe broker observation mutated runtime")
			}
		})
	}
}
