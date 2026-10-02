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
			t.Fatalf("%s claims HA support without a v0.4.21 realization proof", descriptor.ID)
		}
	}
}
