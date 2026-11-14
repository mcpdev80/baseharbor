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

// TestPersesIsNotAnApplicationCapability asserts Perses has no portable
// application capability (it is a management surface, never an application
// capability) and its validation is fail-closed.
func TestPersesIsNotAnApplicationCapability(t *testing.T) {
	desc := capability.PersesManagement
	if len(desc.Capabilities) != 0 {
		t.Fatalf("perses must not declare application capabilities, got %d", len(desc.Capabilities))
	}
	if err := desc.Validate(); err == nil {
		t.Fatal("perses descriptor must fail validation when it has no versioned capability specifications")
	}
}
