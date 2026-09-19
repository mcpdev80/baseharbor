package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestManagedHTTPExposureLifecycleInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("real managed exposure lifecycle runs in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	manifest := `version: 1
app:
  name: exposure-ci
  environment: dev
services:
  postgres:
    enabled: false
  redis:
    enabled: false
  secrets:
    enabled: false
workload:
  compose: compose.yaml
  services:
    - web
exposure:
  http:
    - name: public
      service: web
      port: 8080
      protocol: http
      visibility: internal
`
	if err := os.WriteFile("baseharbor.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	composeYAML := `services:
  web:
    image: alpine:3.22
    command:
      - sh
      - -ec
      - |
        mkdir -p /www
        printf 'baseharbor-exposure-ok\n' >/www/index.html
        exec busybox httpd -f -p 8080 -h /www
    networks:
      - app-internal
networks:
  app-internal:
`
	if err := os.WriteFile("compose.yaml", []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	state := repositoryInitState{
		Hostname:        "localhost",
		TLSMode:         "local",
		RuntimeProvider: bhruntime.ProviderCompose,
	}
	if err := writeRepositoryInitState(root, state); err != nil {
		t.Fatalf("write repository init state: %v", err)
	}

	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("apply managed exposure: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[OK] managed-exposure") {
		t.Fatalf("apply did not report managed exposure:\n%s", out.String())
	}

	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	m, _, err := store.Load("exposure-ci")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(store, m)
	if err != nil {
		t.Fatal(err)
	}
	firstState, providerFiles, err := exposure.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstState.Routes) != 1 {
		t.Fatalf("unexpected managed routes: %#v", firstState.Routes)
	}
	if firstState.Routes[0].Visibility != "internal" {
		t.Fatalf("unexpected route visibility %#v", firstState.Routes[0])
	}
	exposureNetwork := bhruntime.ProjectResource{Kind: "network", Name: application.ApplicationExposureNetworkName(m)}
	exists, err := compose.InspectProjectResource(ctx, application.WorkloadProjectName(m), exposureNetwork)
	if err != nil {
		t.Fatalf("inspect managed exposure network: %v", err)
	}
	if !exists {
		t.Fatalf("managed exposure network %s was not created", exposureNetwork.Name)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "status"}, &out, &out); err != nil {
		t.Fatalf("status managed exposure: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "managed-exposure/public") || !strings.Contains(out.String(), "HTTP 200") {
		t.Fatalf("status missing end-to-end exposure readiness:\n%s", out.String())
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "doctor"}, &out, &out); err != nil {
		t.Fatalf("doctor managed exposure: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[OK] managed HTTP exposure") {
		t.Fatalf("doctor missing managed exposure check:\n%s", out.String())
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("repeat apply managed exposure: %v\n%s", err, out.String())
	}
	secondState, _, err := exposure.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstState, secondState) {
		t.Fatalf("repeated convergence changed provider state:\nfirst=%#v\nsecond=%#v", firstState, secondState)
	}
	running, err := compose.RunningServicesProject(ctx, secondState.Project, providerFiles.Compose, providerFiles.Env)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 1 || running[0] != "route-public" {
		t.Fatalf("unexpected managed exposure services after repeat apply: %#v", running)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "down"}, &out, &out); err != nil {
		t.Fatalf("down managed exposure application: %v\n%s", err, out.String())
	}
	running, err = compose.RunningServicesProject(ctx, secondState.Project, providerFiles.Compose, providerFiles.Env)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 0 {
		t.Fatalf("managed exposure remains running after down: %#v", running)
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "status"}, &out, &out); err != nil {
		t.Fatalf("stopped application status: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "State: STOPPED") {
		t.Fatalf("stopped managed exposure app did not report STOPPED:\n%s", out.String())
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "up"}, &out, &out); err != nil {
		t.Fatalf("up managed exposure application: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[OK] managed-exposure") {
		t.Fatalf("up did not verify managed exposure:\n%s", out.String())
	}
	thirdState, _, err := exposure.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(secondState, thirdState) {
		t.Fatalf("down/up changed provider state:\nbefore=%#v\nafter=%#v", secondState, thirdState)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy managed exposure application: %v\n%s", err, out.String())
	}
	if _, err := os.Stat("baseharbor.yaml"); err != nil {
		t.Fatalf("repository manifest not preserved: %v", err)
	}
	if _, err := os.Stat(providerFiles.Dir); !os.IsNotExist(err) {
		t.Fatalf("managed exposure provider state remains after destroy: %v", err)
	}
	exists, err = compose.InspectProjectResource(ctx, application.WorkloadProjectName(m), exposureNetwork)
	if err != nil {
		t.Fatalf("inspect exposure network after destroy: %v", err)
	}
	if exists {
		t.Fatalf("managed exposure network %s remains after destroy", exposureNetwork.Name)
	}
}

func TestManagedHTTPExposureFailedVerificationCleansProviderResourcesInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("real managed exposure failure cleanup runs in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile("baseharbor.yaml", []byte(`version: 1
app:
  name: exposure-fail-ci
  environment: dev
services:
  postgres:
    enabled: false
  redis:
    enabled: false
  secrets:
    enabled: false
workload:
  compose: compose.yaml
  services:
    - web
exposure:
  http:
    - name: broken
      service: web
      port: 9099
      protocol: http
      visibility: internal
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("compose.yaml", []byte(`services:
  web:
    image: alpine:3.22
    command: ["sh", "-ec", "exec busybox httpd -f -p 8080"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeRepositoryInitState(root, repositoryInitState{
		Hostname: "localhost", TLSMode: "local", RuntimeProvider: bhruntime.ProviderCompose,
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err = runWithIO(ctx, []string{"app", "apply"}, &out, &out)
	if err == nil {
		t.Fatalf("broken managed exposure unexpectedly applied:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "managed HTTP exposure") {
		t.Fatalf("unexpected apply failure: %v\n%s", err, out.String())
	}

	m := application.Manifest{
		Version: application.CurrentVersion, Name: "exposure-fail-ci", Environment: "dev",
		Workload:  application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures: []application.HTTPExposureRequirement{{Name: "broken", Service: "web", Port: 9099, Protocol: "http", Visibility: "internal"}},
	}
	project := exposure.ProjectName(m)
	providerRunning, runErr := compose.RunningServicesProject(ctx, project,
		filepath.Join(root, ".baseharbor", "apps", m.Name, "runtime", "providers", "caddy", "compose.yaml"),
		filepath.Join(root, ".baseharbor", "apps", m.Name, "runtime", "providers", "caddy", "provider.env"))
	if runErr == nil && len(providerRunning) != 0 {
		t.Fatalf("failed exposure left provider services running: %#v", providerRunning)
	}
	network := bhruntime.ProjectResource{Kind: "network", Name: application.ApplicationExposureNetworkName(m)}
	exists, inspectErr := compose.InspectProjectResource(ctx, application.WorkloadProjectName(m), network)
	if inspectErr != nil {
		t.Fatalf("inspect workload-owned exposure network after provider failure: %v", inspectErr)
	}
	if !exists {
		t.Fatalf("workload-owned exposure network %s unexpectedly disappeared while workload remains materialized", network.Name)
	}
	providerNetwork := bhruntime.ProjectResource{Kind: "network", Name: network.Name}
	if _, providerInspectErr := compose.InspectProjectResource(ctx, project, providerNetwork); providerInspectErr == nil {
		t.Fatal("Caddy provider unexpectedly owns the workload exposure network")
	}

	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	if loaded, _, loadErr := store.Load(m.Name); loadErr == nil {
		if files, filesErr := application.ExistingRuntimeFiles(store, loaded); filesErr == nil {
			_, _ = stopRepositoryWorkload(ctx, compose, resolvedApplication{
				Manifest: loaded, ManifestPath: filepath.Join(root, "baseharbor.yaml"), Store: store, FromRepository: true,
			}, files)
		}
	}
}
