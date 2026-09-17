package runtime

import "testing"

var _ Provider = Compose{}

func TestComposeProviderMetadata(t *testing.T) {
	var provider Provider = Compose{}
	if got, want := provider.Kind(), ProviderCompose; got != want {
		t.Fatalf("Kind() = %q, want %q", got, want)
	}

	caps := provider.Capabilities()
	if !caps.WorkloadLifecycle {
		t.Fatal("Compose provider must support workload lifecycle")
	}
	if !caps.ServiceExec {
		t.Fatal("Compose provider must support service exec")
	}
	if !caps.PublishedPorts {
		t.Fatal("Compose provider must support published-port inspection")
	}
	if !caps.ResourceOwnership {
		t.Fatal("Compose provider must support project resource ownership checks")
	}
}

func TestProviderKindIsDeploymentMetadata(t *testing.T) {
	if ProviderCompose == "" {
		t.Fatal("Compose provider kind must be stable and non-empty")
	}
	if got, want := string(ProviderCompose), "compose"; got != want {
		t.Fatalf("ProviderCompose = %q, want %q", got, want)
	}
}

func TestParseProviderKindDefaultsToCompose(t *testing.T) {
	for _, input := range []string{"", " ", "compose", " COMPOSE "} {
		got, err := ParseProviderKind(input)
		if err != nil {
			t.Fatalf("ParseProviderKind(%q) error = %v", input, err)
		}
		if got != ProviderCompose {
			t.Fatalf("ParseProviderKind(%q) = %q, want %q", input, got, ProviderCompose)
		}
	}
}

func TestParseProviderKindRejectsUnavailableProvider(t *testing.T) {
	for _, input := range []string{"kubernetes", "openshift", "docker"} {
		if _, err := ParseProviderKind(input); err == nil {
			t.Fatalf("ParseProviderKind(%q) unexpectedly succeeded", input)
		}
	}
}
