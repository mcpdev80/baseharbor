package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/machine"

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

func TestAmbiguousTargetProducesTypedError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{"a": {}, "b": {}}}
	_, err := selectedTargetName("", "", cfg)
	typed := machine.Classify(err)
	if typed.Code != machine.ErrorConflict || typed.Next == "" {
		t.Fatalf("expected actionable conflict: %#v", typed)
	}
}

func TestDoctorTargetFindingsStayManual(t *testing.T) {
	checks := []health.Check{
		{Name: "target-selection", OK: false, Message: "ambiguous"},
		{Name: "target-access", OK: false, Message: "not connected"},
		{Name: "selected-runtime", OK: false, Message: "unavailable"},
	}
	findings := classifyDoctorFindings(checks)
	if len(findings) != 3 {
		t.Fatalf("got %d findings", len(findings))
	}
	if hasAutoFixableDoctorFinding(findings) {
		t.Fatal("target selection must never trigger implicit runtime mutation")
	}
	for _, finding := range findings {
		if finding.Action == "" {
			t.Fatalf("missing next action for %s", finding.Check.Name)
		}
	}
}

func TestExplicitImplicitLocalSelectionPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{"docker-test": {}}}
	if err := writePersistedTarget("local"); err != nil {
		t.Fatal(err)
	}
	selected, err := selectedTargetName("", "", cfg)
	if err != nil || selected != "local" {
		t.Fatalf("explicit local: %q, %v", selected, err)
	}
}

func TestDeleteTargetClearsPersistedSelection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{"docker-test": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}}}, Access: map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := writePersistedTarget("docker-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := deleteTargetDefinition(context.Background(), "docker-test"); err != nil {
		t.Fatal(err)
	}
	selected, err := readPersistedTarget()
	if err != nil || selected != "" {
		t.Fatalf("deleted selection remains %q: %v", selected, err)
	}
}
