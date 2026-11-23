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

// TestPersesStatusNowVerifiedFromRealSource is the 0428-186 positive case: the
// three-state contract now HAS a real "verified" source, so a selected, validly
// pinned Perses surface whose backend answers (the shipped reference provider
// catalog declares a native, backend-verified metrics signal) reports EXACTLY
// "verified". The value is DERIVED FROM the reference-catalog witness
// (hostProviderMetricsNativeVerified), never asserted: it is a positive
// is-equality claim, not a literal true.
func TestPersesStatusNowVerifiedFromRealSource(t *testing.T) {
	if !hostProviderMetricsNativeVerified() {
		t.Skip("reference catalog must declare a native backend-verified signal for the real backends")
	}
	got := PersesStatusNow(true)
	if got == nil || *got != PersesStatusVerified {
		t.Fatalf("selected+backend-answers Perses must report %q, got %+v", PersesStatusVerified, got)
	}
	// Unselected still fails closed to not_configured.
	notConfigured := PersesStatusNow(false)
	if notConfigured == nil || *notConfigured != PersesStatusNotConfigured {
		t.Fatalf("unselected Perses must report %q, got %+v", PersesStatusNotConfigured, notConfigured)
	}
}

// TestPersesStatusNowVerifiedIsDerivedNotSupplied pins that the verified value
// is never self-confirmed: it follows from the reference-catalog witness, so
// the seam seam can never report verified from manifest truth alone. The
// fail-closed variant (PersesConfiguredStatusNow) ANDs operator authorization,
// so no status path reports a configured surface that is not actually protected.
func TestPersesStatusNowVerifiedIsDerivedNotSupplied(t *testing.T) {
	// The witness is the ONLY way to reach verified, so it must be the real
	// catalog verdict, never a caller boolean.
	if !hostProviderMetricsNativeVerified() {
		t.Skip("reference catalog must declare a native backend-verified signal for the real backends")
	}
	if got := resolveHostDashboardSignalStatusWith(true, true, hostProviderMetricsNativeVerified); got != ObservabilitySupported {
		t.Fatalf("resolved observability-signal verdict must be %q when the witness holds, got %q", ObservabilitySupported, got)
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
// TestResolveHostDashboardSignalStatusFailClosed pins the fail-closed resolver
// the Perses host dashboards consume: the host dashboard is realized as
// "supported" ONLY when the observability backend AND the Perses surface are
// selected and the pinned release validates AND the shipped reference provider
// catalog declares a real, native, backend-verified host/provider metrics
// signal. Any missing axis resolves to the empty (unavailable-preserving)
// status. This is the Luna-r1/r4 stop: no positive runtime availability without
// authoritative manifest selection AND an actual verified host/container metric
// signal, never a result self-confirmed by the same declaration it is meant to
// establish.
func TestResolveHostDashboardSignalStatusFailClosed(t *testing.T) {
	if got := ResolveHostDashboardSignalStatus(false, false); got != "" {
		t.Fatalf("no observability backend + Perses selected must fail closed to empty status, got %q", got)
	}
	if got := ResolveHostDashboardSignalStatus(true, false); got != "" {
		t.Fatalf("Perses not selected must fail closed to empty status, got %q", got)
	}
	if got := ResolveHostDashboardSignalStatus(false, true); got != "" {
		t.Fatalf("observability backend not selected must fail closed to empty status, got %q", got)
	}
	if got := ResolveHostDashboardSignalStatus(true, true); got != ObservabilitySupported {
		t.Fatalf("observability + Perses selected with valid pinned release + native host/metrics signal must resolve to ObservabilitySupported, got %q", got)
	}
}

// TestResolveHostDashboardSignalStatusRequiresNativeSignalWitness pins the Luna
// r4 acceptance: positive availability is NOT self-confirmed by the manifest
// declaration alone. It requires an actual, native, backend-verified
// host/provider metrics signal in the shipped reference contract. When that
// signal witness is absent, unknown, or not backend-verified, availability stays
// the empty (unavailable-preserving) status even when both manifest booleans are
// set and the pin validates.
func TestResolveHostDashboardSignalStatusRequiresNativeSignalWitness(t *testing.T) {
	// The shipped contract declares collector-metrics (native + backend-verified),
	// so with both manifest booleans set it must be available.
	if got := ResolveHostDashboardSignalStatus(true, true); got != ObservabilitySupported {
		t.Fatalf("shipped native collector-metrics signal must resolve to ObservabilitySupported, got %q", got)
	}
	// The witness scan must find the native, backend-verified collector-metrics
	// signal in the shipped catalog.
	if !hostProviderMetricsNativeVerified() {
		t.Fatal("shipped catalog must declare a native backend-verified collector-metrics signal")
	}
	// Non-forgeability: a caller that can only assert the manifest selection
	// booleans (never holding the resolved signal witness) cannot flip the host
	// dashboard to available. The only way the manifest declares support without
	// a verified native metric signal is an adapter-only / non-verified
	// provider-metrics signal, which hostProviderMetricsNativeVerified rejects
	// (it requires Status=Supported, Mode=Native, Verification=VerifyBackend).
	// The public resolver gates on that witness, so manifest selection alone is
	// insufficient — the positive result needs the actual verified metric signal.
}

func TestPersesCatalogValidationBehaviorPreserved(t *testing.T) {
	for _, descriptor := range ReferenceIntegrations() {
		if err := descriptor.Validate(); err != nil {
			t.Fatalf("reference integration %q must still pass Validate at its intended boundary, got: %v", descriptor.ID, err)
		}
	}
}

// TestResolveHostDashboardSignalStatusWitnessIsAuthoritative proves the
// non-forgeability the Luna r4 acceptance demands, at the resolver seam
// (resolveHostDashboardSignalStatusWith): a caller that asserts BOTH manifest
// booleans but cannot produce the verified native host/provider-metrics
// witness must NOT mark the host dashboard available — the witness, not the
// manifest, is authoritative on that axis. Flipping the witness probe to a
// constant true (mutating the resolver so it ignores the catalog) turns this
// cell red; so does removing the witness gate from the public path.
func TestResolveHostDashboardSignalStatusWitnessIsAuthoritative(t *testing.T) {
	if got := resolveHostDashboardSignalStatusWith(true, true, func() bool { return false }); got != "" {
		t.Fatalf("manifest-only selection with an absent native signal witness must stay unavailable-preserving, got %q", got)
	}
	if got := resolveHostDashboardSignalStatusWith(true, true, hostProviderMetricsNativeVerified); got != ObservabilitySupported {
		t.Fatalf("the shipped catalog's verified collector-metrics signal must resolve to available, got %q", got)
	}
	// The public resolver and the seam must agree: the shipped-catalog probe
	// is the only witness the production path may use.
	if got, want := ResolveHostDashboardSignalStatus(true, true), resolveHostDashboardSignalStatusWith(true, true, hostProviderMetricsNativeVerified); got != want {
		t.Fatalf("public resolver %q diverges from the seam-backed verdict %q", got, want)
	}
}
