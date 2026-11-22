package orgconfig

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

type EffectiveValue struct {
	Name   string `json:"name,omitempty"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

type EffectiveProvider struct {
	Provider  string `json:"provider"`
	Reference string `json:"reference"`
	Scope     string `json:"scope,omitempty"`
	Source    string `json:"source"`
}

type EffectivePolicy struct {
	Policy    string `json:"policy"`
	Reference string `json:"reference"`
	Mandatory bool   `json:"mandatory"`
	Source    string `json:"source"`
}

// EnvironmentResolutionInput carries the Core-authoritative identity for the
// active environment as stamped by the deployment registry (card 0428-054,
// environment.json). It is optional and orthogonal to the organization-config
// definition layer: callers that have no registry identity in scope pass the
// zero value, which leaves reconciliation inert but never weakens the
// provider/defaults vs mandatory-policy resolution.
type EnvironmentResolutionInput struct {
	// CoreIdentity is the registry-returned durable identity reference for the
	// active environment. Empty means "no registry identity available", which
	// the reconciliation treats as an absent authoritative source rather than
	// a matching one.
	CoreIdentity string
}

// Effective is the resolved view of the organization configuration for one
// environment. EnvironmentIdentity (card 0428-056) reconciles the
// organization-config identity declaration against the Core registry identity;
// it is only populated when they match and is never an authorization
// credential on its own.
type Effective struct {
	ContractVersion string                       `json:"contract_version"`
	Organization    string                       `json:"organization"`
	Environment     string                       `json:"environment"`
	Target          *EffectiveValue              `json:"target,omitempty"`
	Stack           *EffectiveValue              `json:"stack,omitempty"`
	Providers       map[string]EffectiveProvider `json:"providers,omitempty"`
	Trust           map[string]EffectiveValue    `json:"trust,omitempty"`
	Policies        []EffectivePolicy            `json:"policies,omitempty"`
	// EnvironmentIdentity reconciles the organization-config declaration
	// (definition layer) against the Core registry identity (runtime source of
	// truth, card 0428-054). It is populated only when the declared reference
	// matches the registry identity; a mismatch returns an error and leaves
	// this nil so a drifted identity never leaks into downstream resolution.
	EnvironmentIdentity *EffectiveEnvironmentIdentity `json:"environment_identity,omitempty"`
	Resolution          Resolution                    `json:"resolution"`
	Provenance          map[string]ValueProvenance    `json:"provenance,omitempty"`
	PolicyDecisions     []ConstraintDecision          `json:"policy_decisions,omitempty"`
}

// EffectiveEnvironmentIdentity is the reconciled identity for the active
// environment. Authoritative is true only after the organization-config
// declaration matches the Core registry identity; it is never an
// authorization credential on its own.
type EffectiveEnvironmentIdentity struct {
	Reference     string `json:"reference"`
	Authoritative bool   `json:"authoritative"`
	Source        string `json:"source"`
}

// resolveEffectiveInner is the shared resolution core. ResolveEffective is the
// definition-layer-only entry point with the stable two-or-three-arg public
// signature; ResolveEffectiveWithCoreIdentity adds card 0428-056
// environment-identity reconciliation on top of it.
func resolveEffectiveInner(state ActiveState, environment string, coreEnv EnvironmentResolutionInput, preferences []PreferenceLayer) (Effective, error) {
	layers, err := orderedPreferences(preferences)
	if err != nil {
		return Effective{}, err
	}
	if err := state.Config.Validate(); err != nil {
		return Effective{}, err
	}
	if err := state.Resolution.Validate(); err != nil {
		return Effective{}, err
	}
	environment = strings.TrimSpace(environment)
	if environment == "" {
		environment = "dev"
	}
	result := Effective{
		ContractVersion: ContractVersion,
		Organization:    state.Config.Organization,
		Environment:     environment,
		Providers:       map[string]EffectiveProvider{},
		Trust:           map[string]EffectiveValue{},
		Resolution:      state.Resolution,
		Provenance:      map[string]ValueProvenance{},
	}
	builtin := EnvironmentDefaults{Target: "local"}
	recordCandidates(&result, builtin, ValueCandidate{Scope: ScopeBuiltin, Identity: "baseharbor",
		Digest: PreferenceDigest(builtin), Source: "builtin.defaults"})
	applyDefaults(&result, builtin, "builtin.defaults")
	candidate := ValueCandidate{Scope: ScopeOrganization, Identity: state.Config.Organization,
		Digest: state.Resolution.ResolvedDigest, Source: "organization.defaults"}
	recordCandidates(&result, state.Config.Defaults, candidate)
	applyDefaults(&result, state.Config.Defaults, "organization.defaults")
	if env, ok := state.Config.Environments[environment]; ok {
		candidate.Source = "organization.environment." + environment
		recordCandidates(&result, env, candidate)
		applyDefaults(&result, env, "organization.environment."+environment)
	}
	// Organization-environment reconciliation (card 0428-056). The organization
	// configuration is the definition/authorship layer; the deployment registry
	// (card 0428-054, environment.json) is the authoritative runtime source of
	// truth. When the organization declares a shared-environment identity, it is
	// reconciled against the Core registry identity supplied by the caller:
	// a match marks the identity authoritative; a mismatch fails closed and
	// returns an error so a drifted identity never reaches downstream resolution.
	// An absent registry identity is treated as "no authoritative source
	// available" — reconciliation stays inert and never downgrades the
	// provider/defaults vs mandatory-policy resolution.
	if err := reconcileEnvironmentIdentity(&result, state, environment, coreEnv); err != nil {
		return Effective{}, err
	}
	if team := state.Config.Team; team != nil {
		source := "team." + team.Name
		recordCandidates(&result, team.Defaults, ValueCandidate{Scope: ScopeTeam, Identity: team.Name,
			Digest: state.Resolution.ResolvedDigest, Source: source})
		applyDefaults(&result, team.Defaults, source)
	}
	for _, layer := range layers {
		source := string(layer.Scope) + "." + layer.Identity
		recordCandidates(&result, layer.Defaults, ValueCandidate{Scope: layer.Scope, Identity: layer.Identity,
			Digest: layer.Digest, Source: source})
		applyDefaults(&result, layer.Defaults, source)
	}
	if result.Target != nil {
		name := result.Target.Value
		ref, ok := state.Config.Targets[name]
		if !ok {
			winner := result.Provenance["target"].Winner.Scope
			if winner != ScopeBuiltin && scopeOrder[winner] < scopeOrder[ScopeUser] {
				return Effective{}, fmt.Errorf("effective target %q is not declared by organization configuration", name)
			}
			ref.Reference = name
		}
		result.Target.Name = name
		result.Target.Value = strings.TrimSpace(ref.Reference)
	}
	if result.Stack != nil {
		name := result.Stack.Value
		ref, ok := state.Config.Stacks[name]
		if !ok {
			return Effective{}, fmt.Errorf("effective stack %q is not declared by organization configuration", name)
		}
		result.Stack.Name = name
		result.Stack.Value = strings.TrimSpace(ref.Reference)
	}
	for capability, provider := range result.Providers {
		ref, ok := state.Config.Providers[provider.Provider]
		if !ok {
			return Effective{}, fmt.Errorf("effective provider %q for capability %q is not declared by organization configuration", provider.Provider, capability)
		}
		provider.Reference = strings.TrimSpace(ref.Reference)
		result.Providers[capability] = provider
	}
	for i, policy := range result.Policies {
		ref, ok := state.Config.Policies[policy.Policy]
		if !ok {
			return Effective{}, fmt.Errorf("effective policy %q is not declared by organization configuration", policy.Policy)
		}
		policy.Reference = strings.TrimSpace(ref.Reference)
		result.Policies[i] = policy
	}
	if err := evaluateConstraints(&result, state.Config, state.Config.Constraints, ScopeOrganization, state.Config.Organization); err != nil {
		return result, err
	}
	// The active organization may declare a single machine operation for the
	// production policy path; validate it against the fixed domain so an
	// undeclared verb cannot widen the surface.
	if err := validateOperation(state.Config.Operation); err != nil {
		return result, err
	}
	if team := state.Config.Team; team != nil {
		if err := evaluateConstraints(&result, state.Config, team.Constraints, ScopeTeam, team.Name); err != nil {
			return result, err
		}
	}
	return result, nil
}

// ResolveEffective resolves the effective organization configuration for an
// environment. It is definition-layer-only: it reconciles provider/defaults vs
// mandatory policy but does NOT reconcile the shared environment identity with
// the Core registry. Existing call sites use this stable signature.
func ResolveEffective(state ActiveState, environment string, preferences ...PreferenceLayer) (Effective, error) {
	return resolveEffectiveInner(state, environment, EnvironmentResolutionInput{}, preferences)
}

// ResolveEffectiveWithCoreIdentity resolves the effective organization
// configuration and additionally reconciles the shared environment identity
// (card 0428-056) against the Core registry identity (card 0428-054) supplied by
// the caller. A mismatch fails closed. Callers without a registry identity in
// scope pass EnvironmentResolutionInput{} and get definition-layer resolution
// identical to ResolveEffective.
func ResolveEffectiveWithCoreIdentity(state ActiveState, environment string, coreEnv EnvironmentResolutionInput, preferences ...PreferenceLayer) (Effective, error) {
	return resolveEffectiveInner(state, environment, coreEnv, preferences)
}

// reconcileEnvironmentIdentity applies the card 0428-056 reconciliation. It is
// definition-layer-only: it never mutates the deployment registry or the
// organization configuration, only the in-memory Effective result. A declared
// identity that matches the registry identity is authoritative; a declared
// identity that diverges fails closed; an absent registry identity is inert.
func reconcileEnvironmentIdentity(result *Effective, state ActiveState, environment string, coreEnv EnvironmentResolutionInput) error {
	declared := state.Config.Environments[environment].Identity
	if declared == nil {
		return nil
	}
	registryIdentity := strings.TrimSpace(coreEnv.CoreIdentity)
	if registryIdentity == "" {
		// No authoritative registry identity in this resolution scope; the
		// declaration stands as authored but is not yet authoritative. The
		// definition layer still carries the reference for tooling/display, but
		// downstream consumers must not treat it as reconciled.
		result.EnvironmentIdentity = &EffectiveEnvironmentIdentity{
			Reference: strings.TrimSpace(declared.Reference),
			Source:    "organization.environment." + environment,
		}
		return nil
	}
	if strings.TrimSpace(declared.Reference) == "" {
		return fmt.Errorf("organization environment %q declares an empty identity; a registry identity %q is present", environment, registryIdentity)
	}
	if strings.TrimSpace(declared.Reference) != registryIdentity {
		return &machine.Error{
			Code:      machine.ErrorPolicyDenied,
			CauseCode: "environment_identity_mismatch",
			Message:   "The organization environment identity does not match the Core registry identity.",
			Resource:  "environments." + environment + ".identity.reference",
			Next:      "Reconcile the managed organization configuration with the deployment registry (environment.json) or correct the declared identity reference.",
		}
	}
	result.EnvironmentIdentity = &EffectiveEnvironmentIdentity{
		Reference:     registryIdentity,
		Authoritative: true,
		Source:        "organization.environment." + environment + " (reconciled with deployment registry)",
	}
	return nil
}

func applyDefaults(result *Effective, defaults EnvironmentDefaults, source string) {
	if value := strings.TrimSpace(defaults.Target); value != "" {
		result.Target = &EffectiveValue{Value: value, Source: source}
	}
	if value := strings.TrimSpace(defaults.Stack); value != "" {
		result.Stack = &EffectiveValue{Value: value, Source: source}
	}
	names := make([]string, 0, len(defaults.Providers))
	for name := range defaults.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := defaults.Providers[name]
		result.Providers[name] = EffectiveProvider{
			Provider: strings.TrimSpace(p.Provider),
			Scope:    strings.TrimSpace(p.Scope),
			Source:   source,
		}
	}
	trustNames := make([]string, 0, len(defaults.Trust))
	for name := range defaults.Trust {
		trustNames = append(trustNames, name)
	}
	sort.Strings(trustNames)
	for _, name := range trustNames {
		result.Trust[name] = EffectiveValue{Value: defaults.Trust[name].Reference, Source: source}
	}
	if defaults.Policies != nil {
		retained := make([]EffectivePolicy, 0, len(result.Policies))
		for _, policy := range result.Policies {
			if policy.Mandatory {
				retained = append(retained, policy)
			}
		}
		result.Policies = retained
		for _, policy := range defaults.Policies {
			alreadyMandatory := false
			for _, retained := range result.Policies {
				alreadyMandatory = alreadyMandatory || retained.Policy == strings.TrimSpace(policy.Policy)
			}
			if alreadyMandatory {
				continue
			}
			result.Policies = append(result.Policies, EffectivePolicy{
				Policy: strings.TrimSpace(policy.Policy), Mandatory: policy.Mandatory, Source: source,
			})
		}
	}
}
