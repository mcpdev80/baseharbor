package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// discoverLocalRuntimeState builds an isolated BaseHarbor data root with a
// registered non-local Target and a "local" target under targets/local/, then
// reports whether the local discovery result blocks a full destroy. The
// pre-existing TestMissingLocalTargetRegistrationNeverInventsComposeDestroyAuthority
// already covers the present-runtime-owner case; this helper parametrizes the
// targets/local/ layout so the three #1453 regression branches can share setup
// without duplicating the fixture.
func discoverLocalRuntimeState(t *testing.T, setupLocal func(root string)) (provider, detail string, blocked bool) {
	t.Helper()
	configureTestTarget(t)
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatalf("data root: %v", err)
	}
	if setupLocal != nil {
		setupLocal(root)
	}
	targets, discovery := discoverFullDestroyTargets()
	for _, tg := range targets {
		if tg.Name == "local" {
			provider = tg.RuntimeProvider
		}
	}
	for _, item := range discovery {
		if item.Target == "local" && item.Resource == "runtime-provider" {
			detail = item.Detail
		}
	}
	blocked = countFullDestroyBlockers(discovery) > 0
	if provider != "" {
		t.Fatalf("local target must never invent a runtime provider, got %q", provider)
	}
	return provider, detail, blocked
}

// (a) deployment-only "local" state (targets/local/ with only a stale
// deployment folder, no runtime/) stays a non-blocking recovery SKIPPED so a
// stale local retry is not wedged.
func TestInstallationDestroyLocalOrphanDeploymentFolderDoesNotBlock(t *testing.T) {
	_, _, blocked := discoverLocalRuntimeState(t, func(root string) {
		if err := os.MkdirAll(filepath.Join(root, "targets", "local", "stale-deploy"), 0700); err != nil {
			t.Fatal(err)
		}
	})
	if blocked {
		t.Fatal("deployment-only local orphan must not block destroy; a stale retry must remain possible")
	}
}

// (b) an existing retained local runtime owner (targets/local/runtime/) stays a
// blocking "preserved because" SKIPPED, preserving #1448's fail-closed
// local-runtime scoping.
func TestInstallationDestroyLocalRuntimeOwnerStillBlocks(t *testing.T) {
	_, detail, blocked := discoverLocalRuntimeState(t, func(root string) {
		if err := os.MkdirAll(filepath.Join(root, "targets", "local", "runtime"), 0700); err != nil {
			t.Fatal(err)
		}
	})
	if !blocked {
		t.Fatal("retained local runtime owner must remain a blocking preserved SKIPPED")
	}
	if detail == "" || !strings.Contains(detail, "preserved because") {
		t.Fatalf("blocking local runtime owner detail must be the #1448 preserved reason, got %q", detail)
	}
}

// (c) a non-ENOENT stat failure on the local runtime owner path (permission
// loss, I/O error) must NOT be interpreted as absence; it stays blocking. The
// failure is injected by replacing the single localRuntimeOwnerStatErr seam so
// the branch does not depend on filesystem permissions, which differ on
// privileged CI runners.
func TestInstallationDestroyLocalRuntimeOwnerStatFailureBlocks(t *testing.T) {
	orig := localRuntimeOwnerStatErr
	localRuntimeOwnerStatErr = func(root, target string) error {
		return &os.PathError{Op: "stat", Path: filepath.Join(root, "targets", target, "runtime"), Err: errors.New("permission denied")}
	}
	defer func() { localRuntimeOwnerStatErr = orig }()
	_, detail, blocked := discoverLocalRuntimeState(t, func(root string) {
		// A bare targets/local/ dir (no runtime/) with an injected non-ENOENT
		// stat failure stands in for an unreadable-but-present owner.
		if err := os.MkdirAll(filepath.Join(root, "targets", "local"), 0700); err != nil {
			t.Fatal(err)
		}
	})
	if !blocked {
		t.Fatal("a non-ENOENT local runtime owner stat failure must block destroy, not be treated as absence")
	}
	if detail == "" || !strings.Contains(detail, "preserved because") {
		t.Fatalf("non-ENOENT stat failure must produce the blocking preserved reason, got %q", detail)
	}
}

// (d) the ENOENT case (runtime owner genuinely absent) must remain the single
// non-blocking orphan path, so a regression that treats only success as
// present does not also swallow the retry case.
func TestInstallationDestroyLocalRuntimeOwnerAbsentIsNotBlocking(t *testing.T) {
	orig := localRuntimeOwnerStatErr
	localRuntimeOwnerStatErr = func(root, target string) error {
		return &os.PathError{Op: "stat", Path: filepath.Join(root, "targets", target, "runtime"), Err: os.ErrNotExist}
	}
	defer func() { localRuntimeOwnerStatErr = orig }()
	_, _, blocked := discoverLocalRuntimeState(t, func(root string) {
		if err := os.MkdirAll(filepath.Join(root, "targets", "local"), 0700); err != nil {
			t.Fatal(err)
		}
	})
	if blocked {
		t.Fatal("a genuinely absent local runtime owner (ENOENT) must remain non-blocking")
	}
}
