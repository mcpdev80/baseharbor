package capability

import (
	"context"
	"fmt"
	"strings"
)

const ProviderProtocolV1 = "baseharbor.provider/v1"

type ProviderOperation string

const (
	OperationDescribe  ProviderOperation = "describe"
	OperationPreflight ProviderOperation = "preflight"
	OperationProvision ProviderOperation = "provision"
	OperationBind      ProviderOperation = "bind"
	OperationVerify    ProviderOperation = "verify"
	OperationStatus    ProviderOperation = "status"
	OperationUpdate    ProviderOperation = "update"
	OperationBackup    ProviderOperation = "backup"
	OperationRestore   ProviderOperation = "restore"
	OperationDestroy   ProviderOperation = "destroy"
)

type OptionalLifecycleSupport struct {
	Status  bool `json:"status"`
	Update  bool `json:"update"`
	Backup  bool `json:"backup"`
	Restore bool `json:"restore"`
	Destroy bool `json:"destroy"`
}

type ObservabilitySignalKind string

const (
	ObservabilityMetrics ObservabilitySignalKind = "metrics"
	ObservabilityLogs    ObservabilitySignalKind = "logs"
	ObservabilityTraces  ObservabilitySignalKind = "traces"
)

type ProviderObservabilitySignal struct {
	Name     string                  `json:"name"`
	Kind     ObservabilitySignalKind `json:"kind"`
	Protocol string                  `json:"protocol"`
	Port     int                     `json:"port,omitempty"`
	Path     string                  `json:"path,omitempty"`
}

type ProviderObservability struct {
	Signals []ProviderObservabilitySignal `json:"signals,omitempty"`
}

type IntegrationDescriptor struct {
	ID              string                   `json:"id"`
	Version         string                   `json:"version"`
	Protocol        string                   `json:"protocol"`
	Provider        Provider                 `json:"provider"`
	Capabilities    []SpecificationID        `json:"capabilities"`
	SupportedScopes []ProviderScope          `json:"supported_scopes"`
	Optional        OptionalLifecycleSupport `json:"optional_lifecycle"`
	Observability   ProviderObservability    `json:"observability,omitempty"`
}

func (d IntegrationDescriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("provider id is required")
	}
	if strings.TrimSpace(d.Version) == "" {
		return fmt.Errorf("provider %q implementation version is required", d.ID)
	}
	if d.Protocol != ProviderProtocolV1 {
		return fmt.Errorf("provider protocol %q is unsupported; expected %q", d.Protocol, ProviderProtocolV1)
	}
	if d.Provider.Kind == "" {
		return fmt.Errorf("provider kind is required")
	}
	if len(d.Provider.Capabilities) == 0 {
		return fmt.Errorf("provider %q must declare at least one capability", d.Provider.Kind)
	}
	if len(d.Capabilities) == 0 {
		return fmt.Errorf("provider %q must declare versioned capability specifications", d.Provider.Kind)
	}
	if len(d.SupportedScopes) == 0 {
		return fmt.Errorf("provider %q must declare at least one supported placement scope", d.Provider.Kind)
	}
	seenScopes := make(map[ProviderScope]struct{}, len(d.SupportedScopes))
	for _, scope := range d.SupportedScopes {
		switch scope {
		case ScopeShared, ScopeApplication, ScopeExternal:
		default:
			return fmt.Errorf("provider %q declares unsupported placement scope %q", d.Provider.Kind, scope)
		}
		if _, exists := seenScopes[scope]; exists {
			return fmt.Errorf("provider %q declares placement scope %q more than once", d.Provider.Kind, scope)
		}
		seenScopes[scope] = struct{}{}
	}

	declared := make(map[Kind]struct{}, len(d.Capabilities))
	for _, id := range d.Capabilities {
		spec, err := ParseSpecificationID(id)
		if err != nil {
			return fmt.Errorf("provider %q capability %q: %w", d.Provider.Kind, id, err)
		}
		if _, exists := declared[spec.Kind]; exists {
			return fmt.Errorf("provider %q declares capability %q more than once", d.Provider.Kind, spec.Kind)
		}
		if !d.Provider.Supports(spec.Kind) {
			return fmt.Errorf("provider %q specification %q is not present in provider capabilities", d.Provider.Kind, id)
		}
		declared[spec.Kind] = struct{}{}
	}

	for _, kind := range d.Provider.Capabilities {
		if _, exists := declared[kind]; !exists {
			return fmt.Errorf("provider %q capability %q has no versioned capability specification", d.Provider.Kind, kind)
		}
	}
	seenSignals := map[string]struct{}{}
	for _, signal := range d.Observability.Signals {
		signal.Name = strings.TrimSpace(signal.Name)
		signal.Protocol = strings.TrimSpace(strings.ToLower(signal.Protocol))
		if signal.Name == "" {
			return fmt.Errorf("provider %q observability signal name is required", d.Provider.Kind)
		}
		if _, exists := seenSignals[signal.Name]; exists {
			return fmt.Errorf("provider %q observability signal %q is declared more than once", d.Provider.Kind, signal.Name)
		}
		seenSignals[signal.Name] = struct{}{}
		switch signal.Kind {
		case ObservabilityMetrics:
			if signal.Protocol != "openmetrics" {
				return fmt.Errorf("provider %q metrics signal %q must use openmetrics", d.Provider.Kind, signal.Name)
			}
			if signal.Port < 1 || signal.Port > 65535 || !strings.HasPrefix(signal.Path, "/") {
				return fmt.Errorf("provider %q metrics signal %q requires port and absolute path", d.Provider.Kind, signal.Name)
			}
		case ObservabilityLogs, ObservabilityTraces:
			if signal.Protocol == "" {
				return fmt.Errorf("provider %q observability signal %q requires protocol", d.Provider.Kind, signal.Name)
			}
		default:
			return fmt.Errorf("provider %q observability signal %q has unsupported kind %q", d.Provider.Kind, signal.Name, signal.Kind)
		}
	}
	return nil
}

