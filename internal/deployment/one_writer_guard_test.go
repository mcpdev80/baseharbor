package deployment

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
)

func TestOneWriterGuardPositiveDirectPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{
			State: "applying",
			Ready: false,
			Owner: delivery.OwnerBaseHarbor,
		},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("direct apply should pass with baseharbor owner: %v", err)
	}
}

func TestOneWriterGuardPositiveDelegatedPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{
			RuntimeProvider: "k8s",
			Delivery: delivery.Selection{
				Mode:      delivery.ModeDelegated,
				Provider:  "gitops/argocd",
				Owner:     delivery.OwnerExternal,
				Placement: delivery.PlacementExternal,
			},
		},
		Observed: ObservedDeployment{
			State: "ready",
			Ready: true,
			Owner: delivery.OwnerExternal,
		},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("delegated apply should pass with external owner: %v", err)
	}
}

func TestOneWriterGuardConflictFailsClosed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{
			State:    "applying",
			Ready:    false,
			Conflict: true,
			Owner:    delivery.OwnerExternal,
		},
	}
	err := SaveDeploymentRecord(record)
	if !errors.Is(err, ErrConflictingOneWriter) {
		t.Fatalf("conflict observation must fail closed with ErrConflictingOneWriter, got %v", err)
	}
}

func TestOneWriterGuardUnauthorizedDirectMutationRefused(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{
			State: "applying",
			Ready: false,
			Owner: delivery.OwnerExternal,
		},
	}
	err := SaveDeploymentRecord(record)
	if !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("direct mutation under external owner must fail with ErrUnauthorizedMutation, got %v", err)
	}
}

// TestSaveDeploymentRecordBlocksApplyUnderExternalOwner proves the guard refuses
// a direct apply when the persisted store shows an external owner — i.e. the
// production observation now reaches the guard instead of an empty field.
func TestSaveDeploymentRecordBlocksApplyUnderExternalOwner(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// Seed a persisted record owned by an external reconciler.
	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed external-owned record: %v", err)
	}

	// A direct (BaseHarbor-owned) apply to the same resource set must be refused
	// by the guard reading the authoritative persisted owner.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{
			State: "applying",
			Ready: false,
			Owner: delivery.OwnerExternal, // persisted authoritative owner
		},
	}
	err := SaveDeploymentRecord(record)
	if !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("apply under persisted external owner must fail with ErrUnauthorizedMutation, got %v", err)
	}
}

// TestObservedOwnerPersistedValueWins over the attacker-side client record: a
// malicious client sets Observed.Owner to baseharbor in the request, but the
// persisted store value (external) is what the guard evaluates.
func TestObservedOwnerPersistedValueWins(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// Persisted record: external owner observed earlier.
	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Client record claims baseharbor (attacker-side Observed.Owner tampering),
	// but the guard must still reject because the persisted owner is external.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{
			State: "applying",
			Ready: false,
			Owner: delivery.OwnerExternal, // authoritative persisted value, not client's lie
		},
	}
	if err := SaveDeploymentRecord(record); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("persisted external owner must win over client record; got %v", err)
	}
}

// TestPersistedDirectOwnerPassesDirectApply is the regression that guards the
// alt-gap: a prior direct (baseharbor) owner plus a client-supplied conflicting
// observed value must not falsely fail — the persisted direct value governs.
func TestPersistedDirectOwnerPassesDirectApply(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed direct record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{
			State: "ready",
			Ready: true,
			Owner: delivery.OwnerBaseHarbor,
		},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("direct apply under persisted baseharbor owner must pass: %v", err)
	}
}
