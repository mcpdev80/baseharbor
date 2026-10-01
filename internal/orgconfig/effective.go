package orgconfig

import (
	"fmt"
	"sort"
	"strings"
)

type EffectiveValue struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

type EffectiveProvider struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope,omitempty"`
	Source   string `json:"source"`
}

type Effective struct {
	ContractVersion string                       `json:"contract_version"`
	Organization    string                       `json:"organization"`
	Environment     string                       `json:"environment"`
	Target          *EffectiveValue              `json:"target,omitempty"`
	Stack           *EffectiveValue              `json:"stack,omitempty"`
	Providers       map[string]EffectiveProvider `json:"providers,omitempty"`
	Trust           map[string]EffectiveValue    `json:"trust,omitempty"`
	Policies        []EffectiveValue             `json:"policies,omitempty"`
	Resolution      Resolution                   `json:"resolution"`
}

func ResolveEffective(state ActiveState, environment string) (Effective, error) {
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
	}
	applyDefaults(&result, state.Config.Defaults, "organization.defaults")
	if env, ok := state.Config.Environments[environment]; ok {
		applyDefaults(&result, env, "organization.environment."+environment)
	}
	if result.Target != nil {
		if _, ok := state.Config.Targets[result.Target.Value]; !ok {
			return Effective{}, fmt.Errorf("effective target %q is not declared by organization configuration", result.Target.Value)
		}
	}
	if result.Stack != nil {
		if _, ok := state.Config.Stacks[result.Stack.Value]; !ok {
			return Effective{}, fmt.Errorf("effective stack %q is not declared by organization configuration", result.Stack.Value)
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
		result.Policies = result.Policies[:0]
		for _, policy := range defaults.Policies {
			result.Policies = append(result.Policies, EffectiveValue{Value: policy.Reference, Source: source})
		}
	}
}
