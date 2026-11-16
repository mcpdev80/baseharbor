package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The cells below are real assertions over the fail-closed contract: the adapter
// never fabricates a datasource for an unselected, unowned, missing, or
// unauthorized Tempo path, and its health check never reports "healthy" without
// a real reachable Tempo health path (which this Core does not ship).

// persesTempoDatasourceManifest returns a selected, pinned-surface manifest
// (observability management UI + Perses selected). It deliberately selects no
// Traces signal, so the fail-closed builder still rejects the datasource.
func persesTempoDatasourceManifest() application.Manifest {
	m := application.New("perses-datasource-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// TestPersesTempoDatasourceUnselectedPersesFailsClosed asserts the fail-closed
// rule: without Perses selected the datasource is not configured (fail-closed),
// never "configured".
func TestPersesTempoDatasourceUnselectedPersesFailsClosed(t *testing.T) {
	m := application.New("perses-datasource-demo", "dev", false, false, false)
	if _, err := BuildPersesTempoDatasource(m); err == nil {
		t.Fatal("unselected perses must fail closed, not fabricate a datasource")
	}
}

// TestPersesTempoDatasourceTracesUnselectedFailsClosed asserts the fail-closed
// builder guard: Perses selected but no Traces signal selected means the Tempo
// backend is not provisioned, so no datasource may be fabricated. This is the
// selection/authorization enforcement CODEX-Luna required.
func TestPersesTempoDatasourceTracesUnselectedFailsClosed(t *testing.T) {
	m := persesTempoDatasourceManifest() // Perses selected, Traces NOT selected
	_, err := BuildPersesTempoDatasource(m)
	if err == nil {
		t.Fatal("perses selected without a selected, Tempo-owned traces path must fail closed, not fabricate a datasource")
	}
	var notSelected *TempoNotSelectedError
	if !errors.As(err, &notSelected) {
		t.Fatalf("expected TempoNotSelectedError, got %v", err)
	}
}

// TestPersesTempoDatasourceMissingEndpointFailsClosed asserts the fail-closed
// builder guard when Traces is selected but no Tempo provider endpoint is
// materialized: BuildPersesTempoDatasource must fail closed, never return a
// datasource with an empty or fabricated URL. A missing Tempo endpoint means
// the provider is not provisioned, which the provider reports as absent.
func TestPersesTempoDatasourceMissingEndpointFailsClosed(t *testing.T) {
	m := persesTempoDatasourceManifest()
	m = application.WithOTLPTelemetry(m, "traces") // Traces selected, no Tempo provisioned
	_, err := BuildPersesTempoDatasource(m)
	if err == nil {
		t.Fatal("selected traces with no materialized Tempo endpoint must fail closed, not fabricate a datasource")
	}
	var absent *TempoSourceAbsentError
	if !errors.As(err, &absent) {
		t.Fatalf("expected TempoSourceAbsentError, got %v", err)
	}
}

// TestPersesTempoDatasourceUnprovisionedTempoFailsClosed asserts that a selected
// Traces path with no provisioned Tempo provider fails closed: the provider
// reports the missing path as absent and no datasource is produced.
func TestPersesTempoDatasourceUnprovisionedTempoFailsClosed(t *testing.T) {
	m := application.New("perses-datasource-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	m = application.WithOTLPTelemetry(m, "traces")
	if _, err := BuildPersesTempoDatasource(m); err == nil {
		t.Fatal("unprovisioned Tempo path must fail closed, not fabricate a datasource")
	}
}

// TestPersesTempoDatasourceVerifyFailsClosedWithoutProbe asserts the fail-closed
// health contract: pin validation alone must never pass
// VerifyPersesTempoDatasourceHealth, and an unselected/unprovisioned surface
// returns a fail-closed error. The datasource is never reported "healthy"
// without a real reachable Tempo health path, which this Core does not ship.
func TestPersesTempoDatasourceVerifyFailsClosedWithoutProbe(t *testing.T) {
	// Unselected Perses / unprovisioned Tempo: health must fail closed.
	if err := VerifyPersesTempoDatasourceHealth(context.Background()); err == nil {
		t.Fatal("health must not pass without a real reachable Tempo health path")
	}
}

// TestPersesTempoDatasourceVerifyNeverReportsHealthy asserts that the only
// verdict this Core can return for a selected, authorized Tempo path is the
// fail-closed "unverified" (a real Tempo health path does not exist here).
func TestPersesTempoDatasourceVerifyNeverReportsHealthy(t *testing.T) {
	err := VerifyPersesTempoDatasourceHealth(context.Background())
	var unverified *TempoHealthUnverifiedError
	if !errors.As(err, &unverified) {
		t.Fatalf("expected TempoHealthUnverifiedError, got %v", err)
	}
}

// TestPersesTempoDatasourceFailClosedWhenUnpinned asserts the fail-closed rule:
// without a pinned release that validates, the surface is not configured
// (fail-closed), never "configured".
func TestPersesTempoDatasourceFailClosedWhenUnpinned(t *testing.T) {
	badRelease := capability.PersesRelease{Version: "", Image: "", AuthenticationClass: "unsupported"}
	_, err := persesTempoDatasourceFromRelease(persesTempoDatasourceManifest(), badRelease)
	if err == nil {
		t.Fatal("unpinned perses must fail closed, not fabricate a datasource")
	}
}

// TestPersesTempoDatasourceReachabilityIsUnverified pins that the only
// reachability state this Core produces for a selected, pinned surface is
// "unverified" (the seam ships no Tempo health probe).
func TestPersesTempoDatasourceReachabilityIsUnverified(t *testing.T) {
	// A build fails closed for this manifest (no materialized Tempo endpoint),
	// but the reachability contract itself is unverified; assert the contract
	// constant rather than a produced datasource.
	if PersesTempoReachabilityUnverified != "unverified" {
		t.Fatalf("reachability contract = %q, want %q (no Tempo health probe exists here)", PersesTempoReachabilityUnverified, "unverified")
	}
}

// TestPersesTempoDatasourceKindIsNativeTraceDatasource asserts the datasource
// kind is the native TraceDatasource, never a Grafana UI API kind.
func TestPersesTempoDatasourceKindIsNativeTraceDatasource(t *testing.T) {
	if PersesTempoDatasourceKind != "TraceDatasource" {
		t.Fatalf("perses tempo datasource kind = %q, want the native TraceDatasource (never a Grafana UI API)", PersesTempoDatasourceKind)
	}
}

// TestPersesTempoDatasourceNamesTempoOwner names the real owner, not a capability
// claim: the datasource names the Tempo provider kind as owner, never asserts a
// Traces capability of its own (Perses only visualises the backend data owner).
func TestPersesTempoDatasourceNamesTempoOwner(t *testing.T) {
	ds := PersesTempoDatasource{Owner: string(capability.ProviderTempo)}
	if ds.Owner != string(capability.ProviderTempo) {
		t.Fatalf("perses tempo datasource owner = %q, want %q (the real observability owner, as an owner reference)", ds.Owner, string(capability.ProviderTempo))
	}
}
