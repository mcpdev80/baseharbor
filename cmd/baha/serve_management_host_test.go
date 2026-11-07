package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

const (
	serveManagementHostChildEnv   = "BAHA_TEST_SERVE_MH_CHILD"
	serveManagementHostModeEnv    = "BAHA_TEST_SERVE_MH_MODE"
	serveManagementHostReadyEnv   = "BAHA_TEST_SERVE_MH_READY"
	serveManagementHostErrExit    = 3
	serveManagementHostHolderExit = 7
)

// serveManagementHostConfigureTargetOnce installs a single, isolated local Core
// installation Target under one pair of XDG dirs shared by every process in this
// cell, and records them on the process environment so child processes resolve
// the SAME durable state root. It must be called exactly once in the parent: a
// child that calls configureTestTarget would rewrite the XDG dirs for its own
// temp instance and resolve a different root.
func serveManagementHostConfigureTargetOnce(t *testing.T) {
	xdir := filepath.Join(t.TempDir(), "xdg")
	if err := os.MkdirAll(filepath.Join(xdir, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(xdir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(xdir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(xdir, "data"))
	// An empty installation state root makes Verify report Bootstrap==true, so
	// the two-process contention exercises the bootstrap ownership guard on a
	// genuinely fresh durable root, exactly the missing-root serve path the
	// act-1405b recipe targets.
	configureTestTargetEmpty(t)
}

// TestServeManagementHostBootstrapTwoProcesses is the serveCommand-level,
// causal regression cell required by act-1405b round 3. Two separate OS
// processes contend on the SAME fresh (bootstrap) installation state root the
// serve path's production ownership guard resolves through managementHostForServe.
// The first process (holder) retains bootstrap ownership while it is up; a second
// process is refused with ErrManagementHostOwnedFreshRoot, proving the serve path
// holds exclusive ownership through bind instead of releasing the lock before
// binding. The holder signals readiness via a marker file so the challenger runs
// only while the lock is genuinely held.
func TestServeManagementHostBootstrapTwoProcesses(t *testing.T) {
	serveManagementHostConfigureTargetOnce(t)
	ready := filepath.Join(t.TempDir(), "ready")

	// First process: holder. It runs the serve-path guard (managementHostForServe
	// + Verify) on the fresh bootstrap root, acquires and holds the bootstrap lock
	// (the production ownership guard), then signals readiness and waits.
	holder := exec.Command(os.Args[0], "-test.run=^TestServeManagementHostBootstrapHolder$")
	holder.Env = append(os.Environ(),
		serveManagementHostChildEnv+"=1",
		serveManagementHostModeEnv+"=holder",
		serveManagementHostReadyEnv+"="+ready,
		"BASEHARBOR_RUNTIME_APP_NAME=",
		"BASEHARBOR_RUNTIME_SECRETS_ENABLED=false",
	)
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill() }()
	waitForHolderReady(t, ready, holder)

	// Second process: challenger on the same fresh root. It runs the same
	// serve-path guard and must be refused with the bootstrap-owned error while
	// the holder holds the lock.
	challenger := exec.Command(os.Args[0], "-test.run=^TestServeManagementHostBootstrapChallenger$")
	challenger.Env = append(os.Environ(),
		serveManagementHostChildEnv+"=1",
		serveManagementHostModeEnv+"=challenger",
		"BASEHARBOR_RUNTIME_APP_NAME=",
		"BASEHARBOR_RUNTIME_SECRETS_ENABLED=false",
	)
	out, cerr := challenger.CombinedOutput()
	if cerr == nil {
		t.Fatalf("second serve guard process was not refused while the first held bootstrap ownership; output:\n%s", out)
	}
	if !strings.Contains(string(out), "already owned by a concurrent writer") {
		t.Fatalf("second process refused for the wrong reason; want the bootstrap-owned refusal ErrManagementHostOwnedFreshRoot, got:\n%s", out)
	}
}

// TestServeManagementHostBootstrapProductionGuard proves the causal source
// mutation of the production ownership guard at the serveCommand seam: the guard
// is the single AcquireBootstrapLock(m.stateRoot) call in
// internal/coreinstallation/management_host.go Verify(). With the guard in force,
// a second concurrent serve guard on the same fresh root is refused (this cell
// passes). With the guard removed, the second serve guard no longer refuses and
// the cell fails. Both results are recorded in the evidence dossier with file
// hashes.
func TestServeManagementHostBootstrapProductionGuard(t *testing.T) {
	serveManagementHostConfigureTargetOnce(t)
	ready := filepath.Join(t.TempDir(), "ready")

	holder := exec.Command(os.Args[0], "-test.run=^TestServeManagementHostBootstrapHolder$")
	holder.Env = append(os.Environ(),
		serveManagementHostChildEnv+"=1",
		serveManagementHostModeEnv+"=holder",
		serveManagementHostReadyEnv+"="+ready,
		"BASEHARBOR_RUNTIME_APP_NAME=",
		"BASEHARBOR_RUNTIME_SECRETS_ENABLED=false",
	)
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill() }()
	waitForHolderReady(t, ready, holder)

	challenger := exec.Command(os.Args[0], "-test.run=^TestServeManagementHostBootstrapChallenger$")
	challenger.Env = append(os.Environ(),
		serveManagementHostChildEnv+"=1",
		serveManagementHostModeEnv+"=challenger",
	)
	out, cerr := challenger.CombinedOutput()
	// GREEN: with the guard in force the second serve guard fails closed and the
	// cell passes; the guard removal (RED) is recorded in the evidence dossier.
	if cerr == nil {
		t.Fatalf("second serve guard process was not refused while the first held bootstrap ownership; output:\n%s", out)
	}
	if !strings.Contains(string(out), "already owned by a concurrent writer") {
		t.Fatalf("second process refused for the wrong reason; want ErrManagementHostOwnedFreshRoot, got:\n%s", out)
	}
}

// waitForHolderReady blocks until the holder has written its readiness marker,
// proving it acquired the bootstrap lock and is holding it. A holder that exits
// before the marker means it never held ownership, which is a test setup
// failure, not a test result.
func waitForHolderReady(t *testing.T, ready string, holder *exec.Cmd) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			return
		}
		if holder.ProcessState != nil {
			t.Fatalf("holder serve guard process exited (%v) before holding bootstrap ownership", holder.ProcessState.ExitCode())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if holder.ProcessState != nil {
		t.Fatalf("holder serve guard process exited early (exit=%v); see output above", holder.ProcessState.ExitCode())
	}
	t.Fatalf("holder serve guard process did not reach the bootstrap lock within timeout")
}

