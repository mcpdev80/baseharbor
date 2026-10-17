package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestAppBackupGuidedRuntimeFixtureSelectsNativeTarget(t *testing.T) {
	for _, provider := range []string{"docker", "podman"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("BASEHARBOR_TEST_RUNTIME", provider)
			t.Setenv("BASEHARBOR_TARGET", "foreign-selected-target")
			root := t.TempDir()
			configureAppBackupRuntimeTarget(t, filepath.Join(root, "cfg"), filepath.Join(root, "data"))
			target, err := effectiveTarget(context.Background())
			if err != nil || target.RuntimeProvider != provider || target.Name != "backup-recovery-ci" {
				t.Fatalf("fixture lost requested native Target: %+v %v", target, err)
			}
			cfg, err := deployment.LoadConfig()
			if err != nil || cfg.DefaultTarget != target.Name {
				t.Fatalf("default Target not persisted: %+v %v", cfg, err)
			}
			if _, err := os.Stat(filepath.Join(root, "cfg")); err != nil {
				t.Fatalf("isolated config directory was not created: %v", err)
			}
		})
	}
}

func TestAppBackupGuidedRuntimeFixtureRejectsInvalidRuntime(t *testing.T) {
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "not-a-native-runtime")
	// The contract parser must reject unsupported selection before a
	// real target or container orchestration is ever started.
	kind, err := backupFixtureRuntimeKind()
	if err == nil || kind != "" {
		t.Fatalf("unsupported runtime accepted: kind=%q err=%v", kind, err)
	}
}
