package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestServeCommandManagementHostResolveErrorFailsClosed is the act-1405c round-4
// item 3 cell: when managementHostForServe resolves a NON-nil error (the target
// cannot be resolved), serveCommand returns that error to the caller and the
// control-plane runtime is NOT started. This is the fail-closed production
// boundary the round-3 code added; the regression proves it is covered.
func TestServeCommandManagementHostResolveErrorFailsClosed(t *testing.T) {
	// Isolate XDG so this test does not touch the real deployment config, and
	// force an unresolvable target so effectiveTarget (hence managementHostForServe)
	// returns a non-nil error on the management-host resolution path.
	xdir := filepath.Join(t.TempDir(), "xdg")
	if err := os.MkdirAll(filepath.Join(xdir, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(xdir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(xdir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(xdir, "data"))
	t.Setenv("BASEHARBOR_TARGET", "does-not-exist")
	t.Setenv("BASEHARBOR_CONNECTIVITY_RELAY_MODE", "false")
	t.Setenv("BASEHARBOR_RUNTIME_EXECUTOR_MODE", "false")
	os.Unsetenv("BASEHARBOR_RUNTIME_APP_NAME")

	// TLS files must exist for controlPlaneConfigFromEnv to succeed (it does not
	// validate them, but the config plumbing reads env). The resolution error
	// fires before any TLS requirement is reached.
	cmd := serveCommand(application.Store{Root: t.TempDir()})
	var out, errOut bytes.Buffer
	err := cmd.Run(context.Background(), nil, &out, &errOut)
	if err == nil {
		t.Fatalf("serveCommand with an unresolvable target returned nil error; want the management-host resolution error to fail closed")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "target") {
		t.Fatalf("serveCommand returned error %q; want it to be the management-host target resolution error (fail-closed), not a runtime start", msg)
	}
}
