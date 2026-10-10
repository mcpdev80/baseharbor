package application

import (
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func ComponentHA(m Manifest, component string) bool {
	return AvailabilityIntent(m).Resolve(component).HA
}

// managedHAMemberCount resolves runtime-only member cardinality from the
// provider-neutral availability intent. Provider/member names never enter the
// portable application contract.
func managedHAMemberCount(m Manifest, component string, recommended int) int {
	req := AvailabilityIntent(m).Resolve(component)
	if !req.HA {
		return 1
	}
	if req.Instances > 0 {
		return req.Instances
	}
	if recommended > 1 {
		return recommended
	}
	return 3
}

func validateProviderAvailabilityIntent(m Manifest) error {
	providers := map[string]capability.ProviderKind{"sql": capability.ProviderPostgreSQL, "cache": capability.ProviderValkey, "key_value": capability.ProviderValkey, "messaging": capability.ProviderRabbitMQ, "document_database": capability.ProviderMongoDB, "object_storage": capability.ProviderSeaweedFS, "identity": capability.ProviderKeycloak, "secrets": capability.ProviderOpenBao, "metrics": capability.ProviderPrometheus, "telemetry": capability.ProviderOTelCollector, "logs": capability.ProviderLoki, "traces": capability.ProviderTempo}
	for _, component := range AvailabilityIntent(m).Components() {
		if provider, exists := providers[component]; exists {
			if err := capability.ValidateAvailabilityMembers(provider, AvailabilityIntent(m).Resolve(component)); err != nil {
				return fmt.Errorf("component %s: %w", component, err)
			}
		}
	}
	return nil
}
