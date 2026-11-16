package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestProviderConsumerInventoryDiagnosticDoesNotDisclosePrivateCause(t *testing.T) {
	for _, example := range []struct{ cause, stage string }{
		{"native backend retirement requires complete resolved consumer inventory", "native-backend-retirement"},
		{"exposure retirement requires its complete unused application owner plan", "exposure-retirement"},
		{"provider retirement plan changed for private-native-output", "provider-retirement"},
		{"password=private-native-output", "canonical-consumer-inventory"},
	} {
		err := machine.Classify(invalidProviderConsumerInventory(errors.New(example.cause)))
		data, marshalErr := json.Marshal(err)
		if marshalErr != nil || !strings.Contains(string(data), `"inventory_stage":"`+example.stage+`"`) || strings.Contains(string(data), "private-native-output") {
			t.Fatalf("inventory diagnostic disclosed private cause or lost stage: %s %v", data, marshalErr)
		}
		if err.Code != machine.ErrorOwnershipAmbiguous || err.CauseCode != "provider_consumer_inventory_invalid" {
			t.Fatal("diagnostic changed ownership admission semantics")
		}
	}
}
