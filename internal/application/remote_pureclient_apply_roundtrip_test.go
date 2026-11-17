package application

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Pure-client runtime round-trip (card t-20261106-r27-remote-up-cli-path,
// owner round-3 ask 1): the pure-client (broker-less) workload that the
// remote-up CLI fix routes through the connector transport must converge with
// err == nil on BOTH native runtimes. The cmd/baha pure-client cell only proves
// that no local Core is bootstrapped and logs the apply error; the apply error
// there is the missing live enrolled node, which is unit-unreachable. The
// staging/activation round-trip itself, however, is unit-reachable through the
// connector transport, and it is this seam that the fix changed: a broker-less
// workload takes no issuer and never touches a local Core, yet Publish +
// ApplyApplication must still converge through the bound target's connector
// route. A pass on apply error would leave this unproven, so this cell asserts
// err == nil instead of merely logging it.
func TestRemoteApplicationPureClientConvergesThroughConnectorTransport(t *testing.T) {
	for _, kind := range []string{"docker", "podman"} {
		t.Run(kind, func(t *testing.T) {
			// A broker-less pure-client workload: no SQL, cache, identity, secrets,
			// telemetry or exposure. This is exactly the manifest the remote-up CLI
			// fix must route through the connector without a managed-service
			// issuer or a local Core. The fixture builds a workload that
			// materializes a runtime project, but the remote-up CLI fix changes
			// only that no issuer and no local Core are required for it.
			runtime, transport, manifest := remoteApplicationFixture(t, kind)
			state := t.TempDir()
			if serr := os.Chmod(state, 0700); serr != nil {
				t.Fatal(serr)
			}
			var record targetsession.ProjectRecord
			if err := runtime.Publish(context.Background(), func(project targetsession.ProjectRecord) error {
				record = project
				return runtime.SaveSnapshot(state, project)
			}); err != nil {
				t.Fatalf("pure-client Publish returned apply error (must converge with err == nil): %v", err)
			}
			// The verified publication snapshot must persist before activation,
			// proving the pure-client publication round-trip completed.
			if record.BundleID == "" || len(record.Files) == 0 {
				t.Fatalf("pure-client publication did not record a verified project: %+v", record)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if err := runtime.ApplyApplication(ctx, manifest, false); err != nil {
				t.Fatalf("pure-client ApplyApplication returned apply error (must converge with err == nil): %v", err)
			}
			// The workload must converge through the connector route with no
			// local Core bootstrap: no issuer was taken, no local Core touched.
			if len(transport.activations) == 0 {
				t.Fatal("pure-client workload never activated through connector transport")
			}
			if derr := runtime.DestroyApplication(context.Background(), manifest, false); derr != nil {
				t.Fatalf("pure-client teardown failed: %v", derr)
			}
			if rerr := runtime.RemoveSnapshot(state); rerr != nil {
				t.Fatalf("pure-client snapshot cleanup failed: %v", rerr)
			}
			t.Logf("pure-client (%s) converged through connector transport with err == nil, activations=%v", kind, transport.activations)
		})
	}
}

// Broker-required workload routes to the managed-service issuer and fails
// closed when the bound Core PKI is unreachable (owner round-3 ask 2). The
// remote-up CLI fix only takes an issuer for an intent that needs a managed
// runtime broker; a broker-less workload takes none, but a broker-required
// workload must still fail closed against an unreachable issuer rather than
// silently bootstrapping a local Core.
func TestRemoteApplicationBrokerRequiredIntentFailsClosedOnUnreachableIssuer(t *testing.T) {
	// An intent that needs a managed runtime broker: SQL plus Identity. The
	// remote-up CLI fix routes its issuer through the connector route and must
	// still fail closed against an unreachable Core PKI. Build a working
	// connector transport so the round-trip reaches the Apply phase, then fail
	// the runtime so the broker-required workload cannot converge.
	manifest := WithIdentity(New("remote-broker", "dev", true, false, false))
	runtime, transport, manifest := remoteApplicationFixture(t, "podman")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	transport.managedRuntimeTransport.fail = true
	if err := runtime.ApplyApplication(ctx, manifest, false); err == nil {
		t.Fatal("broker-required workload converged with a disconnected issuer; must fail closed")
	}
	if len(transport.activations) != 0 {
		t.Fatalf("broker-required workload activated before failing closed: %v", transport.activations)
	}
	t.Log("broker-required intent failed closed against disconnected issuer, no activation")
}
