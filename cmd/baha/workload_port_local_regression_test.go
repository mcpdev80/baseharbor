package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestParseRuntimeUpOptionsAcceptsWorkloadPort proves the documented
// --workload-port NAME=PORT flag reaches the local `baha up` parser and sets the
// same name->port map the remote management path produces. A parse divergence
// here is exactly the bug 0428-199 reports.
func TestParseRuntimeUpOptionsAcceptsWorkloadPort(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--workload-port", "web=9090"})
	if err != nil {
		t.Fatalf("up --workload-port must be accepted locally: %v", err)
	}
	if len(opts.WorkloadPorts) != 1 || opts.WorkloadPorts["web"] != "9090" {
		t.Fatalf("unexpected workload ports: %#v", opts.WorkloadPorts)
	}
}

// TestParseRuntimeUpOptionsWorkloadPortEqualsForm covers the
// "--workload-port=NAME=PORT" form too.
func TestParseRuntimeUpOptionsWorkloadPortEqualsForm(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--workload-port=web=8123"})
	if err != nil {
		t.Fatalf("up --workload-port=NAME=PORT must be accepted locally: %v", err)
	}
	if opts.WorkloadPorts["web"] != "8123" {
		t.Fatalf("workload port = %q, want 8123", opts.WorkloadPorts["web"])
	}
}

// TestParseRuntimeUpOptionsRejectsMalformedWorkloadPort fails closed on a
// malformed selector so the documented option is never silently dropped.
func TestParseRuntimeUpOptionsRejectsMalformedWorkloadPort(t *testing.T) {
	for _, args := range [][]string{
		{"--workload-port", "web"},                               // missing =PORT
		{"--workload-port", "99999"},                             // no NAME
		{"--workload-port", "web=70000"},                         // out of range
		{"--workload-port", "web=1", "--workload-port", "web=2"}, // duplicate
	} {
		if _, err := parseRuntimeUpOptions(args); err == nil {
			t.Fatalf("up %v must fail closed for malformed workload port", args)
		}
	}
}

// TestExtractLocalWorkloadPorts mirrors the remote contract: it parses the
// documented flag, drops it from the remaining args, and validates against the
// same rules as extractRemoteWorkloadPorts.
func TestExtractLocalWorkloadPorts(t *testing.T) {
	remaining, ports, err := extractLocalWorkloadPorts([]string{"my-app", "--workload-port", "web=9090", "--yes"})
	if err != nil {
		t.Fatalf("extractLocalWorkloadPorts failed: %v", err)
	}
	for _, arg := range remaining {
		if arg == "--workload-port" {
			t.Fatalf("flag was not stripped from remaining args: %#v", remaining)
		}
	}
	if ports["web"] != "9090" {
		t.Fatalf("parsed port = %q, want 9090", ports["web"])
	}
}

// TestExtractLocalWorkloadPortsFailsClosedMirrorsRemote covers the same
// fail-closed selectors the remote path enforces.
func TestExtractLocalWorkloadPortsFailsClosedMirrorsRemote(t *testing.T) {
	for _, args := range [][]string{
		{"--workload-port"},                                      // missing =PORT value
		{"--workload-port", "99999"},                             // no NAME
		{"--workload-port", "web=70000"},                         // out of range
		{"--workload-port", "web=1", "--workload-port", "web=2"}, // duplicate
	} {
		if _, _, err := extractLocalWorkloadPorts(args); err == nil {
			t.Fatalf("extractLocalWorkloadPorts(%v) must fail closed", args)
		}
	}
}

// TestAppApplyRunAcceptsWorkloadPort drives the real 'baha app apply' command
// surface. The documented flag must no longer be rejected as an extra NAME;
// the command may fail later at target/manifest resolution, but never with the
// "accepts at most one NAME" symptom of the ignored-flag bug.
func TestAppApplyRunAcceptsWorkloadPort(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET_LOCAL_URL", "tcp://127.0.0.1:1")

	var out, errOut bytes.Buffer
	err := appApplyCommand(application.Store{}).Run(machineNoninteractiveContext(t.Context()),
		[]string{"--workload-port", "web=9090"}, &out, &errOut)
	if err != nil && strings.Contains(err.Error(), "accepts at most one NAME") {
		t.Fatalf("app apply still rejects --workload-port as an extra NAME: %v", err)
	}
}

// TestUpRunAcceptsWorkloadPort drives the real 'baha up' command surface. The
// documented flag must no longer surface as "unknown option --workload-port".
func TestUpRunAcceptsWorkloadPort(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET_LOCAL_URL", "tcp://127.0.0.1:1")

	var out, errOut bytes.Buffer
	err := runWithIO(machineNoninteractiveContext(t.Context()),
		[]string{"up", "--workload-port", "web=9090", "--plain"}, &out, &errOut)
	if err != nil && strings.Contains(err.Error(), "unknown option --workload-port") {
		t.Fatalf("up still rejects --workload-port as unknown: %v", err)
	}
}
