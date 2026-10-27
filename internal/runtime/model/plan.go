package model

import (
	"fmt"
	"strings"
)

// Target identifies the deployment-owned runtime scope selected for a workload.
// Scope is intentionally opaque. A Kubernetes provider may realize it as a
// namespace; Docker/Podman may realize it differently.
type Target struct {
	Scope string
}

// EnvironmentVariable is a runtime-consumable non-secret environment value.
type EnvironmentVariable struct {
	Name  string
	Value string
}

// ContainerPort is a workload-local endpoint. Host/public publishing is not
// part of the portable runtime handoff.
type ContainerPort struct {
	Container int
	Protocol  string
}

// Service is the normalized runtime workload service.
//
// Important: source/build instructions are structurally absent. Image is an
// already-resolved OCI artifact reference before the runtime boundary.
type Service struct {
	Name        string
	Image       string
	Args        []string
	Environment []EnvironmentVariable
	Ports       []ContainerPort
}

// Binding carries either a public value or an opaque secret reference.
// Plaintext secret material is not part of the portable runtime plan.
type Binding struct {
	Name        string
	PublicValue string
	SecretRef   string
}

// WorkloadPlan is the provider-neutral handoff from BaseHarbor Core to a
// runtime provider. Runtime-native objects are derived only after this boundary.
type WorkloadPlan struct {
	Application string
	Environment string
	Target      Target
	Services    []Service
	Bindings    []Binding
}

func (p WorkloadPlan) Validate() error {
	if strings.TrimSpace(p.Application) == "" {
		return fmt.Errorf("runtime plan application is required")
	}
	if strings.TrimSpace(p.Environment) == "" {
		return fmt.Errorf("runtime plan environment is required")
	}
	if strings.TrimSpace(p.Target.Scope) == "" {
		return fmt.Errorf("runtime plan target scope is required")
	}
	if len(p.Services) == 0 {
		return fmt.Errorf("runtime plan requires at least one service")
	}

	seenServices := map[string]struct{}{}
	for _, service := range p.Services {
		name := strings.TrimSpace(service.Name)
		if name == "" {
			return fmt.Errorf("runtime service name is required")
		}
		if _, exists := seenServices[name]; exists {
			return fmt.Errorf("duplicate runtime service %q", name)
		}
		seenServices[name] = struct{}{}
		if strings.TrimSpace(service.Image) == "" {
			return fmt.Errorf("runtime service %q requires a resolved OCI image", name)
		}
	}

	seenBindings := map[string]struct{}{}
	for _, binding := range p.Bindings {
		name := strings.TrimSpace(binding.Name)
		if name == "" {
			return fmt.Errorf("runtime binding name is required")
		}
		if _, exists := seenBindings[name]; exists {
			return fmt.Errorf("duplicate runtime binding %q", name)
		}
		seenBindings[name] = struct{}{}
		if binding.PublicValue != "" && binding.SecretRef != "" {
			return fmt.Errorf("runtime binding %q cannot contain public value and secret reference", name)
		}
	}
	return nil
}
