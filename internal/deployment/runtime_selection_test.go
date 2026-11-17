package deployment

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Restored verbatim from origin/develop — preserved as-is (no rename, no rewrite).
func TestRuntimeSelectionDefaultsLegacyState(t *testing.T) {
	selection, err := RuntimeSelectionFromValues(map[string]string{
		"BASEHARBOR_ENVIRONMENT": "production",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != bhruntime.ProviderDocker {
		t.Fatalf("provider = %q, want %q", selection.Provider, bhruntime.ProviderDocker)
	}
	if selection.Profile != RuntimeProfileStandard {
		t.Fatalf("profile = %q, want %q", selection.Profile, RuntimeProfileStandard)
	}
}

// Restored verbatim from origin/develop — the four-environment invariant.
func TestRuntimeSelectionDoesNotInferFromEnvironmentName(t *testing.T) {
	for _, environment := range []string{"dev", "staging", "production", "enterprise"} {
		selection, err := RuntimeSelectionFromValues(map[string]string{
			"BASEHARBOR_ENVIRONMENT": environment,
		})
		if err != nil {
			t.Fatal(err)
		}
		if selection.Provider != bhruntime.ProviderDocker || selection.Profile != RuntimeProfileStandard {
			t.Fatalf("environment %q changed runtime selection: %#v", environment, selection)
		}
	}
}

// Restored verbatim from origin/develop — explicit ApplyRuntimeSelection round-trip.
func TestRuntimeSelectionRoundTripPreservesOtherState(t *testing.T) {
	values := map[string]string{
		"BASEHARBOR_HOSTNAME": "mail.example.test",
		"BASEHARBOR_TLS_MODE": "existing",
	}
	want := RuntimeSelection{
		Provider: bhruntime.ProviderDocker,
		Profile:  RuntimeProfileStandard,
	}
	if err := ApplyRuntimeSelection(values, want); err != nil {
		t.Fatal(err)
	}
	if values["BASEHARBOR_HOSTNAME"] != "mail.example.test" || values["BASEHARBOR_TLS_MODE"] != "existing" {
		t.Fatalf("unrelated deployment state changed: %#v", values)
	}
	got, err := RuntimeSelectionFromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("selection = %#v, want %#v", got, want)
	}
}

// Restored verbatim from origin/develop — fails closed on an unknown profile.
func TestRuntimeSelectionRejectsUnknownProfile(t *testing.T) {
	if _, err := RuntimeSelectionFromValues(map[string]string{
		RuntimeProfileEnvKey: "enterprise-ha",
	}); err == nil {
		t.Fatal("unknown runtime profile unexpectedly accepted")
	}
}

// Restored verbatim from origin/develop — accepts a named future provider.
func TestRuntimeSelectionAcceptsKnownFutureProvider(t *testing.T) {
	selection, err := RuntimeSelectionFromValues(map[string]string{
		RuntimeProviderEnvKey: "kubernetes",
		RuntimeProfileEnvKey:  "standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != bhruntime.ProviderKubernetes {
		t.Fatalf("provider = %q, want %q", selection.Provider, bhruntime.ProviderKubernetes)
	}
	if selection.Profile != RuntimeProfileStandard {
		t.Fatalf("profile = %q, want %q", selection.Profile, RuntimeProfileStandard)
	}
}

// ---- New assertions (added beside the restored develop coverage) ----

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

// ApplyRuntimeProviderState still materializes the standard profile when none was
// persisted, so protected deployment state is a complete provider+profile selection.
func TestRuntimeSelectionLegacyProviderStateDefaultsProfile(t *testing.T) {
	values := map[string]string{}
	if err := ApplyRuntimeProviderState(values, RuntimeProviderState{Provider: bhruntime.ProviderPodman}); err != nil {
		t.Fatalf("ApplyRuntimeProviderState: %v", err)
	}
	if values["BASEHARBOR_RUNTIME_PROFILE"] != string(RuntimeProfileStandard) {
		t.Fatalf("profile not defaulted: got %q", values["BASEHARBOR_RUNTIME_PROFILE"])
	}
	got, err := RuntimeSelectionFromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != RuntimeProfileStandard {
		t.Fatalf("round-trip profile = %q, want %q", got.Profile, RuntimeProfileStandard)
	}
}

// an explicit environment name combined with a named future provider still resolves
// to that provider — the environment name is inert, the explicit selection wins.
func TestRuntimeSelectionExplicitFutureProviderWinsOverEnvironmentName(t *testing.T) {
	for _, environment := range []string{"dev", "staging", "production", "enterprise"} {
		selection, err := RuntimeSelectionFromValues(map[string]string{
			RuntimeProviderEnvKey:    "kubernetes",
			RuntimeProfileEnvKey:     "standard",
			"BASEHARBOR_ENVIRONMENT": environment,
		})
		if err != nil {
			t.Fatalf("environment %q: %v", environment, err)
		}
		if selection.Provider != bhruntime.ProviderKubernetes {
			t.Fatalf("environment %q: provider = %q, want kubernetes", environment, selection.Provider)
		}
		if selection.Profile != RuntimeProfileStandard {
			t.Fatalf("environment %q: profile = %q, want standard", environment, selection.Profile)
		}
	}
}
