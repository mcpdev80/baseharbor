package orgconfig

import (
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// orgEnvState builds a minimal active configuration with one declared
// shared-environment identity so tests can exercise the card 0428-056
// reconciliation without repeating fixtures.
func orgEnvState(identityRef string) ActiveState {
	digest := "sha256:" + strings.Repeat("b", 64)
	return ActiveState{
		Resolution: Resolution{Source: Source{Kind: SourceLocal, Location: "organization.yaml"}, ResolvedDigest: digest},
		Config: Config{
			APIVersion:   ContractVersion,
			Organization: "acme",
			Targets: map[string]Reference{
				"dev": {Reference: "dev-target"},
			},
			Environments: map[string]EnvironmentDefaults{
				"prod": {
					Target:   "dev",
					Identity: &EnvironmentIdentity{Reference: identityRef},
				},
			},
		},
	}
}

// A declared identity that matches the Core registry identity becomes
// authoritative and is surfaced in the reconciled Effective result.
func TestReconcileIdentityMatchIsAuthoritative(t *testing.T) {
	state := orgEnvState("env:prod:uuid-1")
	effective, err := ResolveEffectiveWithCoreIdentity(state, "prod", EnvironmentResolutionInput{CoreIdentity: "env:prod:uuid-1"})
	if err != nil {
		t.Fatalf("matching identity should reconcile, got %v", err)
	}
	if effective.EnvironmentIdentity == nil {
		t.Fatal("expected reconciled environment identity, got nil")
	}
	if effective.EnvironmentIdentity.Reference != "env:prod:uuid-1" {
		t.Fatalf("unexpected reference: %q", effective.EnvironmentIdentity.Reference)
	}
	if !effective.EnvironmentIdentity.Authoritative {
		t.Fatal("matching identity must be authoritative")
	}
	if !strings.Contains(effective.EnvironmentIdentity.Source, "reconciled") {
		t.Fatalf("authoritative source should attest reconciliation, got %q", effective.EnvironmentIdentity.Source)
	}
}

// A declared identity that diverges from the Core registry identity fails
// closed with a typed policy-denial error; the drifted identity never reaches
// downstream resolution.
func TestReconcileIdentityMismatchFailsClosed(t *testing.T) {
	state := orgEnvState("env:prod:uuid-1")
	_, err := ResolveEffectiveWithCoreIdentity(state, "prod", EnvironmentResolutionInput{CoreIdentity: "env:prod:uuid-2"})
	if err == nil {
		t.Fatal("expected mismatch to fail closed")
	}
	var problem *machine.Error
	if !errors.As(err, &problem) || problem.Code != machine.ErrorPolicyDenied || problem.CauseCode != "environment_identity_mismatch" {
		t.Fatalf("expected typed policy-denial identity mismatch, got %v", err)
	}
	if problem.Resource == "" || problem.Next == "" {
		t.Fatalf("identity mismatch must carry actionable resource + remediation, got %+v", problem)
	}
}

// A declared identity with an empty reference when a registry identity is
// present fails closed: the definition layer cannot omit an identity the Core
// registry has already stamped.
func TestReconcileEmptyDeclarationWithRegistryIdentityFailsClosed(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	state := ActiveState{
		Resolution: Resolution{Source: Source{Kind: SourceLocal, Location: "organization.yaml"}, ResolvedDigest: digest},
		Config: Config{
			APIVersion:   ContractVersion,
			Organization: "acme",
			Targets:      map[string]Reference{"dev": {Reference: "dev-target"}},
			Environments: map[string]EnvironmentDefaults{
				"prod": {Target: "dev", Identity: &EnvironmentIdentity{Reference: ""}},
			},
		},
	}
	_, err := ResolveEffectiveWithCoreIdentity(state, "prod", EnvironmentResolutionInput{CoreIdentity: "env:prod:uuid-1"})
	if err == nil {
		t.Fatal("empty identity declaration with registry identity present must fail closed")
	}
}

// An absent registry identity is treated as "no authoritative source
// available": the declaration stands as authored but is not authoritative, and
// the definition-layer resolution is not weakened.
func TestReconcileAbsentRegistryIdentityIsInert(t *testing.T) {
	state := orgEnvState("env:prod:uuid-1")
	effective, err := ResolveEffectiveWithCoreIdentity(state, "prod", EnvironmentResolutionInput{})
	if err != nil {
		t.Fatalf("absent registry identity should be inert, got %v", err)
	}
	if effective.EnvironmentIdentity == nil {
		t.Fatal("declared identity should still surface as authored")
	}
	if effective.EnvironmentIdentity.Authoritative {
		t.Fatal("identity without a registry witness must not be authoritative")
	}
	// The definition-layer resolution (target, providers) must still be intact.
	if effective.Target == nil || effective.Target.Value != "dev-target" {
		t.Fatalf("definition-layer resolution must not be weakened, got %+v", effective.Target)
	}
}

// The public ResolveEffective (definition-layer-only) leaves reconciliation
// inert even when an identity is declared, preserving the stable signature for
// existing call sites: it surfaces the authored identity but never marks it
// authoritative without a registry witness.
func TestResolveEffectiveWithoutCoreIdentityDoesNotReconcile(t *testing.T) {
	state := orgEnvState("env:prod:uuid-1")
	effective, err := ResolveEffective(state, "prod")
	if err != nil {
		t.Fatalf("ResolveEffective must not reconcile, got %v", err)
	}
	if effective.EnvironmentIdentity == nil {
		t.Fatal("authored identity should still surface as definition-layer input")
	}
	if effective.EnvironmentIdentity.Authoritative {
		t.Fatal("ResolveEffective must not mark an identity authoritative without a registry witness")
	}
}

// Untrusted config: a shared-environment identity carrying inline secret
// material is rejected during configuration validation, never reaching
// resolution.
func TestOrgEnvIdentitySecretMaterialRejected(t *testing.T) {
	digest := "sha256:" + strings.Repeat("d", 64)
	state := ActiveState{
		Resolution: Resolution{Source: Source{Kind: SourceLocal, Location: "organization.yaml"}, ResolvedDigest: digest},
		Config: Config{
			APIVersion:   ContractVersion,
			Organization: "acme",
			Targets:      map[string]Reference{"dev": {Reference: "dev-target"}},
			Environments: map[string]EnvironmentDefaults{
				"prod": {
					Target:   "dev",
					Policies: []PolicyDefault{{Policy: "security", Mandatory: true}},
					Identity: &EnvironmentIdentity{Reference: "env:prod?token=deadbeef"},
				},
			},
		},
	}
	if _, err := ResolveEffective(state, "prod"); err == nil {
		t.Fatal("organization env identity with inline secret material must be rejected at validation")
	}
}

// Provider/defaults vs mandatory policy stays distinct from the identity
// reconciliation: a mandatory organization policy is retained even when the
// environment identity is reconciled, and identity mismatch fails before policy
// folding.
func TestOrgEnvIdentityReconciliationDoesNotWeakenMandatoryPolicy(t *testing.T) {
	digest := "sha256:" + strings.Repeat("e", 64)
	state := ActiveState{
		Resolution: Resolution{Source: Source{Kind: SourceLocal, Location: "organization.yaml"}, ResolvedDigest: digest},
		Config: Config{
			APIVersion:   ContractVersion,
			Organization: "acme",
			Targets:      map[string]Reference{"dev": {Reference: "dev-target"}},
			Defaults:     EnvironmentDefaults{Policies: []PolicyDefault{{Policy: "security", Mandatory: true}}},
			Policies:     map[string]Reference{"security": {Reference: "policy:security"}},
			Environments: map[string]EnvironmentDefaults{
				"prod": {Target: "dev", Identity: &EnvironmentIdentity{Reference: "env:prod:uuid-1"}},
			},
		},
	}
	effective, err := ResolveEffectiveWithCoreIdentity(state, "prod", EnvironmentResolutionInput{CoreIdentity: "env:prod:uuid-1"})
	if err != nil {
		t.Fatalf("reconciliation should succeed for matching identity, got %v", err)
	}
	if len(effective.Policies) != 1 || !effective.Policies[0].Mandatory {
		t.Fatalf("mandatory organization policy must survive reconciliation, got %+v", effective.Policies)
	}
}
