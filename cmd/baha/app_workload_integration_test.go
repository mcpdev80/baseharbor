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
	if os.Getenv("CI") == "" {
		t.Skip("real repository workload lifecycle runs in CI")
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
  postgres:
    enabled: true
  redis:
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

	pg, err := compose.ExecProjectFiles(ctx, workload.Project, root, "pg-probe", composeFiles, "sh", "-ec", `psql "$DATABASE_URL" -tAc 'SELECT 1'`)
	if err != nil {
		t.Fatalf("postgres from workload container: %v", err)
	}
	if strings.TrimSpace(pg) != "1" {
		t.Fatalf("unexpected postgres probe result %q", pg)
	}
	pong, err := compose.ExecProjectFiles(ctx, workload.Project, root, "valkey-probe", composeFiles, "sh", "-ec", `valkey-cli -u "$REDIS_URL" ping`)
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
	pg, err = compose.ExecProjectFiles(ctx, workload.Project, root, "pg-probe", composeFiles, "sh", "-ec", `psql "$DATABASE_URL" -tAc 'SELECT 1'`)
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
