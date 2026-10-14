package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type externalReceiverChange struct {
	instance string
	before   application.RuntimeOTLPBinding
	endpoint string
	headers  string
	explicit bool
}

// Existing deployment and registry owners remain authoritative. A receiver or
// authorization change can require consent even when its provider ID is stable.
func applicationExternalReceiverChange(resolved resolvedApplication) (*externalReceiverChange, error) {
	if !application.HasOTLPTelemetry(resolved.Manifest) || application.TelemetryProviderForDeployment().Kind != capability.ProviderExternalOTLP {
		return nil, nil
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	id := ""
	for _, binding := range registry.Bindings {
		if binding.ApplicationID == resolved.Manifest.ApplicationID && (binding.Environment == "" || binding.Environment == resolved.Manifest.Environment) && binding.Resource.Kind == capability.TelemetryOTLP && binding.Resource.Provider == capability.ProviderExternalOTLP {
			id = binding.ProviderInstanceID
		}
	}
	if id == "" {
		return nil, nil
	}
	bound, ok, err := application.ExistingRuntimeOTLPBinding(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil {
		return nil, err
	}
	if !ok || bound.Provider != capability.ProviderExternalOTLP {
		return nil, nil // Missing execution evidence cannot invent a prior receiver.
	}
	input, explicit := os.LookupEnv(telemetry.ExternalHeadersEnv)
	endpoint, headers := application.ExternalOTLPEndpoint(), strings.TrimSpace(input)
	if !explicit && strings.TrimRight(endpoint, "/") == strings.TrimRight(bound.ContainerEndpoint, "/") {
		headers = bound.Headers
	}
	if endpoint == bound.ContainerEndpoint && headers == bound.Headers {
		return nil, nil
	}
	return &externalReceiverChange{instance: id, before: bound, endpoint: endpoint, headers: headers, explicit: explicit}, nil
}

func externalReceiverChangeRequests(resolved resolvedApplication, requests []application.ProviderChangeRequest) ([]application.ProviderChangeRequest, error) {
	change, err := applicationExternalReceiverChange(resolved)
	if err != nil || change == nil {
		return requests, err
	}
	for _, request := range requests {
		if request.InstanceID == change.instance {
			return requests, nil
		}
	}
	consumer := "application/" + resolved.Manifest.ApplicationID + "/" + resolved.Manifest.Environment
	return append(requests, application.ProviderChangeRequest{InstanceID: change.instance, Operation: "rebind", Selection: "explicit", Consumers: []string{consumer}, Materials: []application.MaterialDisposition{}}), nil
}

func appendExternalReceiverChangeEffects(resolved resolvedApplication, observation *application.ProviderImpactObservation) error {
	change, err := applicationExternalReceiverChange(resolved)
	if err != nil || change == nil {
		return err
	}
	fact := observation.Providers[change.instance]
	fact.Effects = append(fact.Effects,
		application.OperationalConsequence{Aspect: "telemetry-receiver", Before: disclosedExternalReceiver(change.before.ContainerEndpoint), After: disclosedExternalReceiver(change.endpoint), Evidence: "existing protected consumer binding and selected external endpoint; delivery to destination remains unverified"},
		application.OperationalConsequence{Aspect: "telemetry-authorization", Before: "Current protected deployment authorization belongs to its existing receiver.", After: "The destination uses explicitly selected authorization, explicitly selected anonymous access, or same-receiver retained authorization. Earlier receiver credentials are not implicitly forwarded. Trace delivery, buffered signals and destination retention require actual verification.", Evidence: "protected deployment owner and current selection; no credential contents are disclosed"})
	fact.InventoryComplete = false
	observation.Providers[change.instance] = fact
	observation.InventoryComplete = false
	// Only the final opaque revision enters the plan. Material bytes and their
	// standalone digests never appear in effects or machine output.
	raw := observation.Revision + "\x00" + change.before.ContainerEndpoint + "\x00" + change.before.Headers + "\x00" + change.endpoint + "\x00" + change.headers
	if change.explicit {
		raw += "\x00explicit"
	}
	digest := sha256.Sum256([]byte(raw))
	observation.Revision = hex.EncodeToString(digest[:])
	return nil
}

func disclosedExternalReceiver(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "unverifiable receiver; no credential-bearing URL is disclosed"
	}
	return u.String()
}
