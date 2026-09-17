package deployment

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRuntimeProviderStateFromValuesDefaultsLegacyStateToCompose(t *testing.T) {
	state, err := RuntimeProviderStateFromValues(map[string]string{
		"BASEHARBOR_HOSTNAME": "mail.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Provider != bhruntime.ProviderCompose {
		t.Fatalf("provider = %q, want %q", state.Provider, bhruntime.ProviderCompose)
	}
}

func TestRuntimeProviderStateRoundTripPreservesOtherDeploymentState(t *testing.T) {
	values := map[string]string{
		"BASEHARBOR_HOSTNAME": "mail.example.test",
		"BASEHARBOR_TLS_MODE": "existing",
	}
	want := RuntimeProviderState{Provider: bhruntime.ProviderCompose}
	if err := ApplyRuntimeProviderState(values, want); err != nil {
		t.Fatal(err)
	}
	if values["BASEHARBOR_HOSTNAME"] != "mail.example.test" || values["BASEHARBOR_TLS_MODE"] != "existing" {
		t.Fatalf("unrelated deployment state changed: %#v", values)
	}
	got, err := RuntimeProviderStateFromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
}

func TestRuntimeProviderStateRejectsUnavailableProvider(t *testing.T) {
	for _, provider := range []string{"kubernetes", "openshift"} {
		if _, err := RuntimeProviderStateFromValues(map[string]string{RuntimeProviderEnvKey: provider}); err == nil {
			t.Fatalf("provider %q unexpectedly accepted", provider)
		}
	}
}

func TestApplyRuntimeProviderStateRejectsNilTarget(t *testing.T) {
	if err := ApplyRuntimeProviderState(nil, RuntimeProviderState{Provider: bhruntime.ProviderCompose}); err == nil {
		t.Fatal("expected nil target to fail")
	}
}
