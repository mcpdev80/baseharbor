package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestDirectedCrossApplicationConnectivityInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_CONNECTIVITY_ACCEPTANCE") != "true" {
		t.Skip("real directed connectivity lifecycle requires BASEHARBOR_CONNECTIVITY_ACCEPTANCE=true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	targetStore := application.Store{Root: filepath.Join(t.TempDir(), "apps")}
	target := application.New("connect-target-ci", "dev", true, false, false)
	targetFiles, err := application.EnsureRuntime(targetStore, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := compose.UpProject(ctx, application.RuntimeProjectName(target), targetFiles.Compose, targetFiles.Env); err != nil {
		t.Fatalf("start target PostgreSQL: %v", err)
	}
	defer func() {
		_ = compose.DestroyProject(context.Background(), application.RuntimeProjectName(target), targetFiles.Compose, targetFiles.Env)
	}()

	sourceRoot := t.TempDir()
	source := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "connect-source-ci",
		Environment: "dev",
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"api"},
		},
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "compose.yaml"), []byte(`services:
  api:
    image: postgres:18-alpine
    environment:
      POSTGRES_DB: source
      POSTGRES_USER: source
      POSTGRES_HOST_AUTH_METHOD: trust
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceStore := application.Store{Root: filepath.Join(sourceRoot, ".baseharbor", "apps")}
	sourceFiles, err := application.EnsureRuntime(sourceStore, source)
	if err != nil {
		t.Fatal(err)
	}
	sourceWorkload, found, err := application.MaterializeWorkload(sourceRoot, source, sourceFiles)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("source workload not materialized")
	}
	sourceComposeFiles := []string{sourceWorkload.Compose, sourceWorkload.Override}
	if err := compose.ConfigProjectFilesEnv(ctx, sourceWorkload.Project, sourceWorkload.RepositoryRoot, nil, sourceComposeFiles...); err != nil {
		t.Fatal(err)
	}
	if err := compose.UpProjectFilesSelected(ctx, sourceWorkload.Project, sourceWorkload.RepositoryRoot, nil, sourceWorkload.Services, sourceComposeFiles...); err != nil {
		t.Fatalf("start source workload: %v", err)
	}
	defer func() {
		_ = compose.DownProjectFilesEnv(context.Background(), sourceWorkload.Project, sourceWorkload.RepositoryRoot, nil, sourceComposeFiles...)
	}()

	waitForSourceDatabase(t, ctx, compose, sourceWorkload, sourceComposeFiles)
	waitForConnectivityTarget(t, ctx, compose, target, targetFiles)

	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"connect", source.Name + "/api", target.Name + "/sql"}, &out, &out); err != nil {
		t.Fatalf("connect failed: %v\n%s", err, out.String())
	}
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("connectivity rules=%#v", rules)
	}
	rule := rules[0]
	defer func() {
		containers, _ := compose.ListComposeContainers(context.Background())
		_ = suspendConnectivityRule(context.Background(), compose, rule, containers)
		_ = application.RemoveConnectivityRule(rule)
		_ = connectivityrelay.RemoveFiles(application.ConnectivityRuleID(rule))
	}()

	sourceContainers, err := compose.ListComposeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sourceNames := containersForResolvedEndpoint(rule.Source, sourceContainers)
	targetNames := containersForResolvedEndpoint(rule.Target, sourceContainers)
	if len(sourceNames) != 1 || len(targetNames) != 1 {
		t.Fatalf("unexpected resolved containers source=%#v target=%#v", sourceNames, targetNames)
	}

	link := application.ConnectivityNetworkName(rule)
	sourceNetworks, err := compose.ContainerNetworks(ctx, sourceNames[0])
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(sourceNetworks, link) {
		t.Fatalf("source is not attached to connectivity link %q: %#v", link, sourceNetworks)
	}
	targetNetworks, err := compose.ContainerNetworks(ctx, targetNames[0])
	if err != nil {
		t.Fatal(err)
	}
	if containsString(targetNetworks, link) {
		t.Fatalf("target unexpectedly joined source connectivity link %q: %#v", link, targetNetworks)
	}

	relayFiles, err := connectivityrelay.ExistingFiles(application.ConnectivityRuleID(rule))
	if err != nil {
		t.Fatal(err)
	}
	relayRunning, err := compose.RunningServicesProject(ctx, relayFiles.Project, relayFiles.Compose, relayFiles.Env)
	if err != nil {
		t.Fatal(err)
	}
	if len(relayRunning) != 1 || relayRunning[0] != "relay" {
		t.Fatalf("relay is not running: %#v", relayRunning)
	}

	probe := application.ConnectivityTargetAlias(rule)
	waitForSourceProbe(t, ctx, compose, sourceWorkload, sourceComposeFiles, probe, rule.Target.Port)
	assertNoReverseConnectivity(t, ctx, compose, target, targetFiles)

	if err := suspendConnectivityForManifest(ctx, compose, target); err != nil {
		t.Fatalf("suspend target connectivity: %v", err)
	}
	rules, err = application.LoadConnectivityRules()
	if err != nil || len(rules) != 1 {
		t.Fatalf("connectivity policy was not preserved across suspend: rules=%#v err=%v", rules, err)
	}
	if _, err := connectivityrelay.ExistingFiles(application.ConnectivityRuleID(rule)); err != nil {
		t.Fatalf("relay definition should remain available for reconciliation: %v", err)
	}

	if err := reconcileConnectivityForManifest(ctx, io.Discard, compose, target); err != nil {
		t.Fatalf("reconcile target connectivity: %v", err)
	}
	waitForSourceProbe(t, ctx, compose, sourceWorkload, sourceComposeFiles, probe, rule.Target.Port)

	out.Reset()
	if err := runWithIO(ctx, []string{"disconnect", source.Name + "/api", target.Name + "/sql"}, &out, &out); err != nil {
		t.Fatalf("disconnect failed: %v\n%s", err, out.String())
	}
	rules, err = application.LoadConnectivityRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("connectivity rules remain after disconnect: %#v", rules)
	}
	if _, err := connectivityrelay.ExistingFiles(application.ConnectivityRuleID(rule)); !os.IsNotExist(err) {
		t.Fatalf("relay state remains after disconnect: %v", err)
	}
}

func waitForSourceDatabase(t *testing.T, ctx context.Context, compose bhruntime.Compose, workload application.WorkloadFiles, composeFiles []string) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var last error
	for deadline.Err() == nil {
		_, last = compose.ExecProjectFiles(deadline, workload.Project, workload.RepositoryRoot, "api", composeFiles, "pg_isready", "-h", "127.0.0.1", "-p", "5432")
		if last == nil {
			return
		}
		select {
		case <-deadline.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatalf("source PostgreSQL did not become ready: %v", last)
}

func assertNoReverseConnectivity(t *testing.T, ctx context.Context, compose bhruntime.Compose, target application.Manifest, files application.RuntimeFiles) {
	t.Helper()
	_, err := compose.ExecProject(ctx, application.RuntimeProjectName(target), files.Compose, files.Env, "postgres", "pg_isready", "-h", "api", "-p", "5432", "-t", "2")
	if err == nil {
		t.Fatal("reverse connectivity unexpectedly succeeded: target resolved/reached source api:5432")
	}
}

func waitForConnectivityTarget(t *testing.T, ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var last error
	for deadline.Err() == nil {
		last = application.VerifyPostgresRuntime(deadline, compose, m, files)
		if last == nil {
			return
		}
		select {
		case <-deadline.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatalf("target PostgreSQL did not become ready: %v", last)
}

func waitForSourceProbe(t *testing.T, ctx context.Context, compose bhruntime.Compose, workload application.WorkloadFiles, composeFiles []string, host string, port int) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var last error
	for deadline.Err() == nil {
		_, last = compose.ExecProjectFiles(deadline, workload.Project, workload.RepositoryRoot, "api", composeFiles, "pg_isready", "-h", host, "-p", strconv.Itoa(port))
		if last == nil {
			return
		}
		select {
		case <-deadline.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatalf("source could not reach directed target %s:%d: %v", host, port, last)
}

