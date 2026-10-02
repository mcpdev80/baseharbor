package application

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/availability"
)

func TestResolveAvailabilityFailsBeforeMutationForUnsupportedRuntimeHA(t *testing.T) {
	m := New("demo", "prod", false, false, false)
	m.ApplicationID = "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"
	m.Workload.Components = []string{"api"}
	m.HA = true
	_, err := ResolveAvailability(m, "docker", availability.Support{
		Level: availability.Unsupported,
		Limits: "single-host runtime",
	})
	var typed *availability.UnsupportedGuaranteeError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %v, want UnsupportedGuaranteeError", err)
	}
}

func TestResolveAvailabilityAllowsExplicitNonHAException(t *testing.T) {
	m := New("demo", "prod", false, false, false)
	m.ApplicationID = "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"
	m.Workload.Components = []string{"api"}
	m.HA = true
	disabled := false
	m = WithAvailabilityOverride(m, "api", &disabled, 0)
	got, err := ResolveAvailability(m, "docker", availability.Support{
		Level: availability.Unsupported,
		Limits: "single-host runtime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || !got.Results[0].Satisfied || got.Results[0].RequiredHA {
		t.Fatalf("resolution = %#v", got)
	}
}
