package runtime

import (
	"testing"
)

type runtimeProviderConformanceExpectation struct {
	kind        ProviderKind
	httpsPort   int
	logMode     LogCollectionMode
	realization string
}

func TestFirstPartyRuntimeProviderConformance(t *testing.T) {
	tests := []struct {
		name     string
		provider RuntimeProvider
		want     runtimeProviderConformanceExpectation
	}{
		{
			name:     "docker",
			provider: DockerProvider{},
			want: runtimeProviderConformanceExpectation{
				kind:        ProviderDocker,
				httpsPort:   443,
				logMode:     LogCollectionSyslog,
				realization: "docker-compose",
			},
		},
		{
			name:     "podman",
			provider: PodmanProvider{},
			want: runtimeProviderConformanceExpectation{
				kind:        ProviderPodman,
				httpsPort:   8443,
				logMode:     LogCollectionJournald,
				realization: "podman-quadlet-systemd-user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRuntimeProviderConformance(t, tt.provider, tt.want)
		})
	}
}

func assertRuntimeProviderConformance(t *testing.T, provider RuntimeProvider, want runtimeProviderConformanceExpectation) {
	t.Helper()
	if provider.Kind() != want.kind {
		t.Fatalf("Kind() = %q, want %q", provider.Kind(), want.kind)
	}
	descriptor := provider.Descriptor()
	if descriptor.Kind != want.kind {
		t.Fatalf("descriptor kind = %q, want %q", descriptor.Kind, want.kind)
	}
	if descriptor.ContractVersion != RuntimeProviderContractVersion {
		t.Fatalf("contract version = %q, want %q", descriptor.ContractVersion, RuntimeProviderContractVersion)
	}
	if descriptor.ProviderVersion == "" {
		t.Fatal("provider version is empty")
	}
	if len(descriptor.WorkloadSources) == 0 || descriptor.WorkloadSources[0] != "compose-spec" {
		t.Fatalf("workload sources = %#v, want compose-spec compatibility", descriptor.WorkloadSources)
	}
	if descriptor.Realization != want.realization {
		t.Fatalf("realization = %q, want %q", descriptor.Realization, want.realization)
	}
	if len(descriptor.Standards) == 0 {
		t.Fatal("provider declares no adopted standards")
	}
	if err := RequireCapabilities(
		provider,
		CapabilityWorkloadLifecycle,
		CapabilityServiceExec,
		CapabilityPublishedPorts,
		CapabilityResourceOwnership,
	); err != nil {
		t.Fatalf("required capabilities: %v", err)
	}
	if port := provider.PreferredLocalHTTPSPort(); port != want.httpsPort {
		t.Fatalf("preferred HTTPS port = %d, want %d", port, want.httpsPort)
	}
	if mode := provider.LogCollectionMode(); mode != want.logMode {
		t.Fatalf("log collection mode = %q, want %q", mode, want.logMode)
	}
}
