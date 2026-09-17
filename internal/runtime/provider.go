package runtime

import "context"

// ProviderKind identifies the runtime implementation selected for application
// workloads. It is deployment configuration, not part of the portable
// application contract.
type ProviderKind string

const (
	ProviderCompose ProviderKind = "compose"
)

// ProviderCapabilities describes runtime behavior that orchestration may rely
// on. Capability providers such as PostgreSQL, Valkey or object storage are a
// separate axis and must not be encoded here.
type ProviderCapabilities struct {
	WorkloadLifecycle bool
	ServiceExec       bool
	PublishedPorts    bool
	ResourceOwnership bool
}

// Provider is the minimal runtime-provider seam. It deliberately exposes only
// provider identity and runtime capabilities at this stage; provider-specific
// lifecycle inputs stay behind the concrete implementation until a portable
// operation has a demonstrated second implementation.
type Provider interface {
	Kind() ProviderKind
	Capabilities() ProviderCapabilities
}

func (Compose) Kind() ProviderKind {
	return ProviderCompose
}

func (Compose) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		WorkloadLifecycle: true,
		ServiceExec:       true,
		PublishedPorts:    true,
		ResourceOwnership: true,
	}
}

// DetectProvider is the single runtime-provider selection seam. v0.4 currently
// selects only the existing Compose implementation. Kubernetes/OpenShift must
// later be selected explicitly from deployment/environment configuration rather
// than inferred from the application contract.
func DetectProvider(ctx context.Context) (Provider, error) {
	return detectCompose(ctx)
}
