package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		"Bind for ::1:443 failed: port is already allocated":               443,
		"Bind for [::1]:8443 failed: port is already allocated":            8443,
		"Bind for [::]:9443 failed: port is already allocated":             9443,
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

func TestEnsureRepositoryWorkloadPortsForUpPersistsFirstRunFallback(t *testing.T) {
	repo := t.TempDir()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	occupied := listener.Addr().(*net.TCPAddr).Port

	compose := fmt.Sprintf("services:\\n  edge:\\n    image: caddy:2-alpine\\n    ports:\\n      - \"${HTTP_PORT:-%d}:80\"\\n", occupied)
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\\nname: demo\\nenvironment: dev\\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := application.New("demo", "dev", false, false, false)
	resolved := resolvedApplication{
		Manifest:       m,
		ManifestPath:   manifestPath,
		Store:          application.Store{Root: filepath.Join(repo, ".baseharbor", "apps")},
		FromRepository: true,
	}
	var out bytes.Buffer
	if err := ensureRepositoryWorkloadPortsForUp(context.Background(), strings.NewReader("\\n"), &out, resolved, repo); err != nil {
		t.Fatal(err)
	}

	values, err := readSimpleEnvFile(repositoryInitEnvPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	got, err := strconv.Atoi(values["HTTP_PORT"])
	if err != nil {
		t.Fatalf("persisted HTTP_PORT = %q", values["HTTP_PORT"])
	}
	if got == occupied {
		t.Fatalf("first-run allocation kept occupied port %d", occupied)
	}
	if !portAvailable(got) {
		t.Fatalf("selected fallback port %d is not available", got)
	}
	if !strings.Contains(out.String(), "Workload host port") || !strings.Contains(out.String(), "saved for this deployment") {
		t.Fatalf("first-run fallback output missing allocation detail: %q", out.String())
	}
}

func TestEnsureRepositoryWorkloadPortsForUpPersistsAvailableDefault(t *testing.T) {
	repo := t.TempDir()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	compose := fmt.Sprintf("services:\\n  edge:\\n    image: caddy:2-alpine\\n    ports:\\n      - \"${HTTP_PORT:-%d}:80\"\\n", port)
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\\nname: demo\\nenvironment: dev\\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Manifest:       application.New("demo", "dev", false, false, false),
		ManifestPath:   manifestPath,
		Store:          application.Store{Root: filepath.Join(repo, ".baseharbor", "apps")},
		FromRepository: true,
	}
	var out bytes.Buffer
	if err := ensureRepositoryWorkloadPortsForUp(context.Background(), strings.NewReader(""), &out, resolved, repo); err != nil {
		t.Fatal(err)
	}
	values, err := readSimpleEnvFile(repositoryInitEnvPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	if values["HTTP_PORT"] != strconv.Itoa(port) {
		t.Fatalf("HTTP_PORT = %q, want %d", values["HTTP_PORT"], port)
	}
}

func TestUpdateRepositoryInitValuesPreservesDeploymentInputs(t *testing.T) {
	repo := t.TempDir()
	if err := updateRepositoryInitValues(repo, map[string]string{
		"BASEHARBOR_HOSTNAME": "demo.example.com",
		"BASEHARBOR_TLS_MODE": "existing",
	}); err != nil {
		t.Fatal(err)
	}
	if err := updateRepositoryInitValues(repo, map[string]string{"HTTP_PORT": "8080"}); err != nil {
		t.Fatal(err)
	}
	values, err := readSimpleEnvFile(repositoryInitEnvPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	if values["BASEHARBOR_HOSTNAME"] != "demo.example.com" || values["BASEHARBOR_TLS_MODE"] != "existing" || values["HTTP_PORT"] != "8080" {
		t.Fatalf("deployment values were not preserved: %#v", values)
	}
}
