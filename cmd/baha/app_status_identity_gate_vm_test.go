package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// dataDirForTest returns the runtime data dir the registry read uses.
func dataDirForTest() (string, error) {
	return bhruntime.DataDir("")
}

// dataDirForTest returns the runtime data dir the registry read uses.
func dataDirForTest() (string, error) {
	return bhruntimeDataDir()
}

// identityGateVmIntent is the authored intent that selects a managed Keycloak
// identity provider with a public OAuth exposure so the identity provider is
// actually realized in the runtime.
const identityGateVmIntent = `version: 1
app:
  name: identity-gate
  environment: dev
workload:
  components:
    - api
services:
  identity: true
identity:
  callback_paths:
    - /oauth/callback
exposure:
  http:
    - name: oauth
      service: api
      port: 8080
      protocol: https
      visibility: public
`

// realKeycloakNativeMetric reports whether the registered signals include a
// real Keycloak native openmetrics signal (the evidence the gate requires):
// provider keycloak, kind metrics, id keycloak-metrics, native mode, backend
// verification.
func realKeycloakNativeMetric(sources []observability.SignalSource) bool {
	for _, s := range sources {
		if s.Provider != capability.ProviderKeycloak || s.Kind != observability.SignalMetrics {
			continue
		}
		if s.ID == "keycloak-metrics" &&
			s.Mode == capability.ObservabilityNative &&
			s.Verification == capability.ObservabilityVerifyBackend {
			return true
		}
	}
	return false
}

// TestStatusResultIdentityGateVM drives the production call-site gate
// (collectApplicationStatusResult) end to end on the exact-SHA Docker runtime:
// it deploys a managed Keycloak identity provider, then asserts that the
// non-forgeable identity_dashboard_eligible field is derived from a REAL
// Keycloak native metrics signal in the observability signal registry plus a
// valid, BaseHarbor-owned Target authorization (fail-closed). This is a smoke
// test that runs on the named SSH test host (baha-docker) and proves the
// positive path that no unit test can reach without a deployed registry signal.
func TestStatusResultIdentityGateVM(t *testing.T) {
	if os.Getenv("BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker Identity acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 20*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "keycloak")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	// Managed Keycloak requires a Core installation to govern the Target, so
	// this smoke test does NOT assert a coreless host. The provider is used
	// by the runtime apply below.
	_ = provider
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("baseharbor.yaml", []byte(identityGateVmIntent), 0600); err != nil {
		t.Fatal(err)
	}
	var initOut bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes"}, &initOut, &initOut); err != nil {
		t.Fatalf("initialize managed-Identity workload: %v", err)
	}
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatalf("apply managed-Identity: %v", err)
	}
	placement, keycloakErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderKeycloak)
	if keycloakErr != nil {
		t.Fatalf("resolve Keycloak placement: %v", keycloakErr)
	}
	sources, listErr := observability.List(
		observability.SignalMetrics,
		placement,
		[]string{resolved.Manifest.Name},
		true,
		true,
	)
	if listErr != nil {
		t.Fatalf("list observability signals: %v", listErr)
	}
	if !realKeycloakNativeMetric(sources) {
		// Diagnostic: read the raw registry file to see what is actually
		// registered for this Target.
		var raw string
		if dir, dErr := dataDirForTest(); dErr == nil {
			if b, rErr := os.ReadFile(filepath.Join(dir, "observability", "signals.json")); rErr == nil {
				raw = string(b)
			}
		}
		t.Fatalf("no real keycloak-metrics native signal in registry; ALL signals (0 in memory): %v; RAW signals.json=%q", sources, raw)
	}
	result, resErr := collectApplicationStatusResult(ctx, resolved.Store, []string{"status"})
	if resErr != nil {
		t.Fatalf("collectApplicationStatusResult: %v", resErr)
	}
	if !result.IdentityDashboardEligible {
		t.Fatalf("identity_dashboard_eligible = false with a real Keycloak signal + authorized Target; the gate must not fail closed on real evidence: %+v", result)
	}
	t.Logf("identity gate positive: identity_dashboard_eligible=true with real keycloak-metrics signal + authorized Target")
}

// TestStatusResultIdentityGateFailClosedWithoutRealSignal pins the negative
// path: with NO real Keycloak native metrics signal in the observability
// registry the production call-site gate fails closed to
// identity_dashboard_eligible == false, even when the identity provider is
// selected and the placement is valid. This cell applies no managed Keycloak
// container, so no real signal is registered; the gate must report unavailable.
func TestStatusResultIdentityGateFailClosedWithoutRealSignal(t *testing.T) {
	ctx := context.Background()
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "keycloak")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("baseharbor.yaml", []byte(identityGateVmIntent), 0600); err != nil {
		t.Fatal(err)
	}
	var initOutNeg bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes"}, &initOutNeg, &initOutNeg); err != nil {
		t.Fatalf("initialize managed-Identity workload: %v", err)
	}
	// No apply: no managed Keycloak container was realized, so no real signal
	// is registered. The gate must fail closed. A "no registered deployment"
	// error is the same fail-closed default (nothing applied).
	result, resErr := collectApplicationStatusResult(ctx, application.Store{}, []string{"status"})
	if resErr != nil {
		lower := strings.ToLower(resErr.Error())
		if !strings.Contains(lower, "file") && !strings.Contains(lower, "exist") && !strings.Contains(lower, "no registered deployment") {
			t.Fatalf("collectApplicationStatusResult without a real signal: %v", resErr)
		}
	}
	if result.IdentityDashboardEligible {
		t.Fatalf("identity_dashboard_eligible must be false without a real Keycloak metric signal (fail-closed)")
	}
	t.Logf("identity gate fail-closed: identity_dashboard_eligible=false with no real signal")
}
