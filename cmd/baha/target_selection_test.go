package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestPersistedTargetSelection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{"docker-test": {}, "podman-test": {}}}
	if _, err := selectedTargetName("", "", cfg); err == nil {
		t.Fatal("multiple targets must fail closed")
	}
	if err := writePersistedTarget("docker-test"); err != nil {
		t.Fatal(err)
	}
	path, err := targetSelectionPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("selection permissions %v", info.Mode().Perm())
	}
	if filepath.Base(path) != "active-target" {
		t.Fatal(path)
	}
	tests := []struct{ explicit, env, want string }{
		{"", "", "docker-test"},
		{"", "podman-test", "podman-test"},
		{"podman-test", "", "podman-test"},
		{"docker-test", "podman-test", "docker-test"},
	}
	for _, tt := range tests {
		got, err := selectedTargetName(tt.explicit, tt.env, cfg)
		if err != nil || got != tt.want {
			t.Fatalf("got %q err %v want %q", got, err, tt.want)
		}
	}
	if err := clearPersistedTarget(); err != nil {
		t.Fatal(err)
	}
	if _, err := selectedTargetName("", "", cfg); err == nil {
		t.Fatal("selection ambiguity not reported")
	}
}
func TestSingleTargetAndStaleTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{"docker-test": {}}}
	got, err := selectedTargetName("", "", cfg)
	if err != nil || got != "docker-test" {
		t.Fatalf("single target %q: %v", got, err)
	}
	if err := writePersistedTarget("deleted"); err != nil {
		t.Fatal(err)
	}
	_, err = selectedTargetName("", "", cfg)
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("expected stale target error, got %v", err)
	}
}
