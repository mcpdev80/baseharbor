package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type sqlOnlyTopologyRuntime struct {
	bhruntime.RuntimeProvider
	coreCompose string
	queries     []string
}

func (r *sqlOnlyTopologyRuntime) RunningServicesProject(_ context.Context, _, compose, _ string) ([]string, error) {
	r.queries = append(r.queries, compose)
	if compose == r.coreCompose {
		return []string{"postgres-member-1", "postgres"}, nil
	}
	return nil, errors.New("empty module has no native services")
}

func TestSharedSQLOnlyTopologyObservesCoreWithoutQueryingEmptyModule(t *testing.T) {
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	root := t.TempDir()
	m := application.New("sql-only", "dev", true, false, false)
	shared := application.SharedBackendFilesAt(root, "sql-target", m.Environment)
	core := bhruntime.Files{Project: "bh-sql-target-shared", Compose: filepath.Join(root, "core-compose.yaml"), Env: filepath.Join(root, "core.env")}
	appFiles := application.RuntimeFiles{Project: "bh-sql-target-sql-only-dev", Compose: filepath.Join(root, "app-compose.yaml")}
	if err := os.MkdirAll(shared.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(map[string]any{"version": 3, "environment": "core", "core_sql": core, "applications": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{
		shared.State:     state,
		shared.Compose:   []byte("services: {}\n"),
		appFiles.Compose: []byte("services: {}\n"),
		core.Compose:     []byte("services:\n  postgres-member-1:\n    image: postgres:18\n  postgres:\n    image: haproxy:3\n"),
	} {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	resolved := resolvedApplication{Manifest: m, TargetStateRoot: root}
	resolved.Target.Name = "sql-target"
	runtime := &sqlOnlyTopologyRuntime{coreCompose: core.Compose}
	checks := collectProviderTopologyChecks(context.Background(), runtime, resolved, appFiles)
	if len(runtime.queries) != 1 || runtime.queries[0] != core.Compose {
		t.Fatalf("logical empty module queried as physical provider: %v", runtime.queries)
	}
	if len(checks) != 1 || !checks[0].OK || !strings.Contains(checks[0].Detail, "data-members=1") || !strings.Contains(checks[0].Detail, "physical-owner=core") {
		t.Fatalf("SQL-only consumer lost actual Core readiness: %#v", checks)
	}
	for _, invalid := range []struct {
		content string
		mode    os.FileMode
	}{
		{"services: null\n", 0600},
		{"services: {}\n", 0644},
		{"services:\n  valkey:\n    image: valkey/valkey:8\n", 0600},
	} {
		if err := os.WriteFile(shared.Compose, []byte(invalid.content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(shared.Compose, invalid.mode); err != nil {
			t.Fatal(err)
		}
		checks := collectProviderTopologyChecks(context.Background(), runtime, resolved, appFiles)
		failed := false
		for _, check := range checks {
			failed = failed || !check.OK
		}
		if !failed {
			t.Fatalf("invalid or nonempty unready module was ignored: %#v", checks)
		}
	}
}
