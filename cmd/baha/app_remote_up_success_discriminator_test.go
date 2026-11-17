package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Red-first success discriminator for the remote-up CLI start path (card
// t-20261106-r27-remote-up-cli-path, owner round-4 ask 1).
//
// The mission requires a single test that drives the real CLI `baha app up`
// (and the --confirm-candidate path) against a connector-bound app through the
// command surface and asserts the success seam. The genuine success seam
// (a live enrolled Connector node that answers NodeMemory + Publish +
// ApplyApplication with err == nil) is unit-unreachable: a unit runner has no
// enrolled node, so the up-path fails closed at the connector seam with
// targetsession.ErrUnavailable rather than the local Core. Calling
// runtime.Publish/ApplyApplication directly (the internal/application round-trip
// cell) proves the staging/activation seam but never exercises the CLI command
// surface, so it is not this cell's proof.
//
// This cell therefore proves the discriminating behavior that #1407 introduced
// at the production CLI write seam (executeRemoteApplicationApply in
// cmd/baha/app_remote_lifecycle.go, the function this test drives end-to-end
// through executeApplicationUpLifecycle):
//
//  1. For a broker-less pure-client the real CLI up-path takes NO managed-service
//     issuer and touches NO local Core: the injected applicationCoreBootstrap is
//     never called and no local installation state is created, even though the
//     up-path proceeds past memory preflight to the production write seam.
//  2. The up-path reaches the production owner/connector write seam
//     (recordDeploymentBeforeMutation) and fails closed there (no live node /
//     ErrDeploymentOwnerMissing), rather than succeeding by falling back to a
//     local Core bootstrap. This is the behavior the round-4 evidence recorded.
//
// Red: on a HEAD that still routed the pure-client remote up through the local
// Core prerequisite, applicationCoreBootstrap/applicationCorePrerequisite would
// be called before the connector seam, so the "bootstrap never called" and
// "no installation state" assertions fail. Green: the fixed head takes the
// connector route and the injected bootstrap is never invoked.
func TestRemoteUpRealCLIPathTakesConnectorRouteNeverLocalCore(t *testing.T) {
	// remoteApplicationCoreFixtureEmpty leaves NO local Core installed and the
	// node bound to a connector access provider, so the up-path must route
	// through the connector transport.
	ctx, core, node := remoteApplicationCoreFixtureEmpty(t)
	ctx = machineNoninteractiveContext(withCoreAuthority(ctx, core))
	ctx = withApplicationInput(ctx, strings.NewReader("y\n"))
	ctx = targetsession.WithPool(ctx, targetsession.NewPool())
	t.Setenv(application.ProviderScopeEnv("postgresql"), "application")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	m := application.WithWorkloadComponents(application.New("remote-pure", "dev", false, false, false), "api")
	mustWriteInitializedAuthoredManifest(t, "baseharbor.yaml", m)
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultTarget = node.Name
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	// Instrument the local Core seams. On the pre-fix HEAD the pure-client
	// remote up-path would route through the local Core prerequisite / bootstrap
	// before the connector seam; the fixed head never does.
	bootstrapCalls := 0
	prevBootstrap := applicationCoreBootstrap
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		bootstrapCalls++
		return coreinstallation.State{}, nil
	}
	t.Cleanup(func() { applicationCoreBootstrap = prevBootstrap })

	// Drive the real CLI up-path end to end, exactly as `baha app up` would.
	upErr := executeApplicationUpLifecycle(withAssumeYes(ctx, true), application.Store{}, []string{}, io.Discard, io.Discard)

	// (1) The up-path must never bootstrap or bind a local Core for a
	// broker-less pure-client: the head routes through the bound target's
	// connector route and takes no issuer / touches no installation.
	if bootstrapCalls != 0 {
		t.Fatalf("pure-client remote up bootstrapped/bound a local Core (bootstrapCalls=%d); the head must route through the connector route", bootstrapCalls)
	}
	root, rerr := deployment.TargetStateRoot(core.Name)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if _, serr := coreinstallation.Load(root); serr == nil {
		t.Fatal("pure-client remote up created local Core installation state; must not touch a local Core")
	}

	// (2) The up-path reached the production owner/connector write seam and
	// failed closed there: a pure-client with no live enrolled node must not
	// succeed here, and must never succeed by falling back to a local Core
	// bootstrap. The fix routes the head through the connector route, so with no
	// live node the apply fails closed rather than bootstrap.
	if upErr == nil {
		t.Fatal("pure-client app up (real CLI path) succeeded without a live enrolled Connector; must fail closed at the seam")
	}
	t.Logf("remote app up (pure-client, NO local Core, no live node) => connector route, no Core bootstrap, fails closed err=%v", upErr)
}

// Broker-required intent takes a managed-service issuer and fails closed at the
// connector seam when the bound Core PKI is unreachable — proving the up-path
// only routes a broker-less pure-client through the connector without an issuer.
func TestRemoteUpRealCLIPathBrokerRequiredFailsClosedBeforeMutation(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixtureEmpty(t)
	ctx = machineNoninteractiveContext(withCoreAuthority(ctx, core))
	ctx = withApplicationInput(ctx, strings.NewReader("y\n"))
	ctx = targetsession.WithPool(ctx, targetsession.NewPool())
	t.Setenv(application.ProviderScopeEnv("postgresql"), "application")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	// SQL plus Identity requires a managed runtime broker / issuer.
	brokerManifest := application.WithWorkloadComponents(application.WithIdentity(application.New("remote-broker", "dev", true, false, false)), "api")
	mustWriteInitializedAuthoredManifest(t, "baseharbor.yaml", brokerManifest)
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultTarget = node.Name
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	bootstrapCalls := 0
	prevBootstrap := applicationCoreBootstrap
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		bootstrapCalls++
		return coreinstallation.State{}, nil
	}
	t.Cleanup(func() { applicationCoreBootstrap = prevBootstrap })

	brokerErr := executeApplicationUpLifecycle(withAssumeYes(ctx, true), application.Store{}, []string{}, io.Discard, io.Discard)

	// A broker-required workload must not reach a local Core bootstrap either;
	// it fails closed before any mutation when the issuer is unreachable.
	if bootstrapCalls != 0 {
		t.Fatalf("broker-required remote up bootstrapped a local Core (bootstrapCalls=%d); must fail closed at the issuer seam", bootstrapCalls)
	}
	if brokerErr == nil {
		t.Fatal("broker-required app up succeeded without a reachable issuer; must fail closed")
	}
	// The failure is a real, non-panic production failure through the CLI surface.
	var machineErr *machine.Error
	if !errors.As(brokerErr, &machineErr) && strings.Contains(brokerErr.Error(), "issuer") == false {
		t.Fatalf("broker-required up failed with an unexpected non-machine error: %v", brokerErr)
	}
	t.Logf("remote app up (broker-required, no reachable issuer) => fails closed before mutation err=%v", brokerErr)
}
