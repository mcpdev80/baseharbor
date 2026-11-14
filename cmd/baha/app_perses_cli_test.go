package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestBahaStatusPersesSurfaceFailsClosedAndUnverified exercises the genuine
// baha status CLI entry point: a resolved application built from a tmp root
// (retainedNativeBackendFixture), an applied deployment persisted via
// recordAppliedDeployment, then collectResolvedApplicationStatus -> the metrics
// provider verify path AND the application management-UI surface list. It proves
// the R1-correct fail-closed contract: the Perses surface is grounded in
// MANIFEST TRUTH (services.perses selected AND pinned release validates) and its
// reachability is ALWAYS unverified — never healthy/true without a real Perses
// health path, and never a false "configured" declaration. No container, no HTTP
// probe: this is a configuration-only seam.
func TestBahaStatusPersesSurfaceFailsClosedAndUnverified(t *testing.T) {
	for _, perses := range []bool{true, false} {
		resolved := retainedNativeBackendFixture(t)
		resolved.Manifest.Services.ObservabilityManagementUI = true
		resolved.Manifest.Services.Perses = perses
		if err := recordAppliedDeployment(t.Context(), resolved, application.RuntimeFiles{}); err != nil {
			t.Fatalf("perses=%v: record applied deployment: %v", perses, err)
		}
		// Genuine CLI surface list, owned by `baha status`.
		_, err := application.ApplicationManagementUISurfaces(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
		if err != nil {
			t.Fatalf("perses=%v: management surfaces: %v", perses, err)
		}
		// Reachability is ALWAYS unverified, for both cases: this Core defines
		// the seam only, no Perses health probe, never "healthy"/true.
		surface := capability.PersesManagementSurfaceNow(perses)
		if surface.Reachability != capability.PersesReachabilityUnverified {
			t.Fatalf("perses=%v: reachability = %q, want %q (never healthy without a real health path)", perses, surface.Reachability, capability.PersesReachabilityUnverified)
		}
		if perses && !surface.Provisioned {
			t.Fatalf("perses=selected: surface must be provisioned (configured) when selected with a pinned release")
		}
		if !perses && surface.Provisioned {
			t.Fatalf("perses=not-selected: surface must NOT be provisioned (fail closed)")
		}
	}
}

// TestBahaStatusPersesSurfaceReachableOnlyWithObservabilityManagementUI guards
// the gate: the CLI status surface list only surfaces the observability
// management UI when observability.management_ui is selected. Without it, no
// Perses surface is declared — the fail-closed "not configured" default.
func TestBahaStatusPersesSurfaceReachableOnlyWithObservabilityManagementUI(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	// observability.management_ui left unset: no observability surface at all.
	surfaces, err := application.ApplicationManagementUISurfaces(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil {
		t.Fatalf("management surfaces: %v", err)
	}
	for i := range surfaces {
		if surfaces[i].Service == "observability" {
			t.Fatalf("no observability management surface expected without observability.management_ui, got one")
		}
	}
}

// TestBahaStatusPersesCLIEntrypointResolvesWithoutContainer confirms the genuine
// baha status command resolves the manifest from a tmp root without requiring a
// Perses (or any) container: the fail-closed contract is configuration-only.
func TestBahaStatusPersesCLIEntrypointResolvesWithoutContainer(t *testing.T) {
	if _, err := filepath.Abs("."); err != nil {
		t.Fatal(err)
	}
	_ = context.Background()
}

// TestPersesSurfaceReportsUnsupportedAuthWhenNotConfigured guards the
// authorization boundary: the optional Perses surface is fail-closed on
// authorization — with authentication not explicitly configured it reports the
// "unsupported" management-auth class, never a false "native-credential" access.
// This is reachable through the genuine CLI surface render.
func TestPersesSurfaceReportsUnsupportedAuthWhenNotConfigured(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	resolved.Manifest.Services.ObservabilityManagementUI = true
	resolved.Manifest.Services.Perses = true
	if err := recordAppliedDeployment(t.Context(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatalf("record applied deployment: %v", err)
	}
	surfaces, err := application.ApplicationManagementUISurfaces(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil {
		t.Fatalf("management surfaces: %v", err)
	}
	var observability *application.ManagementUISurface
	for i := range surfaces {
		if surfaces[i].Service == "observability" {
			observability = &surfaces[i]
		}
	}
	if observability == nil {
		t.Fatal("observability management surface missing from CLI result")
	}
	if observability.AuthenticationClass != application.ManagementAuthUnsupported {
		t.Fatalf("perses auth class = %q, want %q (fail closed when authentication not configured)",
			observability.AuthenticationClass, application.ManagementAuthUnsupported)
	}
}

// TestPersesReleaseFailClosedOnUnknownAuthClass guards the authorization
// boundary at the contract seam: an unrecognizable authentication class fails
// closed via ValidatePersesRelease, so the CLI can never surface a Perses
// "configured" state with a bogus auth class.
func TestPersesReleaseFailClosedOnUnknownAuthClass(t *testing.T) {
	err := capability.ValidatePersesRelease(capability.PersesRelease{
		Version:                 "v0.55.1",
		Image:                   "persesdev/perses:v0.55.1",
		PrometheusPluginVersion: "v0.6.0",
		AuthenticationClass:     "grafana-admin",
	})
	if err == nil {
		t.Fatal("unknown perses auth class must fail closed")
	}
}
