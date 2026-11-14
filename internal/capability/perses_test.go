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
	if !PersesManagementSurfaceNow().Provisioned {
		t.Fatal("default release surface must report provisioned=true")
	}
}

func TestPersesIsManagementSurfaceNotApplicationCapability(t *testing.T) {
	if PersesProviderKind == "" {
		t.Fatal("perses provider kind must be declared")
	}
	if len(PersesManagement.Capabilities) != 0 {
		t.Fatalf("perses must not declare application capabilities, got %d", len(PersesManagement.Capabilities))
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
