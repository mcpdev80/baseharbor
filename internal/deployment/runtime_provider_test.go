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
	if values[RuntimeProfileEnvKey] != string(RuntimeProfileStandard) {
		t.Fatalf("runtime profile = %q, want %q", values[RuntimeProfileEnvKey], RuntimeProfileStandard)
	}
	got, err := RuntimeProviderStateFromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
}

func TestApplyRuntimeProviderStatePreservesExplicitProfile(t *testing.T) {
	values := map[string]string{RuntimeProfileEnvKey: "future-profile"}
	if err := ApplyRuntimeProviderState(values, RuntimeProviderState{Provider: bhruntime.ProviderCompose}); err != nil {
		t.Fatal(err)
	}
	if values[RuntimeProfileEnvKey] != "future-profile" {
		t.Fatalf("explicit runtime profile changed: %#v", values)
	}
}

func TestRuntimeProviderStateAcceptsKnownFutureProvider(t *testing.T) {
	for provider, want := range map[string]bhruntime.ProviderKind{
		"kubernetes": bhruntime.ProviderKubernetes,
		"openshift":  bhruntime.ProviderOpenShift,
	} {
		state, err := RuntimeProviderStateFromValues(map[string]string{RuntimeProviderEnvKey: provider})
		if err != nil {
			t.Fatalf("provider %q error = %v", provider, err)
		}
		if state.Provider != want {
			t.Fatalf("provider %q resolved as %q, want %q", provider, state.Provider, want)
		}
	}
}

func TestApplyRuntimeProviderStateRejectsNilTarget(t *testing.T) {
	if err := ApplyRuntimeProviderState(nil, RuntimeProviderState{Provider: bhruntime.ProviderCompose}); err == nil {
		t.Fatal("expected nil target to fail")
	}
}