// IntegrationDriver is the semantic boundary shared by built-in providers and
// future external provider adapters. The existing Driver lifecycle remains the
// required mutation path; this interface adds a versioned integration contract.
type IntegrationDriver interface {
	Driver
	IntegrationDescriptor() IntegrationDescriptor
}

// DriverAdapter lets current built-in drivers participate in the integration
// contract without changing their implementation or introducing a second
// lifecycle engine.
type DriverAdapter struct {
	driver     Driver
	descriptor IntegrationDescriptor
}

func NewDriverAdapter(driver Driver, descriptor IntegrationDescriptor) (*DriverAdapter, error) {
	if driver == nil {
		return nil, fmt.Errorf("provider driver is required")
	}
	if err := descriptor.Validate(); err != nil {
		return nil, err
	}
	driverProvider := driver.Descriptor()
	if driverProvider.Kind != descriptor.Provider.Kind {
		return nil, fmt.Errorf(
			"provider driver kind %q does not match integration descriptor %q",
			driverProvider.Kind,
			descriptor.Provider.Kind,
		)
	}
	if !sameCapabilitySet(driverProvider.Capabilities, descriptor.Provider.Capabilities) {
		return nil, fmt.Errorf("provider driver %q capabilities do not match integration descriptor", driverProvider.Kind)
	}
	return &DriverAdapter{driver: driver, descriptor: descriptor}, nil
}

func (a *DriverAdapter) Descriptor() Provider {
	return a.driver.Descriptor()
}

func (a *DriverAdapter) IntegrationDescriptor() IntegrationDescriptor {
	return a.descriptor
}

func (a *DriverAdapter) Preflight(ctx context.Context, resource Resource, binding Binding) error {
	return a.driver.Preflight(ctx, resource, binding)
}

func (a *DriverAdapter) Provision(ctx context.Context, resource Resource, binding Binding) error {
	return a.driver.Provision(ctx, resource, binding)
}

func (a *DriverAdapter) Bind(ctx context.Context, resource Resource, binding Binding) error {
	return a.driver.Bind(ctx, resource, binding)
}

func (a *DriverAdapter) Verify(ctx context.Context, resource Resource, binding Binding) error {
	return a.driver.Verify(ctx, resource, binding)
}

func sameCapabilitySet(a, b []Kind) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[Kind]int, len(a))
	for _, kind := range a {
		seen[kind]++
	}
	for _, kind := range b {
		if seen[kind] == 0 {
			return false
		}
		seen[kind]--
	}
	return true
}
