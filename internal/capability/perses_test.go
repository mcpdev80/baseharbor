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
	// Perses declares no metrics/logs/traces capability it IMPLEMENTS on its own:
	// it is an optional observability management/visualization surface, never an
	// application capability provider. It reuses the single observability owner
	// (the Metrics capability kind that Prometheus ships), expressed via the
	// versioned MetricsV1 specification, not as a fake implementation.
	if len(PersesManagement.Provider.Capabilities) != 1 {
		t.Fatalf("perses must bind exactly one capability kind (the observability owner Metrics), got %#v", PersesManagement.Provider.Capabilities)
	}
	for _, spec := range PersesManagement.Capabilities {
		if spec != MetricsV1.ID {
			t.Fatalf("perses must expose only the versioned MetricsV1 specification, got %q", spec)
		}
	}
	// The product version must be a distinct version axis from the provider
	// implementation version (Provider Contract v1: versioning axes).
	if PersesManagement.Version == "" {
		t.Fatal("perses provider implementation version must be declared independently")
	}
	if PersesDefaultRelease.Version == PersesManagement.Version {
		t.Fatal("perses product version must be independent from the provider implementation version")
	}
}

// r2: the exported Perses representation carries a valid, explicit
// catalog/registry status at its intended boundary. It binds the single
// observability capability owner (Metrics + its versioned MetricsV1
// specification) that the pinned Perses release reuses, so IntegrationDescriptor
// Validate now accepts it instead of rejecting the missing capability kinds.
func TestPersesManagementIsAValidIntegrationDescriptor(t *testing.T) {
	if err := PersesManagement.Validate(); err != nil {
		t.Fatalf("perses management descriptor must pass IntegrationDescriptor.Validate after r2, got: %v", err)
	}

	// Exactly one capability kind, bound to the observability owner.
	if len(PersesManagement.Provider.Capabilities) != 1 || PersesManagement.Provider.Capabilities[0] != Metrics {
		t.Fatalf("perses must bind exactly one observability capability kind (Metrics), got %#v", PersesManagement.Provider.Capabilities)
	}
	if len(PersesManagement.Capabilities) != 1 || PersesManagement.Capabilities[0] != MetricsV1.ID {
		t.Fatalf("perses must declare exactly the versioned MetricsV1 specification, got %#v", PersesManagement.Capabilities)
	}

	// The capability derivation yields the single observability service only;
	// no metrics/logs/traces application capability is added.
	services, err := PersesManagement.EffectiveServices()
	if err != nil {
		t.Fatalf("EffectiveServices must not error for a valid descriptor: %v", err)
	}
	if len(services) != 1 || services[0] != ServiceObservability {
		t.Fatalf("perses effective services = %v, want exactly [observability]", services)
	}
}

// r2: the valid Perses descriptor must never be (mis)registered as an
// application capability provider. It is an optional observability
// management surface, not a provider in the shipped reference catalog.
func TestPersesManagementNotRegisteredAsCapabilityProvider(t *testing.T) {
	for _, descriptor := range ReferenceIntegrations() {
		if descriptor.Provider.Kind == PersesProviderKind {
			t.Fatalf("perses management surface must not appear in the reference provider catalog (%s)", descriptor.ID)
		}
	}
}
