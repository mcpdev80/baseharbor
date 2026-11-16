package deployment

import (
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
)

// TestSaveDeploymentRecordRefusesDuplicateApplicationName proves the registry
// refuses a second registration of the same (target, application name,
// environment) triple under a DIFFERENT deployment uuid (0427-149 BF-02). The
// two records carry the identical name+environment but different app identity,
// which is exactly the silent-duplicate path the CLI used to open.
func TestSaveDeploymentRecordRefusesDuplicateApplicationName(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	first := testDeploymentIdentity(t, "default", "dup", "dev")
	firstRecord := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: first,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "created"},
	}
	if err := SaveDeploymentRecord(firstRecord); err != nil {
		t.Fatalf("seed first record: %v", err)
	}

	// Same name + same environment, but a DIFFERENT deployment uuid (a fresh
	// registration of a colliding app). Must fail closed.
	second, err := NewDeploymentIdentity("default", "22222222-2222-4222-8222-222222222222", "dup", "dev")
	if err != nil {
		t.Fatalf("build second identity: %v", err)
	}
	secondRecord := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: second,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "created"},
	}
	savedErr := SaveDeploymentRecord(secondRecord)
	if savedErr == nil {
		t.Fatalf("expected duplicate name refusal, got success")
	}
	if !errors.Is(savedErr, ErrDuplicateApplicationName) {
		t.Fatalf("expected ErrDuplicateApplicationName, got %v", savedErr)
	}
	if !strings.Contains(savedErr.Error(), first.DeploymentID) {
		t.Fatalf("message must name the existing deployment %q: %v", first.DeploymentID, savedErr)
	}
	if !strings.Contains(savedErr.Error(), "dev") {
		t.Fatalf("message must name the colliding environment: %v", savedErr)
	}
}

// TestSaveDeploymentRecordAllowsDistinctEnvironmentConflict proves the guard
// only refuses the SAME name+environment triple. Two deployments of the same
// name on DIFFERENT environments are legitimate and must both save.
func TestSaveDeploymentRecordAllowsDistinctEnvironmentConflict(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	dev := testDeploymentIdentity(t, "default", "dup", "dev")
	devRecord := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: dev,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "created"},
	}
	if err := SaveDeploymentRecord(devRecord); err != nil {
		t.Fatalf("seed dev record: %v", err)
	}

	prod, err := NewDeploymentIdentity("default", "33333333-3333-4333-8333-333333333333", "dup", "prod")
	if err != nil {
		t.Fatalf("build prod identity: %v", err)
	}
	prodRecord := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: prod,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "created"},
	}
	if err := SaveDeploymentRecord(prodRecord); err != nil {
		t.Fatalf("distinct environment must be allowed, got %v", err)
	}
}

// TestSaveDeploymentRecordIdempotentUpdateSameIdentity proves that re-saving a
// record of the SAME identity (same uuid) is not a collision. This guards the
// apply-retry / preference-retention path against the new guard.
func TestSaveDeploymentRecordIdempotentUpdateSameIdentity(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	id := testDeploymentIdentity(t, "default", "dup", "dev")
	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	ready := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(ready); err != nil {
		t.Fatalf("idempotent update of same identity must pass, got %v", err)
	}
}
