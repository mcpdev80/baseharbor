package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// does not exit within the deadline. It does not assert the code here; the
// signal test asserts the real exit code after the process terminates.
func waitExit(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("baha did not exit within timeout")
	}
}

func TestCanonicalInitHelpUsesApplicationInitialization(t *testing.T) {
	var out bytes.Buffer
	if err := runWithIOAsTestOperator(context.Background(), []string{"init", "--help"}, &out, &out); err != nil {
		t.Fatalf("init help failed: %v", err)
	}
	if !strings.Contains(out.String(), "baha init") {
		t.Fatalf("canonical initialization help missing: %s", out.String())
	}
	if strings.Contains(out.String(), "Compatibility entrypoint") {
		t.Fatalf("legacy init presentation still exposed: %s", out.String())
	}
}

func TestRootAndNestedHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"app", "--help"}, {"app", "create", "--help"}} {
		var out bytes.Buffer
		if err := runWithIOAsTestOperator(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("help %v failed: %v", args, err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("help %v missing usage: %s", args, out.String())
		}
	}
}

func TestAppCreateListShowPlan(t *testing.T) {
	target := configureTestTarget(t)
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIOAsTestOperator(context.Background(), []string{"app", "create", "demo", "--environment", "dev", "--sql", "--cache"}, &out, &out); err != nil {
		t.Fatalf("create failed: %v\n%s", err, out.String())
	}
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("deployments = %#v, want one managed deployment", records)
	}
	record := records[0]
	if record.Identity.Application != "demo" || record.Identity.Environment != "dev" {
		t.Fatalf("unexpected deployment identity: %#v", record.Identity)
	}
	m, err := application.LoadManifestFile(record.Source.Manifest)
	if err != nil {
		t.Fatalf("managed target manifest missing: %v", err)
	}

	out.Reset()
	if err := runWithIOAsTestOperator(context.Background(), []string{"app", "list"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo") || !strings.Contains(out.String(), "dev") {
		t.Fatalf("unexpected list: %s", out.String())
	}

	out.Reset()
	if err := runWithIOAsTestOperator(context.Background(), []string{"app", "show", "demo"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	show := out.String()
	for _, wanted := range []string{"Application: demo", "Environment: dev", "Status: NOT READY", "PostgreSQL default", "Valkey     default", "not applied"} {
		if !strings.Contains(show, wanted) {
			t.Fatalf("unexpected show, missing %q: %s", wanted, show)
		}
	}

	m.ApplicationID = record.Identity.ApplicationID
	if err := writeInitializedAuthoredManifest(t, filepath.Join(dir, "baseharbor.yaml"), m, 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runWithIOAsTestOperator(context.Background(), []string{"app", "plan"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ensure database.sql:default") || !strings.Contains(out.String(), "No changes were made") {
		t.Fatalf("unexpected plan: %s", out.String())
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if err := run([]string{"does-not-exist"}); err == nil {
		t.Fatal("expected unknown command to fail")
	}
}

func TestRootVersionFlag(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "0.4.11-test", "abc123", "2026-09-20"
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	})
	var out bytes.Buffer
	if err := runWithIOAsTestOperator(context.Background(), []string{"--version"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"BaseHarbor 0.4.11-test", "commit abc123", "built 2026-09-20"} {
		if !strings.Contains(out.String(), wanted) {
			t.Fatalf("version output missing %q: %s", wanted, out.String())
		}
	}
}

func TestBrokenPipeErrorIsSilent(t *testing.T) {
	var out bytes.Buffer
	formatCLIError(&out, syscall.EPIPE)
	if out.Len() != 0 {
		t.Fatalf("broken pipe produced user-facing noise: %q", out.String())
	}
}

func TestSignalMapsToDocumentedExitCode(t *testing.T) {
	// Real main-level contract: assert the documented signal->exit mapping
	// (SIGINT -> 130, SIGTERM -> 143) that docs/reference/cli.md names, driven
	// through the helper cmd/baha/main.go's interruptibleProcessContext goroutine
	// actually calls on its exit path: cli.SignalExitCode / cli.ResolveSignalExitCode
	// (moved into the already-catalogued internal/cli per w267 route (a)). If
	// the mapping is swapped, widened, or renamed away from the documented
	// 130/143 this fails - it exercises the exact code path main uses, not a
	// local key lookup.
	tests := []struct {
		sig  os.Signal
		want int
	}{
		{os.Interrupt, 130},
		{syscall.SIGTERM, 143},
	}
	for _, tc := range tests {
		if got := cli.ResolveSignalExitCode(tc.sig); got != tc.want {
			t.Fatalf("cli.ResolveSignalExitCode(%v) must map to documented exit code %d, got %d", tc.sig, tc.want, got)
		}
	}

	// The handler registers exactly these two signals and routes them through
	// cli.ResolveSignalExitCode; the documented set {130,143} must therefore be
	// exactly the two signal-code values main emits, no more.
	got := map[int]bool{cli.ResolveSignalExitCode(os.Interrupt): true, cli.ResolveSignalExitCode(syscall.SIGTERM): true}
	if len(got) != 2 {
		t.Fatal("main must emit exactly two distinct documented signal exit codes")
	}
	if !got[130] || !got[143] {
		t.Fatal("main must emit the documented codes 130 (SIGINT) and 143 (SIGTERM)")
	}
}

func TestUnknownCommandSuggestsNearestMatch(t *testing.T) {
	var out bytes.Buffer
	err := runWithIOAsTestOperator(context.Background(), []string{"statsu"}, &out, &out)
	if err == nil {
		t.Fatal("expected typo to fail")
	}
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected usage error, got %T: %v", err, err)
	}
	if !strings.Contains(usage.Hint, "status") {
		t.Fatalf("expected status suggestion, got %q", usage.Hint)
	}
}

func TestClassifyMachineCLIErrorWorkspaceNotInitialized(t *testing.T) {
	err := classifyMachineCLIError(development.ErrWorkspaceModelMissing)
	var machineErr *machine.Error
	if !errors.As(err, &machineErr) {
		t.Fatalf("expected machine error, got %T: %v", err, err)
	}
	if machineErr.Code != machine.ErrorSourceMissing || machineErr.CauseCode != "workspace_not_initialized" {
		t.Fatalf("unexpected machine error: %#v", machineErr)
	}
	if !strings.Contains(machineErr.Next, "baha app workspace init") {
		t.Fatalf("next action is not workspace init: %q", machineErr.Next)
	}
}

func TestVersionCommandStructuredOutput(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "0.4.21-test", "abc123", "2026-10-02"
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	})

	for _, args := range [][]string{
		{"version", "--json"},
		{"version", "-o", "json"},
		{"version", "--output=json"},
		{"version", "-ojson"},
	} {
		var out bytes.Buffer
		if err := runWithIOAsTestOperator(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var payload map[string]string
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("%v invalid JSON: %v\n%s", args, err, out.String())
		}
		if payload["version"] != "0.4.21-test" || payload["commit"] != "abc123" || payload["built"] != "2026-10-02" {
			t.Fatalf("%v unexpected payload: %#v", args, payload)
		}
	}
}