// TestServeManagementHostBootstrapHolder is the holder child body selector:
// with no child selector it is a no-op (kept in the default enumeration with zero
// skips). When selected it runs the production serve-path guard on the fresh
// bootstrap root, acquires and holds the bootstrap lock, signals readiness, then
// waits to be killed by the parent.
func TestServeManagementHostBootstrapHolder(t *testing.T) {
	if os.Getenv(serveManagementHostChildEnv) != "1" || os.Getenv(serveManagementHostModeEnv) != "holder" {
		return
	}
	host, herr := managementHostForServe(context.Background())
	if herr != nil {
		fmt.Fprintf(os.Stderr, "holder managementHostForServe error: %v\n", herr)
		os.Exit(serveManagementHostErrExit)
	}
	if host == nil {
		fmt.Fprintln(os.Stderr, "holder: no management host resolved for local installation target")
		os.Exit(serveManagementHostErrExit)
	}
	bootstrap, unlock, verr := host.Verify()
	if verr != nil {
		if unlock != nil {
			unlock()
		}
		fmt.Fprintf(os.Stderr, "holder Verify error: %v\n", verr)
		os.Exit(serveManagementHostErrExit)
	}
	if !bootstrap {
		// An attach (non-empty root) is not a bootstrap contention; release and
		// report so the parent can fail setup.
		if unlock != nil {
			unlock()
		}
		fmt.Fprintln(os.Stderr, "holder: fresh bootstrap root was not a bootstrap; setup error")
		os.Exit(serveManagementHostErrExit)
	}
	// Held the bootstrap lock (the production ownership guard): signal readiness
	// so the challenger contends only while the lock is genuinely held.
	if err := os.WriteFile(os.Getenv(serveManagementHostReadyEnv), []byte("locked"), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "holder ready write failed: %v\n", err)
		os.Exit(serveManagementHostErrExit)
	}
	// Hold the lock until the parent kills this process.
	select {}
}

// TestServeManagementHostBootstrapChallenger is the challenger child body
// selector: with no child selector it is a no-op. When selected it runs the same
// production serve-path guard and, while the holder holds the bootstrap lock on
// the same fresh root, must be refused with the bootstrap-owned error.
func TestServeManagementHostBootstrapChallenger(t *testing.T) {
	if os.Getenv(serveManagementHostChildEnv) != "1" || os.Getenv(serveManagementHostModeEnv) != "challenger" {
		return
	}
	host, herr := managementHostForServe(context.Background())
	if herr != nil {
		fmt.Fprintf(os.Stderr, "challenger managementHostForServe error: %v\n", herr)
		os.Exit(serveManagementHostErrExit)
	}
	if host == nil {
		fmt.Fprintln(os.Stderr, "challenger: no management host resolved for local installation target")
		os.Exit(serveManagementHostErrExit)
	}
	_, unlock, verr := host.Verify()
	if unlock != nil {
		unlock()
	}
	if verr == nil {
		fmt.Fprintln(os.Stderr, "challenger reached bind on a fresh root while a bootstrap owner held the lock")
		os.Exit(1)
	}
	// The production serve path must fail closed (never bind as a second writer)
	// while another process holds bootstrap ownership.
	fmt.Fprintf(os.Stderr, "challenger failed-closed on fresh root: %v\n", verr)
	os.Exit(serveManagementHostHolderExit)
}

var _ = application.Store{}
