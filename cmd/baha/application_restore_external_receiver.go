package main

import (
	"os"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

// Retirement temporarily withdraws capabilities, while recovery will deliver
// the original manifest again. Bind and disclose that destination using the
// existing receiver projection before any restore delivery can occur.
func restoreExternalReceiverScope(resolved resolvedApplication) ([]application.OperationalConsequence, string, error) {
	if !application.HasOTLPTelemetry(resolved.Manifest) || application.TelemetryProviderForDeployment().Kind != capability.ProviderExternalOTLP {
		return nil, "", nil
	}
	observation := application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{}}
	if err := appendExternalReceiverChangeEffects(resolved, &observation); err != nil {
		return nil, "", err
	}
	effects := []application.OperationalConsequence{{Aspect: "external-telemetry-recovery", Before: "Retained or archived telemetry delivery and authorization belong to their original receiver.", After: "Recovery selects " + disclosedExternalReceiver(application.ExternalOTLPEndpoint()) + ". Delivery uses explicitly selected authorization or same-receiver protected material; no implicit credential transfer, buffered-signal recovery or working destination is verified by this plan.", Evidence: "selected external OTLP configuration and current protected delivery owner; actual destination delivery must be verified after recovery"}}
	ids := make([]string, 0, len(observation.Providers))
	for id := range observation.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		effects = append(effects, observation.Providers[id].Effects...)
	}
	input, explicit := os.LookupEnv(telemetry.ExternalHeadersEnv)
	// Only the final opaque scope revision leaves this function. Neither header
	// bytes nor standalone material digests are included in public consequences.
	revision := runtimeMutationRevision(struct {
		Endpoint, Authorization, Existing string
		Explicit                          bool
	}{application.ExternalOTLPEndpoint(), strings.TrimSpace(input), observation.Revision, explicit})
	return effects, revision, nil
}
