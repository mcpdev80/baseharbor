package runtimebroker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureImageCreatesDefaultState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	if got != DefaultImage {
		t.Fatalf("ensureImage() = %q, want %q", got, DefaultImage)
	}

	assertBrokerImageState(t, dir, DefaultImage)
}

func TestEnsureImageKeepsExistingStateWithoutExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	want := "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0"
	if got != want {
		t.Fatalf("ensureImage() = %q, want %q", got, want)
	}
	assertBrokerImageState(t, dir, want)
}

func TestEnsureImageKeepsExistingStateForSameExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	want := "ghcr.io/mcpdev80/baseharbor-runtime:edge"
	writeBrokerImageState(t, dir, want)
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", want)

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	if got != want {
		t.Fatalf("ensureImage() = %q, want %q", got, want)
	}
	assertBrokerImageState(t, dir, want)
}

func TestEnsureImageReconcilesExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:dev-local")
	want := "ghcr.io/mcpdev80/baseharbor-runtime:edge"
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", want)

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	if got != want {
		t.Fatalf("ensureImage() = %q, want %q", got, want)
	}
	assertBrokerImageState(t, dir, want)
}

func TestEnsureImageRejectsInvalidExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "ghcr.io/mcpdev80/baseharbor-runtime:edge\nbad")

	_, err := ensureImage(dir)
	if err == nil || !strings.Contains(err.Error(), "BASEHARBOR_RUNTIME_IMAGE is invalid") {
		t.Fatalf("ensureImage() error = %v, want invalid override error", err)
	}

	assertBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0")
}

func TestEnsureImageRejectsInvalidPersistedState(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "ghcr.io/mcpdev80/baseharbor-runtime:edge")

	_, err := ensureImage(dir)
	if err == nil || !strings.Contains(err.Error(), "runtime broker image state is invalid") {
		t.Fatalf("ensureImage() error = %v, want invalid state error", err)
	}
}

func writeBrokerImageState(t *testing.T, dir, image string) {
	t.Helper()
	path := filepath.Join(dir, "broker-image")
	if err := os.WriteFile(path, []byte(image+"\n"), 0o600); err != nil {
		t.Fatalf("write broker image state: %v", err)
	}
}

func assertBrokerImageState(t *testing.T, dir, want string) {
	t.Helper()
	path := filepath.Join(dir, "broker-image")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read broker image state: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("broker image state = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat broker image state: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("broker image state mode = %o, want 600", got)
	}
}
