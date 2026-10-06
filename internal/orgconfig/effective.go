package orgconfig

import (
	"fmt"
	"sort"
	"strings"
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

type Effective struct {
	ContractVersion string                       `json:"contract_version"`
	Organization    string                       `json:"organization"`
	Environment     string                       `json:"environment"`
	Target          *EffectiveValue              `json:"target,omitempty"`
	Stack           *EffectiveValue              `json:"stack,omitempty"`
	Providers       map[string]EffectiveProvider `json:"providers,omitempty"`
	Trust           map[string]EffectiveValue    `json:"trust,omitempty"`
	Policies        []EffectivePolicy            `json:"policies,omitempty"`
	Resolution      Resolution                   `json:"resolution"`
	Provenance      map[string]ValueProvenance   `json:"provenance,omitempty"`
	PolicyDecisions []ConstraintDecision         `json:"policy_decisions,omitempty"`
}

func ResolveEffective(state ActiveState, environment string, preferences ...PreferenceLayer) (Effective, error) {
	if err := state.Config.Validate(); err != nil {
		return Effective{}, err
	}
	if err := state.Resolution.Validate(); err != nil {
		return Effective{}, err
	}
	layers, err := orderedPreferences(preferences)
	if err != nil {
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
	if team := state.Config.Team; team != nil {
		if err := evaluateConstraints(&result, state.Config, team.Constraints, ScopeTeam, team.Name); err != nil {
			return result, err
		}
	}
	return result, nil
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
