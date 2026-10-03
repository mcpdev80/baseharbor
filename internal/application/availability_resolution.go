package application

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// AvailabilityResolution is the provider/runtime-neutral requested/resolved
// availability truth used by CLI, JSON and MCP callers.
type AvailabilityResolution struct {
	Results []availability.NegotiationResult `json:"results"`
}

func ResolveAvailability(m Manifest, runtimeProvider string, runtimeSupport availability.Support) (AvailabilityResolution, error) {
	if err := AvailabilityIntent(m).Validate(); err != nil {
		return AvailabilityResolution{}, err
	}
	intent := AvailabilityIntent(m)
	var out AvailabilityResolution

	for _, component := range WorkloadComponentNames(m) {
		req := intent.Resolve(component)
		result, err := availability.Negotiate(req, runtimeProvider, runtimeSupport)
		out.Results = append(out.Results, result)
		if err != nil {
			return out, fmt.Errorf("workload component %q: %w", component, err)
		}
	}

	contract, err := PortableContractFromManifest(m)
	if err != nil {
		return out, err
	}
	for _, requirement := range contract.Capabilities {
		provider, err := referenceCapabilityProvider(requirement.Kind)
		if err != nil {
			return out, err
		}
		component := availabilityComponentForCapability(requirement.Kind)
		req := intent.Resolve(component)
		support, err := capability.AvailabilitySupportForProvider(provider.Kind)
		if err != nil {
			return out, err
		}
		result, err := availability.Negotiate(req, string(provider.Kind), support)
		if strings.TrimSpace(requirement.Name) != "" && requirement.Name != "default" {
			result.Component = component + "/" + requirement.Name
		}
		out.Results = append(out.Results, result)
		if err != nil {
			return out, fmt.Errorf("capability %s %q: %w", requirement.Kind, requirement.Name, err)
		}
	}
	if m.Services.Secrets {
		req := intent.Resolve("secrets")
		support, err := capability.AvailabilitySupportForProvider(capability.ProviderOpenBao)
		if err != nil {
			return out, err
		}
		result, err := availability.Negotiate(req, string(capability.ProviderOpenBao), support)
		out.Results = append(out.Results, result)
		if err != nil {
			return out, fmt.Errorf("capability secrets: %w", err)
		}
	}
	return out, nil
}

func availabilityComponentForCapability(kind capability.Kind) string {
	switch kind {
	case capability.SQL:
		return "sql"
	case capability.KeyValue:
		return "cache"
	case capability.DurableKeyValue:
		return "key_value"
	case capability.DocumentDatabase:
		return "document_database"
	case capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream:
		return "messaging"
	case capability.ObjectStorageS3:
		return "object_storage"
	case capability.Identity:
		return "identity"
	case capability.TelemetryOTLP:
		return "telemetry"
	case capability.Metrics:
		return "metrics"
	case capability.Logs:
		return "logs"
	case capability.ExposureHTTP:
		return "exposure"
	default:
		return string(kind)
	}
}
