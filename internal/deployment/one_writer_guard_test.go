package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
)

func TestOneWriterGuardPositiveDirectPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed direct record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("direct apply under persisted baseharbor owner must pass: %v", err)
	}
}

func TestOneWriterGuardPositiveDelegatedPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

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
		t.Fatalf("seed delegated record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("delegated apply under persisted external owner must pass: %v", err)
	}
}

// TestAttackerEmptyOwnerDoesNotMutateExternalProof proves that a caller forging an
// empty Observed.Owner cannot erase the persisted external ownership. The guard
// reads the authoritative persisted owner before mutating.
func TestAttackerEmptyOwnerDoesNotMutateExternalProof(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

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
		t.Fatalf("seed external record: %v", err)
	}

	recordPath := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")

	// Attacker submits a direct (BaseHarbor) apply with an EMPTY owner. The
	// persisted external owner must win and the mutation must be refused.
	attacker := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: ""},
	}
	if err := SaveDeploymentRecord(attacker); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("empty owner under persisted external owner must refuse with ErrUnauthorizedMutation, got %v", err)
	}
	if _, statErr := os.Stat(recordPath); statErr != nil {
		t.Fatalf("store must not have been mutated: %v", statErr)
	}

	// Attacker submits a direct apply with a FORGED baseharbor owner. The
	// persisted external owner still governs.
	attacker2 := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(attacker2); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("forged baseharbor owner under persisted external owner must refuse, got %v", err)
	}
	if _, statErr := os.Stat(recordPath); statErr != nil {
		t.Fatalf("store must still not have been mutated: %v", statErr)
	}
}

// TestAttackerForgedBaseharborOwnerCannotMasquerade proves that a direct apply
// whose caller forges Owner=baseharbor is accepted only because the persisted
// baseharbor owner genuinely governs — the caller value cannot invent a direct
// apply over a persisted external owner. Seeded external, caller-forged
// baseharbor: the persisted external owner wins and the mutation refuses.
func TestAttackerForgedBaseharborOwnerCannotMasquerade(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

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
		t.Fatalf("seed external record: %v", err)
	}

	recordPath := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")

	// Attacker submits a direct apply with a forged Owner=baseharbor. The
	// persisted external owner governs and refuses the direct mutation.
	attacker := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(attacker); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("forged baseharbor owner over persisted external owner must refuse with ErrUnauthorizedMutation, got %v", err)
	}
	if _, statErr := os.Stat(recordPath); statErr != nil {
		t.Fatalf("store must not have been mutated: %v", statErr)
	}
}

// TestPersistedDirectOwnerPassesDirectApply is the alt-gap guard: a prior direct
// (baseharbor) owner plus a caller-supplied conflicting observed value must not
// falsely fail — the persisted direct value governs.
func TestPersistedDirectOwnerPassesDirectApply(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed direct record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("direct apply under persisted baseharbor owner must pass: %v", err)
	}
}
