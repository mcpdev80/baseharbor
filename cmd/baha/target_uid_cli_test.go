package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// setupUIDTestRoot installs an isolated, tmp BaseHarbor state root (config +
// data) so the CLI writes only into t.TempDir and never touches host state.
func setupUIDTestRoot(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	// Only XDG_CONFIG_HOME/XDG_DATA_HOME are set (no BASEHARBOR_STATE_DIR), so
	// ConfigPath() nests under <home>/baseharbor/ — the path the collision
	// seed below writes to. BASEHARBOR_STATE_DIR would take precedence and
	// redirect the config elsewhere.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
}

// configPathForCollision returns the on-disk config path under the configured
// XDG_CONFIG_HOME (nested under baseharbor/, per ConfigPath()).
func configPathForCollision(t *testing.T) string {
	t.Helper()
	return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "baseharbor", "config.yaml")
}

// TestCLITargetCreateGeneratesStableUID exercises the real `baha target create`
// command end to end (global option parsing, dispatch, config write) and proves
// the stable Target UID is auto-generated (no manual UUID request) and persisted
// on the written definition through the real CLI entry point.
func TestCLITargetCreateGeneratesStableUID(t *testing.T) {
	setupUIDTestRoot(t)

	var out bytes.Buffer
	if err := runWithIOAsTestOperator(context.Background(), []string{
		"target", "create", "podman-prod",
		"--runtime-provider", "podman",
		"--access", "local-docker",
		"--access-provider", "local",
		"--reference", "local",
	}, &out, &out); err != nil {
		t.Fatalf("CLI target create: %v; output=%s", err, out.String())
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	def, ok := cfg.Targets["podman-prod"]
	if !ok {
		t.Fatal("created target missing from persisted config")
	}
	if def.TargetUID == "" {
		t.Fatal("CLI create did not auto-generate a stable Target UID")
	}
	if err := deployment.ValidateTargetUID(def.TargetUID); err != nil {
		t.Fatalf("generated UID invalid: %v", err)
	}
	// Ownership TenantID is a distinct role and stays untouched by UID creation.
	if def.TenantID != "" {
		t.Fatalf("UID creation must not touch the ownership TenantID: %q", def.TenantID)
	}
}

// TestCLITargetUIDStableAcrossReads proves the user-facing create + two reads
// yield ONE stable UID: the first create persists it, and a second
// `baha target list` (which loads the config through the migration seam)
// observes no churn — the idempotent read contract for legacy resolution.
func TestCLITargetUIDStableAcrossReads(t *testing.T) {
	setupUIDTestRoot(t)

	var out bytes.Buffer
	if err := runWithIOAsTestOperator(context.Background(), []string{
		"target", "create", "alpha",
		"--runtime-provider", "docker",
		"--access", "local-docker",
		"--access-provider", "local",
		"--reference", "local",
	}, &out, &out); err != nil {
		t.Fatalf("CLI create alpha: %v; output=%s", err, out.String())
	}
	cfg, _ := deployment.LoadConfig()
	first := cfg.Targets["alpha"].TargetUID
	if first == "" {
		t.Fatal("first create did not persist a UID")
	}

	// Second read through the real CLI: list must not churn the UID.
	var listOut bytes.Buffer
	if err := runWithIOAsTestOperator(context.Background(), []string{"target", "list", "-o", "json"}, &listOut, &listOut); err != nil {
		t.Fatalf("CLI target list: %v; output=%s", err, listOut.String())
	}
	cfg2, _ := deployment.LoadConfig()
	if cfg2.Targets["alpha"].TargetUID != first {
		t.Fatal("UID churned across CLI reads")
	}
}

// TestCLITargetUIDCollisionFailsClosedOnRead proves a corrupted config in which
// two Targets share one UID is rejected by the real CLI read path
// (`baha target list` -> LoadConfig -> Validate uniqueness): fail closed, no
// crash, and no data is moved. This is the collision fail-closed guard at the
// actual command entry point.
func TestCLITargetUIDCollisionFailsClosedOnRead(t *testing.T) {
	setupUIDTestRoot(t)

	uid, err := deployment.NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := configPathForCollision(t)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	colliding := "version: 1\n" +
		"access:\n  local-docker:\n    provider: docker\n    reference: local\n" +
		"targets:\n" +
		"  alpha:\n    target-uid: " + uid + "\n" +
		"    runtime:\n      provider: docker\n" +
		"    access:\n      reference: local-docker\n" +
		"  beta:\n    target-uid: " + uid + "\n" +
		"    runtime:\n      provider: docker\n" +
		"    access:\n      reference: local-docker\n"
	if err := os.WriteFile(cfgPath, []byte(colliding), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err = runWithIOAsTestOperator(context.Background(), []string{"target", "list", "-o", "json"}, &out, &out)
	if err == nil {
		t.Fatal("corrupted config with two Targets sharing a UID must fail closed at the CLI")
	}
	if !strings.Contains(err.Error(), uid) {
		t.Fatalf("collision error did not name the conflicting UID: %v", err)
	}
}
