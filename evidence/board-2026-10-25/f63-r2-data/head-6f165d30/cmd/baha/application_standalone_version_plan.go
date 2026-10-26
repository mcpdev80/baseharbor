package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"

	"github.com/mcpdev80/baseharbor/internal/traces"
)

func standaloneProviderVersionChangeRequests(ctx context.Context, resolved resolvedApplication, requests []application.ProviderChangeRequest) ([]application.ProviderChangeRequest, error) {
	enabled, err := application.TracesCollectionEnabled(resolved.Manifest)
	if err != nil {
		return nil, err
	}
	if !enabled || isRemoteApplication(resolved) {
		return requests, nil
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	// Runtime-only backend bindings are retained by the existing Target owner;
	// the portable app preview deliberately does not invent these declarations.
	for _, binding := range registry.Bindings {
		if binding.ApplicationID == resolved.Manifest.ApplicationID && binding.Environment == resolved.Manifest.Environment && binding.Resource.Kind == capability.Traces {
			selected[binding.ProviderInstanceID] = true
		}
	}
	seen := map[string]bool{}
	for _, request := range requests {
		seen[request.InstanceID] = true
	}
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return nil, err
	}
	for _, instance := range registry.Instances {
		if !selected[instance.ID] || seen[instance.ID] || instance.Provider.Kind != capability.ProviderTempo || instance.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		required, err := traces.ExistingRuntimeUpgradeRequiredAt(resolved.TargetStateRoot, resolved.Target.Name, prior, providerInstancePlacement(instance))
		if errors.Is(err, os.ErrNotExist) {
			continue
		} // Fresh materialization, not a known retained version change.
		if err != nil {
			return nil, &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "provider_version_owner_unverifiable", Resource: instance.ID, Message: "Retained provider version cannot be verified against its existing protected canonical owner.", Next: "Recover the existing runtime owner before applying; no version or material has been changed.", Cause: err}
		}
		// A failed prior apply may already have written the new renderer while
		// its old container remains. Observe the actual module as well; an
		// unavailable image check is unresolved materialization, not a no-op.
		if !required {
			required = !tempoRuntimeHasCurrentImage(ctx, resolved, prior, instance)
		}
		if required {
			requests = append(requests, application.ProviderChangeRequest{InstanceID: instance.ID, Operation: "update", Selection: "automatic", Consumers: []string{}, Materials: []application.MaterialDisposition{}})
		}
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].InstanceID < requests[j].InstanceID })
	return requests, nil
}

func tempoRuntimeHasCurrentImage(ctx context.Context, resolved resolvedApplication, m application.Manifest, instance capability.ProviderInstance) bool {
	current, _ := tempoRuntimeImageRevision(ctx, resolved, m, instance)
	return current
}

func tempoRuntimeImageRevision(ctx context.Context, resolved resolvedApplication, m application.Manifest, instance capability.ProviderInstance) (bool, string) {
	_, p, resources, err := traces.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, m, providerInstancePlacement(instance))
	if err != nil {
		return false, "unverified"
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return false, "unverified"
	}
	if _, err := provider.InspectProjectResources(ctx, p.Project, resources); err != nil {
		return false, "unverified"
	}
	services := []string{}
	for _, resource := range resources {
		if resource.Kind != "container" {
			continue
		}
		service := strings.TrimSuffix(strings.TrimPrefix(resource.Name, p.Project+"-"), "-1")
		// These resources already matched the exact renderer. Cover all of
		// its Tempo roles, including HA block builders and both frontends;
		// its gateway and Kafka initialization use different images.
		if (service == "tempo" || strings.HasPrefix(service, "tempo-")) && service != "tempo-access" && service != "tempo-kafka-init" {
			services = append(services, service)
		}
	}
	if len(services) == 0 {
		return false, "unverified"
	}
	identities := map[string]any{}
	current := true
	for _, service := range services {
		identity, err := provider.ProjectServiceImageIdentity(ctx, p.Project, service)
		if err != nil || identity.ImageID == "" {
			return false, "unverified"
		}
		identities[service] = identity
		current = current && identity.Reference == traces.ProviderImage
	}
	return current, runtimeMutationRevision(identities)
}

func appendStandaloneVersionChangeEffects(ctx context.Context, resolved resolvedApplication, requests []application.ProviderChangeRequest, observation *application.ProviderImpactObservation) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return
	}
	images := map[string]string{}
	for _, request := range requests {
		if request.Operation != "update" && request.Operation != "resume" && request.Operation != "add" {
			continue
		}
		for _, instance := range registry.Instances {
			if instance.ID != request.InstanceID || instance.Provider.Kind != capability.ProviderTempo {
				continue
			}
			fact, exists := observation.Providers[instance.ID]
			if !exists {
				continue
			}
			before := "Protected current Tempo definition; actual running image may differ after an interrupted apply and is separately revalidated."
			prior, priorErr := retainedApplicationBackendIntent(resolved)
			if priorErr == nil {
				_, images[instance.ID] = tempoRuntimeImageRevision(ctx, resolved, prior, instance)
				previous, imageErr := traces.ExistingRuntimeUpgradeRequiredAt(resolved.TargetStateRoot, resolved.Target.Name, prior, providerInstancePlacement(instance))
				if imageErr == nil && previous {
					before = "Protected retained definition requests Tempo 3.0.2; actual process image, pending exports and retained data are separately observed or unknown."
				}
			}
			fact.Effects = append(fact.Effects, application.OperationalConsequence{Aspect: "provider-version", Before: before, After: "This materialization uses " + traces.ProviderImage + " and explicitly keeps vParquet4 block writes. It can replace the service process; in-flight exports, live version transfer, continued consumer access and rollback require actual verification. Existing volumes and protected access owners are retained; no data-format migration or silent credential handoff is proposed.", Evidence: "current canonical renderer; existing-provider image revalidation; version continuity remains partial until source-matched execution evidence"})
			observation.Providers[instance.ID] = fact
		}
	}
	if len(images) > 0 {
		observation.Revision = runtimeMutationRevision(struct {
			Files  string
			Images map[string]string
		}{observation.Revision, images})
	}
}
