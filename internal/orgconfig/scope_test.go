package orgconfig

import (
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func scopeState() ActiveState {
	digest := "sha256:" + strings.Repeat("a", 64)
	return ActiveState{
		Resolution: Resolution{Source: Source{Kind: SourceLocal, Location: "organization.yaml"}, ResolvedDigest: digest},
		Config: Config{
			APIVersion: ContractVersion, Organization: "acme",
			Targets: map[string]Reference{
				"org": {Reference: "org-target"}, "team": {Reference: "team-target"},
				"user": {Reference: "user-target"}, "repo": {Reference: "repo-target"},
				"explicit": {Reference: "explicit-target"},
			},
			Defaults: EnvironmentDefaults{Target: "org", Policies: []PolicyDefault{{Policy: "security", Mandatory: true}}},
			Policies: map[string]Reference{"security": {Reference: "policy:security"}},
		},
	}
}

func preference(scope ScopeKind, target string) PreferenceLayer {
	defaults := EnvironmentDefaults{Target: target}
	return PreferenceLayer{Scope: scope, Identity: string(scope), Digest: PreferenceDigest(defaults), Defaults: defaults}
}

func TestEveryPreferenceBoundaryAndDeterministicOrder(t *testing.T) {
	state := scopeState()
	check := func(state ActiveState, want ScopeKind, target string, layers ...PreferenceLayer) {
		t.Helper()
		result, err := ResolveEffective(state, "dev", layers...)
		if err != nil {
			t.Fatal(err)
		}
		if result.Target.Name != target || result.Provenance["target"].Winner.Scope != want {
			t.Fatalf("wrong winning target/scope: %+v", result)
		}
	}
	check(state, ScopeOrganization, "org")
	noDefault := scopeState()
	noDefault.Config.Defaults.Target = ""
	check(noDefault, ScopeBuiltin, "local")
	state.Config.Team = &TeamConfiguration{Name: "platform", Defaults: EnvironmentDefaults{Target: "team"}}
	check(state, ScopeTeam, "team")
	user, repo, explicit := preference(ScopeUser, "user"), preference(ScopeRepository, "repo"), preference(ScopeInvocation, "explicit")
	check(state, ScopeUser, "user", user)
	check(state, ScopeRepository, "repo", repo, user)
	check(state, ScopeInvocation, "explicit", explicit, repo, user)
	result, err := ResolveEffective(state, "dev", explicit, user, repo)
	if err != nil {
		t.Fatal(err)
	}
	history := result.Provenance["target"].Overridden
	if len(history) != 5 || history[0].Scope != ScopeBuiltin || history[1].Scope != ScopeOrganization || history[2].Scope != ScopeTeam || history[3].Scope != ScopeUser || history[4].Scope != ScopeRepository {
		t.Fatalf("non-deterministic provenance: %+v", history)
	}
	if state.Config.Defaults.Target != "org" {
		t.Fatal("resolution mutated portable/default input")
	}
}

func TestMandatoryPoliciesSurviveEmptyReplacementAndDowngrade(t *testing.T) {
	for _, policies := range [][]PolicyDefault{nil, {}, {{Policy: "security", Mandatory: false}}} {
		state := scopeState()
		state.Config.Environments = map[string]EnvironmentDefaults{"prod": {Policies: policies}}
		state.Config.Team = &TeamConfiguration{Name: "team", Defaults: EnvironmentDefaults{Policies: []PolicyDefault{}}}
		result, err := ResolveEffective(state, "prod")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Policies) != 1 || !result.Policies[0].Mandatory || result.Policies[0].Source != "organization.defaults" {
			t.Fatalf("mandatory organization policy replaced: %+v", result.Policies)
		}
	}
}

func TestOrganizationAndTeamPolicyConstrainExplicitOverride(t *testing.T) {
	state := scopeState()
	state.Config.Constraints = []Constraint{{Field: "target", Allowed: []string{"org", "explicit"}}}
	state.Config.Team = &TeamConfiguration{Name: "platform", Constraints: []Constraint{{Field: "target", Allowed: []string{"org"}}}}
	result, err := ResolveEffective(state, "dev", preference(ScopeInvocation, "explicit"))
	var problem *machine.Error
	if !errors.As(err, &problem) || problem.Code != machine.ErrorPolicyDenied || problem.CauseCode != "configuration_constraint_denied" || problem.Next == "" {
		t.Fatalf("missing typed actionable denial: %v", err)
	}
	if len(result.PolicyDecisions) != 2 || !result.PolicyDecisions[0].Allowed || result.PolicyDecisions[1].Allowed {
		t.Fatalf("constraints did not intersect: %+v", result.PolicyDecisions)
	}
	if _, err := ResolveEffective(state, "dev"); err != nil {
		t.Fatal("permitted selection denied:", err)
	}
	state.Config.Defaults.Target = ""
	if _, err := ResolveEffective(state, "dev"); err == nil {
		t.Fatal("absence bypassed mandatory target constraint")
	}
}

func TestPreferenceTrustAndSecretSafety(t *testing.T) {
	state := scopeState()
	for _, scope := range []ScopeKind{ScopeBuiltin, ScopeOrganization, ScopeTeam, "unknown"} {
		if _, err := ResolveEffective(state, "dev", preference(scope, "org")); err == nil {
			t.Fatalf("request injected scope %s", scope)
		}
	}
	user := preference(ScopeUser, "user")
	if _, err := ResolveEffective(state, "dev", user, user); err == nil {
		t.Fatal("conflicting duplicate scope accepted")
	}
	user.Digest = "latest"
	if _, err := ResolveEffective(state, "dev", user); err == nil {
		t.Fatal("mutable source identity accepted")
	}
	user = preference(ScopeUser, "user")
	user.Defaults.Policies = []PolicyDefault{{Policy: "security"}}
	if _, err := ResolveEffective(state, "dev", user); err == nil {
		t.Fatal("lower-trust policy injection accepted")
	}
	for _, value := range []string{"https://user:password@example.org/org", "https://example.org/org?access_token=secret", "-----BEGIN PRIVATE KEY-----"} {
		state := scopeState()
		state.Resolution.Source.Location = value
		if _, err := ResolveEffective(state, "dev"); err == nil || strings.Contains(err.Error(), value) {
			t.Fatal("credential source was accepted or leaked")
		}
	}
}

func TestAbsentAndEmptyPreferenceAreNoSelectionNotDeletion(t *testing.T) {
	state := scopeState()
	result, err := ResolveEffective(state, "dev", preference(ScopeInvocation, ""))
	if err != nil || result.Target.Name != "org" {
		t.Fatalf("empty preference deleted default: %+v %v", result, err)
	}
	state.Config.Constraints = []Constraint{{Field: "unknown", Allowed: []string{"x"}}}
	if _, err := ResolveEffective(state, "dev"); err == nil {
		t.Fatal("unknown constrained field accepted")
	}
}
