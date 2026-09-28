package docker

import (
	"context"
	"os/exec"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type Provider struct{ bhruntime.Compose }

var capabilities = runtimecontract.ProviderCapabilities{
	WorkloadLifecycle: true,
	ServiceExec:       true,
	PublishedPorts:    true,
	ResourceOwnership: true,
}

func New(ctx context.Context) (*Provider, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, bhruntime.ErrRuntimeNotFound
	}
	cmd := exec.CommandContext(ctx, path, "compose", "version")
	if err := cmd.Run(); err != nil {
		return nil, bhruntime.ErrRuntimeNotFound
	}
	return &Provider{Compose: bhruntime.NewCLIBackend(path, "compose")}, nil
}

func Descriptor() runtimecontract.ProviderDescriptor {
	return runtimecontract.ProviderDescriptor{
		Kind:            runtimecontract.ProviderDocker,
		ContractVersion: runtimecontract.RuntimeProviderContractVersion,
		ProviderVersion: "0.4.17",
		Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
		WorkloadSources: []string{"compose-spec"},
		Realization:     "docker-compose",
		Capabilities:    capabilities,
	}
}

func (p Provider) Kind() runtimecontract.ProviderKind { return runtimecontract.ProviderDocker }
func (p Provider) Descriptor() runtimecontract.ProviderDescriptor { return Descriptor() }
func (p Provider) Capabilities() runtimecontract.ProviderCapabilities { return capabilities }
func (p Provider) PreferredLocalHTTPSPort() int { return 443 }

var _ runtimecontract.RuntimeProvider = (*Provider)(nil)
