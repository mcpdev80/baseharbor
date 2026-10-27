package capability

import (
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/availability"
)

// ValidateAvailabilityMembers prevents negotiation from promising a topology
// the native provider cannot build. Workload scaling is negotiated separately.
func ValidateAvailabilityMembers(kind ProviderKind, req availability.Requirement) error {
	if !req.HA && req.Instances > 1 {
		return fmt.Errorf("provider %s requires explicit ha:true for multiple data/service members", kind)
	}
	if !req.HA || req.Instances == 0 {
		return nil
	}
	n := req.Instances
	switch kind {
	case ProviderPostgreSQL, ProviderSeaweedFS, ProviderValkey, ProviderRabbitMQ, ProviderMongoDB:
		if n < 3 || n%2 == 0 {
			return fmt.Errorf("provider %s HA requires an odd member count of at least three", kind)
		}
	case ProviderOpenBao, ProviderKeycloak, ProviderLoki:
		if n != 3 {
			return fmt.Errorf("provider %s native HA requires exactly three members", kind)
		}
	case ProviderTempo:
		if n != 2 {
			return fmt.Errorf("Tempo native HA requires two service partitions with its fixed replicated storage layout")
		}
	case ProviderPrometheus, ProviderOTelCollector:
		if n < 2 {
			return fmt.Errorf("provider %s HA requires multiple service members", kind)
		}
	}
	return nil
}
