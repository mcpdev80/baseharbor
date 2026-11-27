package metrics

import (
	"reflect"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
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

// TestTempoTraceQueryTimeoutReadsFastServiceMatrix asserts the Tempo trace-query
// probe reads the shared application StatusTimeoutMatrix FastServiceTimeout
// group (10s) rather than a private duplicated literal (H-V42, 0428-315). The
// value is unchanged; this is a consolidation assertion, not a tuning change.
func TestTempoTraceQueryTimeoutReadsFastServiceMatrix(t *testing.T) {
	if tempoTraceQueryTimeout != application.FastServiceTimeout {
		t.Fatalf("tempo trace-query timeout = %s, want matrix FastServiceTimeout %s (single source of truth)", tempoTraceQueryTimeout, application.FastServiceTimeout)
	}
	if tempoTraceQueryTimeout != 10*time.Second {
		t.Fatalf("tempo trace-query timeout = %s, want 10s (value preserved through consolidation)", tempoTraceQueryTimeout)
	}
}

// TestTempoTraceQueryTimeoutIsCompileTimeConstant is the static companion of
// the runtime assertion above: it references the package-level compile-time
// guards. The drift-guard type only exists while both the alias and the matrix
// group are compile-time constants with the SAME value (array lengths must be
// constant expressions, and a negative length fails the build), and the
// constant-array guard only exists while the alias is a constant. Turning the
// alias back into a mutable var, or drifting it away from the matrix, breaks
// compilation — this is no longer a point-in-time value assertion (Codex
// re-review correction, 0428-315).
func TestTempoTraceQueryTimeoutIsCompileTimeConstant(t *testing.T) {
	// The guard function returns one zero-sized type per drift direction:
	// both exist (size 0) only while the alias is a compile-time constant
	// equal to the matrix group value. A mutated or drifted alias means this
	// file does not compile at all.
	returns := reflect.TypeOf(assertTempoAliasMatrixEquality)
	for i := 0; i < returns.NumOut(); i++ {
		if size := returns.Out(i).Size(); size != 0 {
			t.Fatalf("drift-guard type %d size = %d, want 0 (alias must equal the matrix constant at compile time)", i, size)
		}
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
// only the real observability owner (Prometheus) as an owner reference. The
// intent is unchanged and now asserted in its positive form: the contract
// carries the observability management-ui interface and the real owner, has no
// fake capability claim, and never appears in the shipped reference provider
// catalog.
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
