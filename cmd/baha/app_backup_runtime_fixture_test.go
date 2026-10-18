package main

import (
	"context"
	"errors"
	"fmt"
	"time"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
	"strings"
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

func TestAppBackupCoreStateUsesSelectedTargetForNativeRuntimes(t *testing.T) {
	for _, provider := range []string{"docker", "podman"} {
		t.Run(provider, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("BASEHARBOR_TEST_RUNTIME", provider)
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(root, "unrelated-global-compose-state"))
			configureAppBackupRuntimeTarget(t, filepath.Join(root, "config"), filepath.Join(root, "data"))
			target, err := effectiveTarget(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			stateDir, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			// Use the actual Core owner to generate its runtime input
			// definition; no Compose artifacts are fabricated for Podman.
			want, err := bhruntime.EnsureFilesForProjectAndResources(
				stateDir, targetRuntimeProjectName(target),
				bhruntime.SharedResourceProjectName(target.Name),
				bhruntime.Ports{Postgres: 15432, OpenBao: 18200}, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bhruntime.ExistingFiles(""); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("isolated global lookup unexpectedly succeeds: %v", err)
			}
			got, err := existingTargetRuntimeFiles(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.Compose != want.Compose || got.Env != want.Env ||
				got.Project != want.Project || got.ResourceProject != want.ResourceProject {
				t.Fatalf("selected Core runtime differs: got %+v, want %+v", got, want)
			}
			if !strings.HasPrefix(got.Compose, stateDir+string(os.PathSeparator)) {
				t.Fatalf("Core files escaped selected Target: %q", got.Compose)
			}
			if err := os.Remove(want.Compose); err != nil {
				t.Fatal(err)
			}
			if _, err := existingTargetRuntimeFiles(context.Background()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing selected Core input must fail closed: %v", err)
			}
		})
	}
}

func TestAppBackupOwnedCoreFixtureCleanupOnEveryExit(t *testing.T) {
	for _, scenario := range []struct {
		name string
		startErr error
		readyErr error
	}{
		{name: "initialized and authenticated"},
		{name: "partial startup error", startErr: errors.New("partial start")},
		{name: "readiness refuses initialized without manager", readyErr: errors.New("manager auth unavailable")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			owned := filepath.Join(root, "owned")
			foreign := filepath.Join(root, "foreign")
			if err := os.WriteFile(owned, []byte("owned"), 0600); err != nil { t.Fatal(err) }
			if err := os.WriteFile(foreign, []byte("foreign"), 0600); err != nil { t.Fatal(err) }
			calls := []string{}
			cancelled, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() {
				if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("owned state leaked: %v", err)
				}
				if _, err := os.Stat(foreign); err != nil {
					t.Errorf("foreign sentinel affected: %v", err)
				}
				want := []string{"start", "cleanup"}
				if scenario.startErr == nil { want = []string{"start", "ready", "cleanup"} }
				if fmt.Sprint(calls) != fmt.Sprint(want) {
					t.Errorf("fixture operations %v, want %v", calls, want)
				}
			})
			err := runOwnedBackupCoreFixture(t, cancelled,
				func(ctx context.Context) error { calls = append(calls, "start"); cancel(); return scenario.startErr },
				func(ctx context.Context) error { calls = append(calls, "ready"); return scenario.readyErr },
				func(ctx context.Context) error {
					calls = append(calls, "cleanup")
					if ctx.Err() != nil { return fmt.Errorf("cleanup inherited cancelled context") }
					if _, ok := ctx.Deadline(); !ok { return fmt.Errorf("cleanup missing bounded deadline") }
					return os.Remove(owned)
				},
			)
			if !errors.Is(err, scenario.startErr) && !errors.Is(err, scenario.readyErr) {
				if err != nil || scenario.startErr != nil || scenario.readyErr != nil {
					t.Errorf("failure cause lost: %v", err)
				}
			}
		})
	}
}

func TestAppBackupOwnedCoreCleanupContextIsBounded(t *testing.T) {
	var cleaned bool
	t.Run("registered before failing start", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_ = runOwnedBackupCoreFixture(t, ctx, func(context.Context) error {
			return errors.New("start failed")
		}, func(context.Context) error { t.Error("unexpected ready"); return nil },
		func(cleanup context.Context) error {
			cleaned = true
			if cleanup.Err() != nil { return cleanup.Err() }
			if deadline, ok := cleanup.Deadline(); !ok || time.Until(deadline) > 3*time.Minute {
				return errors.New("cleanup context not bounded")
			}
			return nil
		})
	})
	if !cleaned { t.Fatal("cleanup not executed on partial-start failure") }
}
