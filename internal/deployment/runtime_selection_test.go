package deployment

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// An empty profile defaults to standard — a new deployment without an explicit
// profile still selects a concrete, safe runtime profile.
func TestParseRuntimeProfileDefaultsToStandard(t *testing.T) {
	got, err := ParseRuntimeProfile("")
	if err != nil {
		t.Fatalf("empty profile: %v", err)
	}
	if got != RuntimeProfileStandard {
		t.Fatalf("empty profile: want standard, got %q", got)
	}
}

// whitespace and case are normalized, not rejected.
func TestParseRuntimeProfileNormalizes(t *testing.T) {
	got, err := ParseRuntimeProfile("  STANDARD ")
	if err != nil {
		t.Fatalf("uppercase profile: %v", err)
	}
	if got != RuntimeProfileStandard {
		t.Fatalf("normalized profile: want standard, got %q", got)
	}
}

// an unsupported profile fails closed — no silent fall-through to an unknown
// runtime profile.
func TestParseRuntimeProfileRejectsUnknown(t *testing.T) {
	if _, err := ParseRuntimeProfile("ha-only"); err == nil {
		t.Fatal("unsupported runtime profile: expected a closed error, got nil")
	}
}

// an explicit podman provider selection is preserved through the deployment state.
func TestRuntimeSelectionFromValuesPodman(t *testing.T) {
	values := map[string]string{RuntimeProviderEnvKey: "podman", RuntimeProfileEnvKey: "standard"}
	selection, err := RuntimeSelectionFromValues(values)
	if err != nil {
		t.Fatalf("podman selection: %v", err)
	}
	if selection.Provider != bhruntime.ProviderPodman {
		t.Fatalf("podman selection: want podman, got %q", selection.Provider)
	}
}

// an unsupported provider in the deployment state fails closed. An invalid
// provider kind (illegal characters) is rejected, never coerced.
func TestRuntimeSelectionFromValuesRejectsUnknownProvider(t *testing.T) {
	values := map[string]string{RuntimeProviderEnvKey: "bogus!!provider"}
	if _, err := RuntimeSelectionFromValues(values); err == nil {
		t.Fatal("unknown provider: expected a closed error, got nil")
	}
}

// an unknown profile in deployment state fails closed.
func TestRuntimeSelectionFromValuesRejectsUnknownProfile(t *testing.T) {
	values := map[string]string{RuntimeProviderEnvKey: "docker", RuntimeProfileEnvKey: "ghost"}
	if _, err := RuntimeSelectionFromValues(values); err == nil {
		t.Fatal("unknown profile in deployment state: expected a closed error, got nil")
	}
}

// ApplyRuntimeSelection writes a normalized provider+profile and round-trips
// back through RuntimeSelectionFromValues.
func TestApplyRuntimeSelectionRoundTrip(t *testing.T) {
	values := map[string]string{}
	err := ApplyRuntimeSelection(values, RuntimeSelection{
		Provider: bhruntime.ProviderPodman,
		Profile:  RuntimeProfileStandard,
	})
	if err != nil {
		t.Fatalf("ApplyRuntimeSelection: %v", err)
	}

	got, err := RuntimeSelectionFromValues(values)
	if err != nil {
		t.Fatalf("round-trip read: %v", err)
	}
	if got.Provider != bhruntime.ProviderPodman {
		t.Fatalf("round-trip provider: want podman, got %q", got.Provider)
	}
	if got.Profile != RuntimeProfileStandard {
		t.Fatalf("round-trip profile: want standard, got %q", got.Profile)
	}
}

// ApplyRuntimeProviderState preserves unrelated deployment-owned state such as
// hostname while writing the provider.
func TestApplyRuntimeProviderStatePreservesUnrelated(t *testing.T) {
	values := map[string]string{"hostname": "node-1"}
	err := ApplyRuntimeProviderState(values, RuntimeProviderState{Provider: bhruntime.ProviderDocker})
	if err != nil {
		t.Fatalf("ApplyRuntimeProviderState: %v", err)
	}
	if values["hostname"] != "node-1" {
		t.Fatalf("unrelated state was overwritten: hostname=%q", values["hostname"])
	}
	if values[RuntimeProviderEnvKey] != "docker" {
		t.Fatalf("provider not written: %q", values[RuntimeProviderEnvKey])
	}
	if values[RuntimeProfileEnvKey] != string(RuntimeProfileStandard) {
		t.Fatalf("profile not defaulted: %q", values[RuntimeProfileEnvKey])
	}
}

// a nil target map fails closed — there is nowhere safe to write selection.
func TestApplyRuntimeProviderStateRejectsNil(t *testing.T) {
	if err := ApplyRuntimeProviderState(nil, RuntimeProviderState{Provider: bhruntime.ProviderDocker}); err == nil {
		t.Fatal("nil target: expected a closed error, got nil")
	}
}
