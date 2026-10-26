package main

import "strings"

// Public diagnostics identify only known lifecycle stages. Arbitrary causes
// can contain protected input or native command output and remain private.
func providerConsumerInventoryStage(err error) string {
	if err != nil {
		for _, reason := range []struct{ prefix, stage string }{
			{"native backend retirement requires complete resolved consumer inventory", "native-backend-retirement"},
			{"exposure retirement requires its complete unused application owner plan", "exposure-retirement"},
			{"provider retirement plan changed for ", "provider-retirement"},
			{"runtime intent differs from deployment owner", "runtime-owner"},
			{"broker intent differs from deployment owner", "broker-owner"},
			{"previous deployment intent does not match its owner", "previous-owner"},
			{"installation binding differs from selected Target", "installation-target"},
			{"runtime broker dependency ", "broker-dependency"},
			{"invalid retained backend selection", "retained-backend-selection"},
			{"ambiguous retained backend placement", "retained-backend-placement"},
		} {
			if strings.HasPrefix(err.Error(), reason.prefix) {
				return reason.stage
			}
		}
	}
	return "canonical-consumer-inventory"
}
