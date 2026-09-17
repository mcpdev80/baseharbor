package deployment

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRuntimeSelectionDefaultsLegacyState(t *testing.T) {
	selection, err := RuntimeSelectionFromValues(map[string]string{
		"BASEHARBOR_ENVIRONMENT": "production",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != bhruntime.ProviderCompose {
		t.Fatalf("provider = %q, want %q", selection.Provider, bhruntime.ProviderCompose)
	}
	if selection.Profile != RuntimeProfileStandard {
		t.Fatalf("profile = %q, want %q", selection.Profile, RuntimeProfileStandard)
	}
}

func TestRuntimeSelectionDoesNotInferFromEnvironmentName(t *testing.T) {
	for _, environment := range []string{"dev", "staging", "production", "enterprise"} {
		selection, err := RuntimeSelectionFromValues(map[string]string{
			"BASEHARBOR_ENVIRONMENT": environment,
		})
		if err != nil {
			t.Fatal(err)
		}
		if selection.Provider != bhruntime.ProviderCompose || selection.Profile != RuntimeProfileStandard {
			t.Fatalf("environment %q changed runtime selection: %#v", environment, selection)
		}
	}
}

func TestRuntimeSelectionRoundTripPreservesOtherState(t *testing.T) {
	values := map[string]string{
		"BASEHARBOR_HOSTNAME": "mail.example.test",
		"BASEHARBOR_TLS_MODE": "existing",
	}
	want := RuntimeSelection{
		Provider: bhruntime.ProviderCompose,
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

func TestRuntimeSelectionRejectsUnknownProfile(t *testing.T) {
	if _, err := RuntimeSelectionFromValues(map[string]string{
		RuntimeProfileEnvKey: "enterprise-ha",
	}); err == nil {
		t.Fatal("unknown runtime profile unexpectedly accepted")
	}
}

func TestRuntimeSelectionRejectsUnavailableProvider(t *testing.T) {
	if _, err := RuntimeSelectionFromValues(map[string]string{
		RuntimeProviderEnvKey: "kubernetes",
		RuntimeProfileEnvKey:  "standard",
	}); err == nil {
		t.Fatal("unavailable runtime provider unexpectedly accepted")
	}
}
