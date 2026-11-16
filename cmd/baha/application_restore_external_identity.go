package main

import (
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func restoreExternalRecoveryScope(resolved resolvedApplication) ([]application.OperationalConsequence, string, error) {
	effects, receiverRevision, err := restoreExternalReceiverScope(resolved)
	if err != nil {
		return nil, "", err
	}
	identityEffects, identityRevision, err := restoreExternalIdentityScope(resolved)
	if err != nil {
		return nil, "", err
	}
	if identityRevision == "" {
		return effects, receiverRevision, nil
	}
	return append(effects, identityEffects...), runtimeMutationRevision(struct{ Receiver, Identity string }{receiverRevision, identityRevision}), nil
}

// Retirement withdraws Identity intent. Recovery must bind the original intent's
// selected issuer and private input snapshot even when no applied owner survives.
func restoreExternalIdentityScope(resolved resolvedApplication) ([]application.OperationalConsequence, string, error) {
	if !application.HasIdentity(resolved.Manifest) {
		return nil, "", nil
	}
	selected, err := application.IdentityProviderForDeployment()
	if err != nil || selected.Kind != capability.ProviderExternalOIDC {
		return nil, "", err
	}
	observation := application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{}}
	if err := appendExternalIdentityChangeEffects(resolved, &observation); err != nil {
		return nil, "", err
	}
	change, err := identityprovider.InspectRegisteredExternalBindingChange(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest), resolved.TargetStateRoot)
	if err != nil {
		return nil, "", err
	}
	effects := []application.OperationalConsequence{{Aspect: "external-identity-recovery", Before: "Existing or archived login, client credentials, trust, tokens and sessions belong to their original issuer and consumer.", After: "Recovery selects " + disclosedExternalReceiver(change.IssuerAfter) + ". Only explicitly selected registered client, credential and trust material may be delivered. Original login, token validation/JWKS refresh, session and refresh-token continuity is not established by archive restoration; actual consumer authentication must be verified. Foreign issuer infrastructure is not changed.", Evidence: "original recovery intent, canonical external registration and protected material comparison; no confidential values or standalone material fingerprints are disclosed"}}
	ids := make([]string, 0, len(observation.Providers))
	for id := range observation.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		effects = append(effects, observation.Providers[id].Effects...)
	}
	return effects, runtimeMutationRevision(struct{ Observation, Selected string }{observation.Revision, change.Revision}), nil
}
