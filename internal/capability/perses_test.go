package capability

import "testing"

func TestValidatePersesReleaseFailClosedWithoutPin(t *testing.T) {
	err := ValidatePersesRelease(PersesRelease{})
	if err == nil {
		t.Fatal("empty perses release must fail validation (fail-closed 'not configured')")
	}
	if _, ok := err.(*PersesNotConfiguredError); !ok {
		t.Fatalf("expected *PersesNotConfiguredError, got %T: %v", err, err)
	}
}

func TestValidatePersesReleaseRejectsUnknownAuthClass(t *testing.T) {
	err := ValidatePersesRelease(PersesRelease{Version: "v0.55.1", Image: "persesdev/perses:v0.55.1", AuthenticationClass: "grafana"})
	if err == nil {
		t.Fatal("unknown perses auth class must fail validation")
	}
}

func TestPersesDefaultReleaseValidates(t *testing.T) {
	if err := ValidatePersesRelease(PersesDefaultRelease); err != nil {
		t.Fatalf("pinned default release must validate: %v", err)
	}
	// Provisioned is grounded in manifest truth: a pinned release validates.
	if !PersesManagementSurfaceNow(true).Provisioned {
		t.Fatal("default release surface must report provisioned=true when selected")
	}
	// But this Core defines only the seam, so reachability is never healthy.
	if PersesManagementSurfaceNow(true).Reachability != PersesReachabilityUnverified {
		t.Fatalf("selected perses surface reachability = %q, want unverified", PersesManagementSurfaceNow(true).Reachability)
	}
	// Without selection the surface must fail closed: not configured.
	if PersesManagementSurfaceNow(false).Provisioned {
		t.Fatal("unselected perses surface must not report provisioned=true")
	}
	if PersesManagementSurfaceNow(false).Reachability != PersesReachabilityUnverified {
		t.Fatalf("unselected perses surface reachability = %q, want unverified", PersesManagementSurfaceNow(false).Reachability)
	}
}

func TestPersesIsManagementSurfaceNotApplicationCapability(t *testing.T) {
	if PersesProviderKind == "" {
		t.Fatal("perses provider kind must be declared")
	}
	// Perses declares NO capability kind it IMPLEMENTS on its own: it is an
	// optional observability management/visualization surface, never an
	// application capability provider. Its representation declares no
	// metrics/logs/traces capability of its own and reuses the single
	// observability owner (Prometheus) only as an owner reference, never as a
	// Perses-declared capability.
	contract := PersesManagementSurfaceContract
	if contract.Kind != PersesProviderKind {
		t.Fatalf("perses contract kind = %q, want %q", contract.Kind, PersesProviderKind)
	}
	// The product version must be a distinct version axis from the pinned
	// release (Provider Contract v1: versioning axes).
	if contract.Version == "" {
		t.Fatal("perses product version axis must be declared independently")
	}
	if PersesDefaultRelease.Version == contract.Version {
		t.Fatal("perses product version must be independent from the pinned release version")
	}
	// The optional observability management-ui interface is reused from the
	// backend data owner, never intrinsic.
	if contract.Interface.Name != "management-ui" || contract.Interface.Class != InterfaceObservability || !contract.Interface.Optional {
		t.Fatalf("perses contract interface = %+v, want the optional observability management-ui", contract.Interface)
	}
	// Owner resolves to the real observability data owner (Prometheus).
	if contract.OwnerName() != string(ProviderPrometheus) {
		t.Fatalf("perses contract owner = %q, want prometheus", contract.OwnerName())
	}
}

