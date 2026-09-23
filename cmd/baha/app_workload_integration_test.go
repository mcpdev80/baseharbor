package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_CI_RUNTIME_INTEGRATION") != "1" {
		t.Skip("real repository workload lifecycle requires BASEHARBOR_CI_RUNTIME_INTEGRATION=1")
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

	manifest := `version: 1
app:
  name: workload-ci
  environment: dev
services:
  sql:
    enabled: true
  cache:
    enabled: true
  secrets:
    enabled: false
workload:
  compose: compose.yaml
  services:
    - pg-probe
    - valkey-probe
`
	if err := os.WriteFile("baseharbor.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	composeYAML := `services:
  pg-probe:
    image: postgres:18-alpine
    command: ["sh", "-ec", "sleep infinity"]
    networks:
      - app-internal
  valkey-probe:
    image: valkey/valkey:9.1.2-alpine
    command: ["sh", "-ec", "sleep infinity"]
    networks:
      - app-internal
networks:
  app-internal:
`
	if err := os.WriteFile("compose.yaml", []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("apply repository workload: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "workload") {
		t.Fatalf("apply did not report workload readiness:\n%s", out.String())
	}

	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	m, _, err := store.Load("workload-ci")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(store, m)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, m, files)
	if err != nil || !found {
		t.Fatalf("materialize workload for verification: found=%v err=%v", found, err)
	}
	composeFiles := []string{workload.Compose, workload.Override}

	pg, err := compose.ExecProjectFiles(ctx, workload.Project, root, "pg-probe", composeFiles, "sh", "-ec", `psql ${DATABASE_URL} -tAc 'SELECT 1'`)
	if err != nil {
		t.Fatalf("postgres from workload container: %v", err)
	}
	if strings.TrimSpace(pg) != "1" {
		t.Fatalf("unexpected postgres probe result %q", pg)
	}
	pong, err := compose.ExecProjectFiles(ctx, workload.Project, root, "valkey-probe", composeFiles, "sh", "-ec", `valkey-cli -u ${REDIS_URL} ping`)
	if err != nil {
		t.Fatalf("valkey from workload container: %v", err)
	}
	if strings.TrimSpace(pong) != "PONG" {
		t.Fatalf("unexpected valkey probe result %q", pong)
	}

	for _, command := range [][]string{{"app", "status"}, {"app", "doctor"}} {
		out.Reset()
		if err := runWithIO(ctx, command, &out, &out); err != nil {
			t.Fatalf("%v failed: %v\n%s", command, err, out.String())
		}
		if !strings.Contains(out.String(), "workload") {
			t.Fatalf("%v did not report workload state:\n%s", command, out.String())
		}
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "down"}, &out, &out); err != nil {
		t.Fatalf("down workload application: %v\n%s", err, out.String())
	}
	running, err := compose.RunningServicesProjectFiles(ctx, workload.Project, root, composeFiles...)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 0 {
		t.Fatalf("workload services remain after down: %v", running)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "up"}, &out, &out); err != nil {
		t.Fatalf("up workload application: %v\n%s", err, out.String())
	}
	pg, err = compose.ExecProjectFiles(ctx, workload.Project, root, "pg-probe", composeFiles, "sh", "-ec", `psql ${DATABASE_URL} -tAc 'SELECT 1'`)
	if err != nil || strings.TrimSpace(pg) != "1" {
		t.Fatalf("postgres probe after up: result=%q err=%v", pg, err)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy workload application: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); err != nil {
		t.Fatalf("repository manifest not preserved: %v", err)
	}
}

func TestRepositoryComposeWorkloadOnlyLifecycleInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_CI_RUNTIME_INTEGRATION") != "1" {
		t.Skip("real repository workload-only lifecycle requires BASEHARBOR_CI_RUNTIME_INTEGRATION=1")
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

	manifest := `version: 1
app:
  name: workload-only-ci
  environment: dev
services:
  sql:
    enabled: false
  cache:
    enabled: false
  secrets:
    enabled: false
workload:
  compose: compose.yaml
  services:
    - app
`
	if err := os.WriteFile("baseharbor.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	composeYAML := `services:
  app:
    image: alpine:3.22
    command: ["sh", "-ec", "sleep infinity"]
    environment:
      APP_OWNED_VALUE: preserved
    networks:
      - app-internal
networks:
  app-internal:
`
	if err := os.WriteFile("compose.yaml", []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("apply workload-only application: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "workload") {
		t.Fatalf("apply did not report workload readiness:\n%s", out.String())
	}

	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	m, _, err := store.Load("workload-only-ci")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(store, m)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, m, files)
	if err != nil || !found {
		t.Fatalf("materialize workload-only verification: found=%v err=%v", found, err)
	}
	override, err := os.ReadFile(workload.Override)
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []string{"baseharbor-backend", "DATABASE_URL", "REDIS_URL", "VALKEY_URL"} {
		if strings.Contains(string(override), unexpected) {
			t.Fatalf("workload-only override invented %q:\n%s", unexpected, string(override))
		}
	}

	composeFiles := []string{workload.Compose, workload.Override}
	probe, err := compose.ExecProjectFiles(ctx, workload.Project, root, "app", composeFiles, "sh", "-ec", `test "$APP_OWNED_VALUE" = preserved; test -z "$DATABASE_URL"; test -z "$REDIS_URL"; echo ok`)
	if err != nil || strings.TrimSpace(probe) != "ok" {
		t.Fatalf("workload-only application contract probe: result=%q err=%v", probe, err)
	}

	for _, command := range [][]string{{"app", "status"}, {"app", "doctor"}} {
		out.Reset()
		if err := runWithIO(ctx, command, &out, &out); err != nil {
			t.Fatalf("%v failed for workload-only app: %v\n%s", command, err, out.String())
		}
		if !strings.Contains(out.String(), "workload") {
			t.Fatalf("%v did not report workload-only state:\n%s", command, out.String())
		}
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "down"}, &out, &out); err != nil {
		t.Fatalf("down workload-only application: %v\n%s", err, out.String())
	}
	running, err := compose.RunningServicesProjectFiles(ctx, workload.Project, root, composeFiles...)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 0 {
		t.Fatalf("workload-only services remain after down: %v", running)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "up"}, &out, &out); err != nil {
		t.Fatalf("up workload-only application: %v\n%s", err, out.String())
	}
	probe, err = compose.ExecProjectFiles(ctx, workload.Project, root, "app", composeFiles, "sh", "-ec", `test "$APP_OWNED_VALUE" = preserved; echo ok`)
	if err != nil || strings.TrimSpace(probe) != "ok" {
		t.Fatalf("workload-only probe after up: result=%q err=%v", probe, err)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy workload-only application: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); err != nil {
		t.Fatalf("repository manifest not preserved: %v", err)
	}
}

func TestRepositoryBuildWorkloadRebuildsSourceChangesInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_CI_RUNTIME_INTEGRATION") != "1" {
		t.Skip("real repository build convergence requires BASEHARBOR_CI_RUNTIME_INTEGRATION=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect runtime: %v", err)
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
  name: build-convergence-ci
  environment: dev
services:
  sql:
    enabled: false
  cache:
    enabled: false
  secrets:
    enabled: false
workload:
  compose: compose.yaml
  services:
    - app
`
	composeYAML := `services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
    command: ["sh", "-ec", "sleep infinity"]
`
	dockerfile := "FROM alpine:3.22\nCOPY message.txt /message.txt\n"
	if err := os.WriteFile("baseharbor.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("compose.yaml", []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".dockerignore", []byte("ignored.txt\n.baseharbor/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("message.txt", []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("ignored.txt", []byte("ignored-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("initial build apply: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "rebuilt app") {
		t.Fatalf("initial build did not report build realization:\n%s", out.String())
	}

	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	m, _, err := store.Load("build-convergence-ci")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(store, m)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, m, files)
	if err != nil || !found {
		t.Fatalf("materialize build workload: found=%v err=%v", found, err)
	}
	composeFiles := []string{workload.Compose, workload.Override}
	assertMessage := func(want string) {
		t.Helper()
		got, err := compose.ExecProjectFiles(ctx, workload.Project, root, "app", composeFiles, "cat", "/message.txt")
		if err != nil {
			t.Fatalf("read realized message: %v", err)
		}
		if strings.TrimSpace(got) != want {
			t.Fatalf("realized message = %q, want %q", strings.TrimSpace(got), want)
		}
	}
	assertMessage("one")
	firstIdentity, err := compose.ProjectServiceImageIdentity(ctx, workload.Project, "app")
	if err != nil {
		t.Fatalf("inspect first workload image: %v", err)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "up"}, &out, &out); err != nil {
		t.Fatalf("unchanged app up: %v\n%s", err, out.String())
	}
	unchangedIdentity, err := compose.ProjectServiceImageIdentity(ctx, workload.Project, "app")
	if err != nil {
		t.Fatalf("inspect unchanged workload image: %v", err)
	}
	if unchangedIdentity.ImageID != firstIdentity.ImageID {
		t.Fatalf("unchanged workload image changed: %s -> %s", firstIdentity.ImageID, unchangedIdentity.ImageID)
	}

	if err := os.WriteFile("ignored.txt", []byte("ignored-two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "up"}, &out, &out); err != nil {
		t.Fatalf("ignored-input app up: %v\n%s", err, out.String())
	}
	ignoredIdentity, err := compose.ProjectServiceImageIdentity(ctx, workload.Project, "app")
	if err != nil {
		t.Fatalf("inspect ignored-input workload image: %v", err)
	}
	if ignoredIdentity.ImageID != firstIdentity.ImageID {
		t.Fatalf("ignored input changed workload image: %s -> %s", firstIdentity.ImageID, ignoredIdentity.ImageID)
	}

	if err := os.WriteFile("message.txt", []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "up"}, &out, &out); err != nil {
		t.Fatalf("changed-source app up: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "rebuilt app") {
		t.Fatalf("source change did not report selective rebuild:\n%s", out.String())
	}
	assertMessage("two")
	changedIdentity, err := compose.ProjectServiceImageIdentity(ctx, workload.Project, "app")
	if err != nil {
		t.Fatalf("inspect changed workload image: %v", err)
	}
	if changedIdentity.ImageID == firstIdentity.ImageID {
		t.Fatalf("source change retained stale workload image %s", changedIdentity.ImageID)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy build workload: %v\n%s", err, out.String())
	}
}
