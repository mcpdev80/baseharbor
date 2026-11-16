package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Red-first pure-client cell (card t-20261106-r27-remote-up-cli-path, option a):
// a workload-only app bound to a remote node target with NO runnable local Core
// staged. Before the fix the fresh-apply HEAD bootstraps/binds a local Core
// (requireApplicationCore -> coreRequired) and halts "BaseHarbor needs its Core
// services". After the fix the head routes through the connector route of the
// bound target and never touches a local Core: the injected installCore must
// never be called.
func TestRemoteFreshApplyPureClientNeverBootstrapsLocalCore(t *testing.T) {
	// remoteApplicationCoreFixtureEmpty leaves NO local Core installed.
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

	bootstrapCalls := 0
	prereqCalls := 0
	prevPrereq := applicationCorePrerequisite
	applicationCorePrerequisite = func(c context.Context, r io.Reader, w io.Writer) error {
		prereqCalls++
		return prevPrereq(c, r, w)
	}
	t.Cleanup(func() { applicationCorePrerequisite = prevPrereq })
	previous := applicationCoreBootstrap
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		bootstrapCalls++
		return coreinstallation.State{}, nil
	}
	t.Cleanup(func() { applicationCoreBootstrap = previous })

	err = executeApplicationUpLifecycle(withAssumeYes(ctx, true), application.Store{}, []string{}, io.Discard, io.Discard)
	// A workload-only pure-client must not route through the local Core
	// prerequisite at the fresh-apply production line and never through a
	// bootstrap: the head must route through the bound target's connector.
	if prereqCalls != 0 || bootstrapCalls != 0 {
		t.Fatalf("fresh remote apply routed through the local Core (prerequisite calls=%d, bootstrap calls=%d); the head must route through the bound target's connector route", prereqCalls, bootstrapCalls)
	}
	// The local Core installation state must remain absent.
	root, rerr := deployment.TargetStateRoot(core.Name)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if _, serr := coreinstallation.Load(root); serr == nil {
		t.Fatal("fresh remote apply created local Core installation state")
	}
	// Fail-closed at the connector seam: a pure-client with no live enrolled
	// node must not succeed here, and must never succeed by falling back to a
	// local Core. The fix routes the head through the bound target's connector
	// route, so with no live node the apply fails closed rather than bootstrap.
	if err == nil {
		t.Fatal("pure-client app up succeeded without a live enrolled Connector or local Core; must fail closed")
	}
	t.Logf("app up (pure-client, NO local Core, no live node) => no local Core bootstrap, fails closed err=%v", err)
}
