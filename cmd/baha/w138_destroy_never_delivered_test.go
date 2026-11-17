package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestW138DestroyAfterRefusedApplyReturnsZero is the permanent regression for
// t-20261104-r27 (w138c). It proves the fix: an application that was
// initialized but never delivered (refused by the candidate consent choke on
// apply, or simply never applied) retires cleanly with rc=0 instead of failing
// closed with an "incomplete deployment state" error.
//
// It drives the REAL CLI in an isolated XDG state:
//  1. app init (initialize the application identity, no runtime)
//  2. app apply WITHOUT --confirm-candidate (refused: fail-closed, nothing written)
//  3. app destroy --yes (must return rc=0: nothing was ever delivered)
//
// It avoids Docker entirely: the refusal happens at the consent choke before any
// runtime is realized, and the never-delivered destroy has no runtime to tear
// down. A separate helper exercises the healthy re-apply path with the correct
// digest in a Docker-backed test that the CI matrix runs.
func TestW138DestroyAfterRefusedApplyReturnsZero(t *testing.T) {
	root := t.TempDir()
	old, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatal(wdErr)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeRepositorySQLApp(t, root)
	gitInitRepo(t, root)

	ctx := context.Background()
	var out bytes.Buffer

	// (1) app init: initialize the application identity.
	if err := runWithIO(ctx, []string{"app", "init", "demo-db", "--sql", "--workload-component", "demo-app"}, &out, &out); err != nil {
		t.Fatalf("app init: %v", err)
	}

	// (2) app apply WITHOUT confirm-candidate: the candidate choke refuses and
	// writes no deployment record. This is the fail-closed invariant that must
	// stay unchanged.
	out.Reset()
	err := runWithIO(ctx, []string{"app", "apply", "--no-input"}, &out, &out)
	if err == nil {
		t.Fatal("app apply without --confirm-candidate must fail closed")
	}

	// (3) app destroy --yes: nothing was ever delivered, so destroy must retire
	// the never-realized workload with rc=0 (the reported bug returned rc=1).
	out.Reset()
	derr := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out)
	if derr != nil {
		t.Fatalf("destroy of never-delivered application must return rc=0, got: %v\noutput:\n%s", derr, out.String())
	}
}

// TestW138DestroyNeverDeliveredNoComposeReturnsZero covers a workload that was
// initialized via `app init` but never delivered and has no compose source: the
// environment is in a well-defined "never delivered" state and destroy must
// retire it with rc=0 rather than failing closed.
func TestW138DestroyNeverDeliveredNoComposeReturnsZero(t *testing.T) {
	root := t.TempDir()
	old, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatal(wdErr)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	var out bytes.Buffer
	// app init creates the identity without any runtime (no compose to adopt).
	if err := runWithIO(context.Background(), []string{"app", "init", "solo", "--sql", "--no-input"}, &out, &out); err != nil {
		t.Fatalf("app init: %v", err)
	}
	if err := runWithIO(context.Background(), []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy of never-delivered application must return rc=0, got: %v", err)
	}
}

func gitInitRepo(t *testing.T, root string) {
	t.Helper()
	if err := runGit(t, root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(t, root, "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(t, root, "config", "user.name", "test"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(t, root, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(t, root, "commit", "-q", "-m", "init app"); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) error {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return joinErr(err, out)
	}
	return nil
}

func joinErr(err error, out []byte) error {
	if len(out) == 0 {
		return err
	}
	return &wrappedErr{err: err, detail: string(out)}
}

type wrappedErr struct {
	err    error
	detail string
}

func (e *wrappedErr) Error() string { return e.err.Error() + ": " + e.detail }
func (e *wrappedErr) Unwrap() error { return e.err }

func writeRepositorySQLApp(t *testing.T, root string) {
	t.Helper()
	// Only write the compose source here; `app init` creates the repository
	// manifest (and initializes the application identity). Writing the manifest
	// up front would make `app init` refuse because the manifest already exists.
	compose := "services:\n  demo-app:\n    build: .\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeRepositoryWorkloadOnlyApp(t *testing.T, root string) {
	t.Helper()
	compose := "services:\n  demo-app:\n    build: .\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestW138MissingRecordAfterPersistenceMustNotClaimNeverDelivered(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeRepositorySQLApp(t, root)
	gitInitRepo(t, root)
	ctx := context.Background()
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "demo-db", "--sql", "--workload-component", "demo-app"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	// This case deliberately persists a deployment before deleting its record.
	// Provision the actual managed-state owner ONLY for this persisted-record
	// scenario; the separate never-delivered Coreless cases remain Core-free.
	if err := registerInstallationForTarget(t, "local"); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordPendingDeployment(ctx, resolved); err != nil {
		t.Fatal(err)
	}
	if err := deployment.VerifyNeverDeliveredProof(resolved.DeploymentIdentity); !os.IsNotExist(err) {
		t.Fatalf("first deployment write must consume initialization proof, got %v", err)
	}
	// Simulate lost deployment.json with potentially still-owned runtime: no
	// fabricated RETIRED result and no provider/registry mutation permitted.
	record := filepath.Join(resolved.DeploymentStateRoot, "deployment.json")
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err == nil {
		t.Fatal("lost deployment record must fail closed even if the app identity remains")
	}
}

func TestW138DestroyWithoutYesPreservesNeverDeliveredProof(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeRepositorySQLApp(t, root)
	gitInitRepo(t, root)
	ctx := context.Background()
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "demo-db", "--sql", "--workload-component", "demo-app"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "destroy")
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = runWithIO(ctx, []string{"app", "destroy"}, &out, &out)
	if err := deployment.VerifyNeverDeliveredProof(resolved.DeploymentIdentity); err != nil {
		t.Fatalf("destroy without --yes must not consume initialization proof: %v", err)
	}
}
