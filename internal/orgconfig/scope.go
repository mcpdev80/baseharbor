package orgconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

type ScopeKind string

const (
	ScopeBuiltin      ScopeKind = "builtin"
	ScopeOrganization ScopeKind = "organization"
	ScopeTeam         ScopeKind = "team"
	ScopeUser         ScopeKind = "user"
	ScopeRepository   ScopeKind = "repository"
	ScopeInvocation   ScopeKind = "invocation"
)

var scopeOrder = map[ScopeKind]int{
	ScopeBuiltin: 0, ScopeOrganization: 1, ScopeTeam: 2,
	ScopeUser: 3, ScopeRepository: 4, ScopeInvocation: 5,
}

// PreferenceLayer is lower-trust input. Organization and team policy can only
// come from the active managed configuration, never from an invocation layer.
type PreferenceLayer struct {
	Scope    ScopeKind           `json:"scope" yaml:"scope"`
	Identity string              `json:"identity" yaml:"identity"`
	Digest   string              `json:"digest,omitempty" yaml:"digest,omitempty"`
	Defaults EnvironmentDefaults `json:"defaults" yaml:"defaults"`
}

type Constraint struct {
	Field   string   `json:"field" yaml:"field"`
	Allowed []string `json:"allowed" yaml:"allowed"`
}

type TeamConfiguration struct {
	Name        string              `json:"name" yaml:"name"`
	Defaults    EnvironmentDefaults `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Constraints []Constraint        `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

type ValueCandidate struct {
	Scope    ScopeKind `json:"scope"`
	Identity string    `json:"identity"`
	Digest   string    `json:"digest,omitempty"`
	Source   string    `json:"source"`
	Value    string    `json:"value"`
}

type ValueProvenance struct {
	Kind       string           `json:"kind"`
	Winner     ValueCandidate   `json:"winner"`
	Overridden []ValueCandidate `json:"overridden,omitempty"`
}

type ConstraintDecision struct {
	Field    string    `json:"field"`
	Scope    ScopeKind `json:"scope"`
	Identity string    `json:"identity"`
	Allowed  bool      `json:"allowed"`
}

func validateConstraints(constraints []Constraint) error {
	seen := map[string]bool{}
	for _, constraint := range constraints {
		if !selectionField(constraint.Field) || seen[constraint.Field] || len(constraint.Allowed) == 0 {
			return fmt.Errorf("configuration constraint requires a unique supported field and nonempty allowed set")
		}
		seen[constraint.Field] = true
		values := map[string]bool{}
		for _, value := range constraint.Allowed {
			if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || containsSecretMaterial(value) || values[value] {
				return fmt.Errorf("configuration constraint contains an invalid or duplicate allowed reference")
			}
			values[value] = true
		}
	}
	return nil
}

func selectionField(field string) bool {
	if field == "target" || field == "stack" {
		return true
	}
	for _, prefix := range []string{"provider.", "trust."} {
		if strings.HasPrefix(field, prefix) && strings.TrimSpace(strings.TrimPrefix(field, prefix)) != "" {
			return true
		}
	}
	return false
}

func orderedPreferences(layers []PreferenceLayer) ([]PreferenceLayer, error) {
	ordered := append([]PreferenceLayer(nil), layers...)
	seen := map[ScopeKind]bool{}
	for i := range ordered {
		layer := &ordered[i]
		if layer.Scope != ScopeUser && layer.Scope != ScopeRepository && layer.Scope != ScopeInvocation {
			return nil, machine.NewError(machine.ErrorPolicyDenied, "Request preferences cannot supply managed scopes.", "Configure organization/team constraints through the authorized managed configuration source.", false)
		}
		if seen[layer.Scope] || strings.TrimSpace(layer.Identity) == "" || containsSecretMaterial(layer.Identity) {
			return nil, fmt.Errorf("preference scope requires one non-secret source identity")
		}
		seen[layer.Scope] = true
		actualDigest := PreferenceDigest(layer.Defaults)
		if layer.Scope == ScopeInvocation && layer.Digest == "" {
			layer.Digest = actualDigest
		}
		if !digestPattern.MatchString(layer.Digest) || layer.Digest != actualDigest {
			return nil, fmt.Errorf("preferences require a matching immutable canonical-input digest")
		}
		if len(layer.Defaults.Policies) != 0 {
			return nil, machine.NewError(machine.ErrorPolicyDenied, "Request preferences cannot replace policy references.", "Keep policy in the managed organization/team source.", false)
		}
		if err := validateDefaults(string(layer.Scope), layer.Defaults); err != nil {
			return nil, err
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return scopeOrder[ordered[i].Scope] < scopeOrder[ordered[j].Scope] })
	return ordered, nil
}

// PreferenceDigest binds provenance to the canonical, non-secret typed defaults.
func PreferenceDigest(defaults EnvironmentDefaults) string {
	data, _ := json.Marshal(defaults)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func recordCandidates(result *Effective, defaults EnvironmentDefaults, candidate ValueCandidate) {
	values := map[string]string{}
	if defaults.Target != "" {
		values["target"] = strings.TrimSpace(defaults.Target)
	}
	if defaults.Stack != "" {
		values["stack"] = strings.TrimSpace(defaults.Stack)
	}
	for name, provider := range defaults.Providers {
		values["provider."+name] = provider.Provider
	}
	for name, trust := range defaults.Trust {
		values["trust."+name] = trust.Reference
	}
	for field, value := range values {
		provenance := result.Provenance[field]
		if provenance.Kind != "" {
			provenance.Overridden = append(provenance.Overridden, provenance.Winner)
		}
		candidate.Value = value
		provenance.Kind, provenance.Winner = "preference", candidate
		result.Provenance[field] = provenance
	}
}

func evaluateConstraints(result *Effective, config Config, constraints []Constraint, scope ScopeKind, identity string) error {
	for _, constraint := range constraints {
		provenance, exists := result.Provenance[constraint.Field]
		allowed := false
		if exists {
			for _, value := range constraint.Allowed {
				allowed = allowed || canonicalSelection(config, constraint.Field, provenance.Winner.Value) == canonicalSelection(config, constraint.Field, value)
			}
		}
		result.PolicyDecisions = append(result.PolicyDecisions, ConstraintDecision{
			Field: constraint.Field, Scope: scope, Identity: identity, Allowed: allowed,
		})
		if !allowed {
			return &machine.Error{
				Code: machine.ErrorPolicyDenied, CauseCode: "configuration_constraint_denied",
				Message:  "The selected realization violates a mandatory configuration constraint.",
				Resource: constraint.Field, Next: "Select an allowed reference or ask the organization/team operator to change its managed constraint.",
			}
		}
	}
	return nil
}

func canonicalSelection(config Config, field, value string) string {
	var references map[string]Reference
	switch {
	case field == "target":
		references = config.Targets
	case field == "stack":
		references = config.Stacks
	case strings.HasPrefix(field, "provider."):
		references = config.Providers
	}
	if reference, found := references[value]; found {
		return strings.TrimSpace(reference.Reference)
	}
	return strings.TrimSpace(value)
}
