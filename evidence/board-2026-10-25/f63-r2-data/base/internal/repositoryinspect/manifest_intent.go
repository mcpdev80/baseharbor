package repositoryinspect

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// CapabilityIntentsFromManifest normalizes explicit Application Intent through
// the portable contract before repository evidence is reconciled. Repository
// inspection therefore consumes the same capability truth as plan/runtime/MCP
// and does not maintain a second manifest-specific capability catalog.
func CapabilityIntentsFromManifest(manifest application.Manifest) ([]CapabilityIntent, error) {
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		return nil, err
	}
	intents := make([]CapabilityIntent, 0, len(contract.Capabilities)+1)
	for _, requirement := range contract.Capabilities {
		intents = append(intents, CapabilityIntent{
			Capability: string(requirement.Kind),
			Name:       requirement.Name,
			Direction:  capabilityIntentDirection(requirement.Kind),
		})
	}
	if contract.Secrets.Managed {
		intents = append(intents, CapabilityIntent{
			Capability: string(capability.Secrets),
			Direction:  DirectionConsume,
		})
	}
	return intents, nil
}

func capabilityIntentDirection(kind capability.Kind) Direction {
	switch kind {
	case capability.ExposureHTTP, capability.Metrics:
		return DirectionProvide
	case capability.TelemetryOTLP, capability.Logs:
		return DirectionExport
	default:
		return DirectionConsume
	}
}
