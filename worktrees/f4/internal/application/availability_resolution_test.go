package application

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestResolveAvailabilityFailsBeforeMutationForUnsupportedRuntimeHA(t *testing.T) {
	m := New("demo", "prod", false, false, false)
	m.Services.SQL = false
	m.ApplicationID = "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"
	m.Workload.Components = []string{"api"}
	m.HA = true
	_, err := ResolveAvailability(m, "docker", availability.Support{
		Level:  availability.Unsupported,
		Limits: "single-host runtime",
	})
	var typed *availability.UnsupportedGuaranteeError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %v, want UnsupportedGuaranteeError", err)
	}
}

func TestResolveAvailabilityAllowsExplicitNonHAException(t *testing.T) {
	m := New("demo", "prod", false, false, false)
	m.Services.SQL = false
	m.ApplicationID = "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"
	m.Workload.Components = []string{"api"}
	m.HA = true
	disabled := false
	m = WithAvailabilityOverride(m, "api", &disabled, 0)
	got, err := ResolveAvailability(m, "docker", availability.Support{
		Level:  availability.Unsupported,
		Limits: "single-host runtime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || !got.Results[0].Satisfied || got.Results[0].RequiredHA {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestProviderIntentMatchesNativeHAAndRejectsUnsupportedMemberOverrides(t *testing.T) {
	m := New("demo", "dev", true, false, false)
	enabled := true
	disabled := false
	m.Availability = map[string]availability.Override{"sql": {HA: &enabled, Instances: 5}}
	result, err := ResolveAvailability(m, "docker", availability.Support{Level: availability.Unsupported})
	if err != nil || len(result.Results) != 1 || result.Results[0].EffectiveInstances != 5 {
		t.Fatalf("native shared SQL HA contract drift: %#v %v", result, err)
	}
	m.Availability["sql"] = availability.Override{HA: &disabled, Instances: 3}
	if _, err := ResolveAvailability(m, "docker", availability.Support{Level: availability.Unsupported}); err == nil {
		t.Fatal("non-HA provider silently negotiated multiple members")
	}
	m.Availability["sql"] = availability.Override{HA: &enabled, Instances: 2}
	if _, err := ResolveAvailability(m, "docker", availability.Support{Level: availability.Unsupported}); err == nil {
		t.Fatal("unsupported even datastore quorum negotiated")
	}
	m.Availability["sql"] = availability.Override{HA: &enabled}
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	if _, err := ResolveAvailability(m, "docker", availability.Support{Level: availability.Unsupported}); err == nil {
		t.Fatal("single-only application SQL silently advertised as native HA")
	}
}
