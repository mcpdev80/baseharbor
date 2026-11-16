package capability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/availability"
)

func TestEveryReferenceProviderHasAvailabilityClassification(t *testing.T) {
	for _, descriptor := range ReferenceIntegrations() {
		support, err := AvailabilitySupportForProvider(descriptor.Provider.Kind)
		if err != nil {
			t.Fatalf("%s: %v", descriptor.ID, err)
		}
		if err := support.Validate(); err != nil {
			t.Fatalf("%s: %v", descriptor.ID, err)
		}
		if support.Level == availability.Supported {
			if support.RecommendedInstances < 2 {
				t.Fatalf("%s claims HA support with recommended instances %d", descriptor.ID, support.RecommendedInstances)
			}
			if !support.Guarantees.MemberFailureTolerance {
				t.Fatalf("%s claims HA support without member-failure tolerance", descriptor.ID)
			}
			if support.Guarantees.FailureDomain == "" {
				t.Fatalf("%s claims HA support without an explicit failure domain", descriptor.ID)
			}
		}
	}
}
