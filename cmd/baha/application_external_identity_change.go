package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func applicationExternalIdentityChange(resolved resolvedApplication) (string, *identityprovider.ExternalIdentityBindingChange, error) {
	if !application.HasIdentity(resolved.Manifest) {
		return "", nil, nil
	}
	selected, err := application.IdentityProviderForDeployment()
	if err != nil {
		return "", nil, err
	}
	if selected.Kind != capability.ProviderExternalOIDC {
		return "", nil, nil
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return "", nil, err
	}
	for _, binding := range registry.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != "" && binding.Environment != resolved.Manifest.Environment || binding.Resource.Kind != capability.Identity || binding.Resource.Provider != capability.ProviderExternalOIDC {
			continue
		}
		change, err := identityprovider.InspectRegisteredExternalBindingChange(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest), resolved.TargetStateRoot)
		if err != nil {
			return "", nil, err
		}
		if !change.Changed {
			return "", nil, nil
		}
		return binding.ProviderInstanceID, &change, nil
	}
	return "", nil, nil
}

func externalIdentityChangeRequests(resolved resolvedApplication, requests []application.ProviderChangeRequest) ([]application.ProviderChangeRequest, error) {
	id, change, err := applicationExternalIdentityChange(resolved)
	if err != nil || change == nil {
		return requests, err
	}
	for _, request := range requests {
		if request.InstanceID == id {
			return requests, nil
		}
	}
	consumer := "application/" + resolved.Manifest.ApplicationID + "/" + resolved.Manifest.Environment
	return append(requests, application.ProviderChangeRequest{InstanceID: id, Operation: "rebind", Selection: "explicit", Consumers: []string{consumer}, Materials: []application.MaterialDisposition{}}), nil
}

func appendExternalIdentityChangeEffects(resolved resolvedApplication, observation *application.ProviderImpactObservation) error {
	id, change, err := applicationExternalIdentityChange(resolved)
	if err != nil || change == nil {
		return err
	}
	fact := observation.Providers[id]
	fact.Effects = append(fact.Effects,
		application.OperationalConsequence{Aspect: "identity-issuer", Before: disclosedExternalReceiver(change.IssuerBefore), After: disclosedExternalReceiver(change.IssuerAfter), Evidence: "protected applied binding and canonical external registration; login/token/session continuation remains unverified"},
		application.OperationalConsequence{Aspect: "identity-material", Before: "Existing client, confidential credential and trust delivery belong to the original issuer.", After: "The selected registration resolves its explicit client/credential/trust references. Updated delivery can replace these projections; earlier login, refresh/session and recovery access are not automatically migrated. Missing or conflicting secure material prevents execution without changing current delivery.", Evidence: "read-only protected material comparison and registered adapter preparation; no confidential values are disclosed"})
	fact.InventoryComplete = false
	observation.Providers[id] = fact
	observation.InventoryComplete = false
	digest := sha256.Sum256([]byte(observation.Revision + "\x00" + change.Revision))
	observation.Revision = hex.EncodeToString(digest[:])
	return nil
}
