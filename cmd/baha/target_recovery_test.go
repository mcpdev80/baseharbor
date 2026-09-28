package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestTargetRecoveryFileResolutionAndPersistence(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	ctx := context.Background()
	path, source, err := resolveTargetRecoveryFile(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	wantDefault := filepath.Join(dataRoot, "baseharbor-recovery", "local", "openbao-recovery.json")
	if path != wantDefault || source != "target default" {
		t.Fatalf("default recovery resolution = %q (%s), want %q (target default)", path, source, wantDefault)
	}

	custom := filepath.Join(t.TempDir(), "custom-recovery.json")
	if err := persistTargetRecoveryFileReference(ctx, custom); err != nil {
		t.Fatal(err)
	}
	path, source, err = resolveTargetRecoveryFile(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	absCustom, _ := filepath.Abs(custom)
	if path != absCustom || source != "persisted target" {
		t.Fatalf("persisted recovery resolution = %q (%s), want %q (persisted target)", path, source, absCustom)
	}

	override := filepath.Join(t.TempDir(), "override.json")
	path, source, err = resolveTargetRecoveryFile(ctx, override)
	if err != nil {
		t.Fatal(err)
	}
	absOverride, _ := filepath.Abs(override)
	if path != absOverride || source != "explicit" {
		t.Fatalf("explicit recovery resolution = %q (%s), want %q (explicit)", path, source, absOverride)
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	local, ok := cfg.Targets["local"]
	if !ok {
		t.Fatal("persisting the recovery-file reference must materialize the implicit local target")
	}
	if local.OpenBao.RecoveryFile != absCustom {
		t.Fatalf("target path reference not persisted: %+v", local)
	}
	if local.Runtime.Provider == "" || local.Access.Reference == "" {
		t.Fatalf("materialized local target is incomplete: %+v", local)
	}
	if _, err := os.Stat(absCustom); !os.IsNotExist(err) {
		t.Fatalf("persisting a recovery-file reference must not create recovery material: %v", err)
	}
}


func TestPreflightNewTargetRecoveryFileRejectsExistingMaterial(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	ctx := context.Background()
	path, err := defaultTargetRecoveryFile("local")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale recovery material\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err = preflightNewTargetRecoveryFile(ctx, "")
	if err == nil {
		t.Fatal("preflight accepted existing recovery material")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("existing recovery material was changed: %v", statErr)
	}

	fresh := filepath.Join(t.TempDir(), "fresh-recovery.json")
	got, source, err := preflightNewTargetRecoveryFile(ctx, fresh)
	if err != nil {
		t.Fatalf("fresh recovery preflight: %v", err)
	}
	absFresh, _ := filepath.Abs(fresh)
	if got != absFresh || source != "explicit" {
		t.Fatalf("fresh recovery preflight = %q (%s), want %q (explicit)", got, source, absFresh)
	}
}
