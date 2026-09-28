package runtime

import "testing"

var _ Provider = DockerProvider{}
var _ Provider = PodmanProvider{}

func TestFirstPartyProviderMetadata(t *testing.T) {
	tests := []struct {
		name       string
		provider   Provider
		descriptor ProviderDescriptor
		kind       ProviderKind
	}{
		{name: "docker", provider: DockerProvider{}, descriptor: DockerProviderDescriptor(), kind: ProviderDocker},
		{name: "podman", provider: PodmanProvider{}, descriptor: PodmanProviderDescriptor(), kind: ProviderPodman},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.Kind(); got != tt.kind {
				t.Fatalf("Kind() = %q, want %q", got, tt.kind)
			}
			if tt.descriptor.Kind != tt.kind {
				t.Fatalf("descriptor kind = %q, want %q", tt.descriptor.Kind, tt.kind)
			}
			if tt.descriptor.ContractVersion != RuntimeProviderContractVersion {
				t.Fatalf("contract = %q, want %q", tt.descriptor.ContractVersion, RuntimeProviderContractVersion)
			}
			if tt.descriptor.ProviderVersion == "" || len(tt.descriptor.WorkloadSources) == 0 || tt.descriptor.Realization == "" {
				t.Fatalf("incomplete descriptor: %#v", tt.descriptor)
			}
			caps := tt.provider.Capabilities()
			if !caps.WorkloadLifecycle || !caps.ServiceExec || !caps.PublishedPorts || !caps.ResourceOwnership {
				t.Fatalf("incomplete provider capabilities: %#v", caps)
			}
		})
	}
}

func TestProviderKindAliasesRemainStable(t *testing.T) {
	if got, want := string(ProviderDocker), "docker"; got != want {
		t.Fatalf("ProviderDocker = %q, want %q", got, want)
	}
	if got, want := string(ProviderPodman), "podman"; got != want {
		t.Fatalf("ProviderPodman = %q, want %q", got, want)
	}
}
