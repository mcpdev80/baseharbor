package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestWorkloadPublishedPortVariables(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	content := `services:
  edge:
    ports:
      - "${HTTP_PORT:-80}:80"
      - "${HTTPS_PORT:-443}:443"
  api:
    ports:
      - "127.0.0.1:${API_PORT:-8000}:8000"
`
	if err := os.WriteFile(composePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := workloadPublishedPortVariables(application.WorkloadFiles{Compose: composePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 published port variables, got %#v", got)
	}
	want := map[string]int{"HTTP_PORT": 80, "HTTPS_PORT": 443, "API_PORT": 8000}
	for _, item := range got {
		if want[item.Name] != item.DefaultPort {
			t.Fatalf("unexpected port variable %#v", item)
		}
	}
}

func TestConflictingWorkloadHostPort(t *testing.T) {
	cases := map[string]int{
		"Bind for 127.0.0.1:80 failed: port is already allocated":          80,
		"failed to bind host port for 0.0.0.0:443: address already in use": 443,
		"host port 8080 is busy":                                           8080,
	}
	for message, want := range cases {
		if got := conflictingWorkloadHostPort(errors.New(message)); got != want {
			t.Fatalf("%q: expected %d, got %d", message, want, got)
		}
	}
}

func TestPersistedWorkloadPortOverrides(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	environment := map[string]string{}
	if err := persistWorkloadPortOverride(files, environment, "HTTP_PORT", 8080); err != nil {
		t.Fatal(err)
	}
	if environment["HTTP_PORT"] != "8080" {
		t.Fatalf("expected live environment override, got %#v", environment)
	}

	reloaded := map[string]string{}
	if err := mergePersistedWorkloadPortOverrides(reloaded, files); err != nil {
		t.Fatal(err)
	}
	if reloaded["HTTP_PORT"] != "8080" {
		t.Fatalf("expected persisted override, got %#v", reloaded)
	}

	t.Setenv("HTTP_PORT", "9090")
	explicit := map[string]string{}
	if err := mergePersistedWorkloadPortOverrides(explicit, files); err != nil {
		t.Fatal(err)
	}
	if _, ok := explicit["HTTP_PORT"]; ok {
		t.Fatalf("explicit process environment must take precedence, got %#v", explicit)
	}
}
