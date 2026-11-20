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
	if capability.PersesDefaultRelease.Version == capability.PersesManagementSurfaceContract.Version {
		t.Fatal("perses product version must not equal the provider implementation version; they are independent version axes")
	}
}

// TestPersesIsNotAnApplicationCapability asserts Perses is a management
// surface, never an application-capability provider. r2 (wrongly) moved the
// exported representation onto a valid IntegrationDescriptor that reused the
// single observability capability owner (Metrics + the versioned MetricsV1
// specification); Luna REQUEST_CHANGES rejected that because it supplied a fake
// Metrics capability claim. r4 replaced the representation with a distinct
// management-tool/surface contract that declares NO capability kind and names
// only the real observability owner (Prometheus) as an owner reference: an
// IntegrationDescriptor would falsely bind Perses to a Metrics/Logs/Traces
// capability, because Perses only visualises the surface the backend data owner
// already ships. The intent is unchanged and asserted here in its positive
// form: the contract carries the observability management-ui interface and the
// real owner, has no fake capability claim, and never appears in the shipped
// reference provider catalog.
func TestPersesIsNotAnApplicationCapability(t *testing.T) {
	contract := capability.PersesManagementSurfaceContract
	// It declares no capability kind at all: it is a management surface, never
	// an application-capability provider.
	if contract.Kind != capability.PersesProviderKind {
		t.Fatalf("perses contract kind = %q, want %q (no capability claim)", contract.Kind, capability.PersesProviderKind)
	}
	// The observability management-ui interface is reused from the backend data
	// owner, and the real owner (Prometheus) is named as an owner reference.
	if contract.Interface.Class != capability.InterfaceObservability || contract.Interface.Name != "management-ui" {
		t.Fatalf("perses contract interface = %+v, want the observability management-ui", contract.Interface)
	}
	if contract.OwnerName() != string(capability.ProviderPrometheus) {
		t.Fatalf("perses contract owner = %q, want prometheus", contract.OwnerName())
	}
	// Fail-closed against false catalog membership: a management surface is
	// never a shipped application-capability provider.
	for _, descriptor := range capability.ReferenceIntegrations() {
		if descriptor.Provider.Kind == capability.PersesProviderKind {
			t.Fatalf("perses management surface must not appear in the reference provider catalog (%s)", descriptor.ID)
		}
	}
}
