package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

const ProviderImpactScopeVersion = "baseharbor.provider-impact-scope/v1"

// ProviderScopeOwner is an observed existing registration owner. It deliberately
// has no Target: a global registration is not a fictitious runtime deployment.
// Revision binds the actual owner content; references contain no secret values.
type ProviderScopeOwner struct {
	Owner     string `json:"owner"`
	Reference string `json:"reference"`
	Subject   string `json:"subject"`
	Revision  string `json:"revision"`
	// Destruction of the scope owner itself (for example installation state)
	// needs separate data consent even when no provider leaf exists.
	Destructive bool `json:"destructive,omitempty"`
}

// ProviderImpactScope composes the SAME per-Target impact plans. It performs no
// resolution, discovery, persistence or execution. InventoryComplete requires
// the adapter to account for all authoritative target/deployment/management
// owners, including those absent from current user configuration.
type ProviderImpactScope struct {
	Version               string               `json:"version"`
	Owner                 ProviderScopeOwner   `json:"owner"`
	PlanID                string               `json:"plan_id"`
	Complete              bool                 `json:"complete"`
	Attention             string               `json:"attention,omitempty"`
	ConfirmationsRequired int                  `json:"confirmations_required"`
	Destructive           bool                 `json:"destructive"`
	Targets               []ProviderImpactPlan `json:"targets"`
}

func ComposeProviderImpactScope(owner ProviderScopeOwner, plans []ProviderImpactPlan, inventoryComplete bool) (ProviderImpactScope, error) {
	s := ProviderImpactScope{Version: ProviderImpactScopeVersion, Owner: owner, Complete: inventoryComplete, Destructive: owner.Destructive, ConfirmationsRequired: 1, Targets: append([]ProviderImpactPlan{}, plans...)}
	if owner.Owner == "" || owner.Reference == "" || owner.Subject == "" || owner.Revision == "" {
		return s, &ProviderChangeError{Code: "invalid-impact-scope-owner"}
	}
	sort.Slice(s.Targets, func(i, j int) bool { return s.Targets[i].Target < s.Targets[j].Target })
	seen := map[string]bool{}
	for _, p := range s.Targets {
		if p.Target == "" || seen[p.Target] || validateProviderConsentRequirements(p.Version, p.PlanID, p.ConfirmationsRequired) != nil {
			return s, &ProviderChangeError{Code: "invalid-impact-scope-target"}
		}
		seen[p.Target] = true
		s.Complete = s.Complete && p.Complete
		s.Destructive = s.Destructive || p.Destructive
		if p.ConfirmationsRequired == 2 || p.Attention != "" {
			s.ConfirmationsRequired = 2
		}
	}
	if !s.Complete || s.Destructive || s.ConfirmationsRequired == 2 {
		s.Attention = "Achtung"
		s.ConfirmationsRequired = 2
	}
	if err := validateScopeStrings(s); err != nil {
		return s, err
	}
	stable := s
	stable.Targets = make([]ProviderImpactPlan, len(s.Targets))
	for index, p := range s.Targets {
		stable.Targets[index] = stableValidityImpact(p)
	}
	raw, err := json.Marshal(stable)
	if err != nil {
		return s, err
	}
	digest := sha256.Sum256(raw)
	s.PlanID = hex.EncodeToString(digest[:])
	return s, nil
}

func validateScopeStrings(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return err
	}
	return validateImpactStrings(tree)
}

// ValidateProviderImpactScopeConsent uses the same staged consent requirements
// as a single Target. A leaf plan's approval cannot authorize global removal.
func ValidateProviderImpactScopeConsent(scope ProviderImpactScope, consent ProviderChangeConsent) error {
	if scope.Version != ProviderImpactScopeVersion {
		return &ProviderChangeError{Code: "invalid-impact-scope"}
	}
	current, err := ComposeProviderImpactScope(scope.Owner, scope.Targets, scope.Complete)
	if err != nil || current.PlanID != scope.PlanID || current.ConfirmationsRequired != scope.ConfirmationsRequired || current.Destructive != scope.Destructive || current.Attention != scope.Attention {
		return &ProviderChangeError{Code: "invalid-impact-scope"}
	}
	return validateProviderConsent(scope.PlanID, scope.ConfirmationsRequired, scope.Destructive, consent)
}