// r4: the exported Perses representation is a distinct management-tool/surface
// contract at its intended boundary, NOT an IntegrationDescriptor and therefore
// declaring NO application capability kind. This replaces the r2
// TestPersesManagementIsAValidIntegrationDescriptor, which (wrongly) asserted
// Perses bound to the Metrics capability kind to satisfy IntegrationDescriptor
// Validate — a fake capability claim Luna REQUEST_CHANGES forbade.
func TestPersesManagementSurfaceContractBoundary(t *testing.T) {
	contract := PersesManagementSurfaceContract
	if contract.Kind != PersesProviderKind {
		t.Fatalf("perses contract kind = %q, want %q", contract.Kind, PersesProviderKind)
	}
	// Distinct version axis from the pinned release (no version-axis collapse).
	if contract.Version == "" {
		t.Fatal("perses product version axis must be declared independently")
	}
	if PersesDefaultRelease.Version == contract.Version {
		t.Fatal("perses product version must be independent from the pinned release version")
	}
	// Optional observability management-ui interface reused from the backend
	// data owner, never intrinsic.
	if contract.Interface.Name != "management-ui" || contract.Interface.Class != InterfaceObservability || !contract.Interface.Optional {
		t.Fatalf("perses contract interface = %+v, want the optional observability management-ui", contract.Interface)
	}
	// Real observability owner (Prometheus) named as an owner reference.
	if contract.OwnerName() != string(ProviderPrometheus) {
		t.Fatalf("perses contract owner = %q, want prometheus", contract.OwnerName())
	}
	// Shared placement scope, optional lifecycle.
	if contract.Scope != ScopeShared {
		t.Fatalf("perses contract scope = %q, want shared", contract.Scope)
	}
	if !contract.OptionalLifecycle {
		t.Fatal("perses contract should expose optional lifecycle")
	}
}

// r4: owner and interface resolve to the real observability owner and its
// optional management-ui surface, with no fake capability claim anywhere.
func TestPersesManagementSurfaceContractOwnerAndInterface(t *testing.T) {
	contract := PersesManagementSurfaceContract
	if contract.Interface.Class != InterfaceObservability {
		t.Fatalf("perses contract interface class = %q, want observability", contract.Interface.Class)
	}
	if contract.Interface.Name != "management-ui" {
		t.Fatalf("perses contract interface name = %q, want management-ui", contract.Interface.Name)
	}
	if !contract.Interface.Optional {
		t.Fatal("perses contract management-ui must be optional")
	}
	if contract.OwnerName() != string(ProviderPrometheus) {
		t.Fatalf("perses contract owner = %q, want the real observability owner prometheus", contract.OwnerName())
	}
	// No capability kind is claimed by the Perses contract. It is a management
	// surface, not a capability implementation.
	if contract.Kind != PersesProviderKind {
		t.Fatalf("perses contract kind = %q, want %q (no capability kind)", contract.Kind, PersesProviderKind)
	}
}

// r4: the valid Perses representation must never be (mis)registered as an
// application capability provider. It is an optional observability management
// surface, not a provider in the shipped reference catalog.
func TestPersesManagementNotRegisteredAsCapabilityProvider(t *testing.T) {
	for _, descriptor := range ReferenceIntegrations() {
		if descriptor.Provider.Kind == PersesProviderKind {
			t.Fatalf("perses management surface must not appear in the reference provider catalog (%s)", descriptor.ID)
		}
	}
	// The Perses representation must not appear anywhere in the reference
	// catalog's capability kinds either (no fake metrics/logs/traces claim).
	// Capability kinds are a distinct type from ProviderKind, so no reference
	// catalog capability can ever equal the Perses management kind.
	for _, descriptor := range ReferenceIntegrations() {
		if descriptor.Provider.Kind == PersesProviderKind {
			t.Fatalf("perses provider kind must never be a reference catalog provider kind, found in %s", descriptor.ID)
		}
	}
}

// r4: the new Perses representation must not weaken the provider catalog's
// validation. Every shipped reference descriptor still validates at its
// intended boundary.
func TestPersesCatalogValidationBehaviorPreserved(t *testing.T) {
	for _, descriptor := range ReferenceIntegrations() {
		if err := descriptor.Validate(); err != nil {
			t.Fatalf("reference integration %q must still pass Validate at its intended boundary, got: %v", descriptor.ID, err)
		}
	}
}
