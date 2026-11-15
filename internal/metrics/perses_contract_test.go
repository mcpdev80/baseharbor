package metrics

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPersesReleaseIsPinnedAndCurrent asserts the v0.4.28 candidate pins a
// release whose security fixes are current (v0.55.0+), with a product version
// and production image. This is a declaration contract, not a running container.
func TestPersesReleaseIsPinnedAndCurrent(t *testing.T) {
	release := capability.PersesDefaultRelease
	if release.Version != "v0.55.1" {
		t.Fatalf("perses release version = %q, want a pinned v0.55.x (current security fixes)", release.Version)
	}
	if release.Image == "" {
		t.Fatal("perses release image must be pinned")
	}
	if err := capability.ValidatePersesRelease(release); err != nil {
		t.Fatalf("pinned perses release must validate: %v", err)
	}
}

// TestPersesManagementSurfaceFailClosedWhenUnprovisioned asserts that the
// optional Perses surface reports "not configured" -> unsupported rather than a
// false "configured" declaration when no release is pinned.
func TestPersesManagementSurfaceFailClosedWhenUnprovisioned(t *testing.T) {
	surface := capability.PersesManagementSurfaceUnprovisioned()
	if surface.Provisioned {
		t.Fatal("unprovisioned perses surface must not report provisioned=true")
	}
	if surface.AuthenticationClass != "unsupported" {
		t.Fatalf("unprovisioned perses surface auth class = %q, want unsupported", surface.AuthenticationClass)
	}
}

// TestPersesReleaseVersionIsRealizationMetadataNotProviderVersion asserts the
// Perses product version never leaks into the provider implementation version
// or a capability specification version (Provider Contract v1).
func TestPersesReleaseVersionIsRealizationMetadataNotProviderVersion(t *testing.T) {
	if capability.PersesDefaultRelease.Version == capability.PersesManagement.Version {
		t.Fatal("perses product version must not equal the provider implementation version; they are independent version axes")
	}
}

// TestPersesIsNotAnApplicationCapability asserts Perses is a management
// surface, never an application-capability provider. r2 moved the exported
// representation onto a valid IntegrationDescriptor that reuses the single
// observability capability owner (Metrics + the versioned MetricsV1
// specification), so the old "no versioned specifications => Validate must
// fail" expectation is obsolete. The intent is unchanged and now asserted in
// its positive form: at its intended boundary the descriptor validates, it
// binds exactly the observability owner (no fake metrics/logs/traces
// capabilities of its own), and it never appears in the shipped reference
// provider catalog.
func TestPersesIsNotAnApplicationCapability(t *testing.T) {
	desc := capability.PersesManagement
	// Intended boundary (r2): the management-surface descriptor is valid.
	if err := desc.Validate(); err != nil {
		t.Fatalf("perses management descriptor must validate at its intended boundary, got: %v", err)
	}
	// It reuses exactly the one observability owner and declares no application
	// capability of its own beyond that owner's versioned specification.
	if len(desc.Provider.Capabilities) != 1 || desc.Provider.Capabilities[0] != capability.Metrics {
		t.Fatalf("perses must bind exactly one capability kind (the observability owner Metrics), got %#v", desc.Provider.Capabilities)
	}
	if len(desc.Capabilities) != 1 || desc.Capabilities[0] != capability.MetricsV1.ID {
		t.Fatalf("perses must declare exactly the versioned MetricsV1 specification, got %#v", desc.Capabilities)
	}
	services, err := desc.EffectiveServices()
	if err != nil {
		t.Fatalf("EffectiveServices must not error for a valid descriptor: %v", err)
	}
	if len(services) != 1 || services[0] != capability.ServiceObservability {
		t.Fatalf("perses effective services = %v, want exactly [observability]: perses is a management surface, never an application capability", services)
	}
	// Fail-closed against false catalog membership: a management surface is
	// never a shipped application-capability provider.
	for _, descriptor := range capability.ReferenceIntegrations() {
		if descriptor.Provider.Kind == capability.PersesProviderKind {
			t.Fatalf("perses management surface must not appear in the reference provider catalog (%s)", descriptor.ID)
		}
	}
}
