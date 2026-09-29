package runtime

import "testing"

func TestProviderKindAliasesRemainStable(t *testing.T) {
	if got, want := string(ProviderDocker), "docker"; got != want {
		t.Fatalf("ProviderDocker = %q, want %q", got, want)
	}
	if got, want := string(ProviderPodman), "podman"; got != want {
		t.Fatalf("ProviderPodman = %q, want %q", got, want)
	}
}
