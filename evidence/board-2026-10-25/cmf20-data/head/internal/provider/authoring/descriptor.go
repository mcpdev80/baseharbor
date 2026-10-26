package authoring

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

type Descriptor struct {
	ID               string   `json:"id" yaml:"id"`
	Version          string   `json:"version" yaml:"version"`
	ProviderProtocol string   `json:"providerProtocol" yaml:"providerProtocol"`
	ServiceKinds     []string `json:"serviceKinds" yaml:"serviceKinds"`
	ServiceContracts []string `json:"serviceContracts" yaml:"serviceContracts"`
	Capabilities     []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	SupportedScopes  []string `json:"supportedScopes" yaml:"supportedScopes"`
}

func (d Descriptor) Validate() error {
	meta := extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            d.ID,
		Family:        extension.FamilyProvider,
		Version:       d.Version,
	}
	if err := meta.Validate(); err != nil {
		return err
	}
	if d.ProviderProtocol != capability.ProviderProtocolV1 {
		return fmt.Errorf("provider protocol %q is unsupported; expected %q", d.ProviderProtocol, capability.ProviderProtocolV1)
	}
	if len(d.ServiceContracts) == 0 {
		return fmt.Errorf("provider %q requires at least one service contract", d.ID)
	}
	if len(d.SupportedScopes) == 0 {
		return fmt.Errorf("provider %q requires at least one supported scope", d.ID)
	}
	_, err := d.IntegrationDescriptor()
	return err
}

func (d Descriptor) IntegrationDescriptor() (capability.IntegrationDescriptor, error) {
	var (
		kinds    []capability.Kind
		specs    []capability.SpecificationID
		services []capability.ServiceKind
	)
	seenKinds := map[capability.Kind]struct{}{}
	seenServices := map[capability.ServiceKind]struct{}{}
	for _, value := range d.ServiceContracts {
		id := capability.SpecificationID(strings.TrimSpace(value))
		spec, err := capability.ParseSpecificationID(id)
		if err != nil {
			return capability.IntegrationDescriptor{}, fmt.Errorf("service contract %q: %w", value, err)
		}
		if _, exists := seenKinds[spec.Kind]; exists {
			return capability.IntegrationDescriptor{}, fmt.Errorf("service contract for capability %q is declared more than once", spec.Kind)
		}
		seenKinds[spec.Kind] = struct{}{}
		kinds = append(kinds, spec.Kind)
		specs = append(specs, spec.ID)
		service, err := capability.ServiceKindForCapability(spec.Kind)
		if err != nil {
			return capability.IntegrationDescriptor{}, err
		}
		if _, exists := seenServices[service]; !exists {
			seenServices[service] = struct{}{}
			services = append(services, service)
		}
	}
	if len(d.ServiceKinds) > 0 {
		declared := map[string]struct{}{}
		for _, service := range d.ServiceKinds {
			declared[strings.TrimSpace(service)] = struct{}{}
		}
		for _, service := range services {
			if _, ok := declared[string(service)]; !ok {
				return capability.IntegrationDescriptor{}, fmt.Errorf("serviceKinds does not include derived service %q", service)
			}
		}
		if len(declared) != len(services) {
			return capability.IntegrationDescriptor{}, fmt.Errorf("serviceKinds contains entries not derived from serviceContracts")
		}
	}
	var scopes []capability.ProviderScope
	for _, value := range d.SupportedScopes {
		scope := capability.ProviderScope(strings.TrimSpace(value))
		switch scope {
		case capability.ScopeShared, capability.ScopeApplication, capability.ScopeExternal:
			scopes = append(scopes, scope)
		default:
			return capability.IntegrationDescriptor{}, fmt.Errorf("unsupported provider scope %q", value)
		}
	}
	return capability.IntegrationDescriptor{
		ID:       d.ID,
		Version:  d.Version,
		Protocol: d.ProviderProtocol,
		Provider: capability.Provider{
			Kind:         capability.ProviderKind(d.ID),
			Capabilities: kinds,
		},
		Services:        services,
		Capabilities:    specs,
		SupportedScopes: scopes,
	}, nil
}

func Check(d Descriptor) capability.ConformanceReport {
	integration, err := d.IntegrationDescriptor()
	if err != nil {
		return capability.ConformanceReport{
			ProviderID: d.ID,
			Protocol:   d.ProviderProtocol,
			Status:     capability.ConformanceFail,
			Checks: []capability.ConformanceCheck{{
				Name: "provider-descriptor", Status: capability.ConformanceFail, Message: err.Error(),
			}},
		}
	}
	return capability.CheckIntegrationContract(integration)
}
