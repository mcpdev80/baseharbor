package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// writeDeploymentRecordDirect persists a deployment record straight to its
// deployment.json on disk, bypassing SaveDeploymentRecord's duplicate-name
// guard. It reproduces a pre-existing duplicate that was registered before the
// guard existed (0427-149): the guard only fires on a brand-new deployment
// directory, so a directory written out of band survives.
func writeDeploymentRecordDirect(t *testing.T, record deployment.DeploymentRecord) {
	t.Helper()
	root, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// registerTwoCollidingDeployments writes two deployment records with the same
// name+environment but different deployment uuids into a freshly seeded target,
// reproducing the pre-fix duplicate-registration state (0427-149 BF-01/BF-05).
func registerTwoCollidingDeployments(t *testing.T) (first, second deployment.DeploymentIdentity) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := deployment.TargetStateRoot(target.Name); err != nil {
		t.Fatal(err)
	}

	first, idErr := deployment.NewDeploymentIdentity(target.Name, "11111111-1111-4111-8111-111111111111", "dup", "dev")
	if idErr != nil {
		t.Fatal(idErr)
	}
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: first,
		Source:   deployment.DeploymentSource{Kind: "managed"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct(), Intent: json.RawMessage(`{"version":1,"name":"dup","environment":"dev","services":{"sql":true}}`)},
		Observed: deployment.ObservedDeployment{State: "created"},
	})

	second, idErr = deployment.NewDeploymentIdentity(target.Name, "22222222-2222-4222-8222-222222222222", "dup", "dev")
	if idErr != nil {
		t.Fatal(idErr)
	}
	writeDeploymentRecordDirect(t, deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: second,
		Source:   deployment.DeploymentSource{Kind: "managed"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct(), Intent: json.RawMessage(`{"version":1,"name":"dup","environment":"dev","services":{"sql":true}}`)},
		Observed: deployment.ObservedDeployment{State: "created"},
	})
	return first, second
}

// TestResolveRegisteredApplicationAmbiguousNameSurfacesUUID proves that a
// name-colliding deployment (pre-existing duplicate) now resolves to an
// actionable ambiguity error naming the deployment uuids, rather than the
// former opaque "environment selection required" that no -e value can fix
// (0427-149 BF-01/BF-10).
func TestResolveRegisteredApplicationAmbiguousNameSurfacesUUID(t *testing.T) {
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}

	first, _ := registerTwoCollidingDeployments(t)

	_, err = resolveRegisteredApplication(t.Context(), target, targetRoot, "dup", "", "show", "")
	if err == nil {
		t.Fatal("expected ambiguous name error for colliding deployment")
	}
	var typed *machine.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected machine error, got %T: %v", err, err)
	}
	if typed.Code != machine.ErrorOwnershipAmbiguous {
		t.Fatalf("unexpected code %q: %#v", typed.Code, typed)
	}
	if typed.CauseCode != "application_name_ambiguous" {
		t.Fatalf("unexpected cause %q: %#v", typed.CauseCode, typed)
	}
	if typed.Next == "" || !strings.Contains(typed.Next, "--uuid") {
		t.Fatalf("remediation must name the uuid path: %q", typed.Next)
	}
	if !strings.Contains(typed.Error(), first.DeploymentID) {
		t.Fatalf("error must name the existing deployment %q: %v", first.DeploymentID, typed)
	}
}

// TestResolveRegisteredApplicationByUUIDResolvesUnambiguously proves the --uuid
// selector disambiguates a name-colliding deployment onto its exact identity,
// the actionable alternative the ambiguity error points to (0427-149).
func TestResolveRegisteredApplicationByUUIDResolvesUnambiguously(t *testing.T) {
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	targetRoot, _ := deployment.TargetStateRoot(target.Name)

	first, second := registerTwoCollidingDeployments(t)

	// --uuid <id> resolves the exact deployment, bypassing the name filter.
	resolved, err := resolveRegisteredApplication(t.Context(), target, targetRoot, "dup", "", "show", first.DeploymentID)
	if err != nil {
		t.Fatalf("--uuid resolution failed: %v", err)
	}
	if resolved.DeploymentIdentity.DeploymentID != first.DeploymentID {
		t.Fatalf("--uuid resolved %q, expected %q", resolved.DeploymentIdentity.DeploymentID, first.DeploymentID)
	}

	// --uuid <other-id> resolves the other colliding record.
	resolved2, err := resolveRegisteredApplication(t.Context(), target, targetRoot, "dup", "", "show", second.DeploymentID)
	if err != nil {
		t.Fatalf("--uuid resolution for second failed: %v", err)
	}
	if resolved2.DeploymentIdentity.DeploymentID != second.DeploymentID {
		t.Fatalf("--uuid resolved %q, expected %q", resolved2.DeploymentIdentity.DeploymentID, second.DeploymentID)
	}
}

// TestResolveRegisteredApplicationUUIDNotFound proves a --uuid with no matching
// deployment fails closed with a not-found machine error naming the id.
func TestResolveRegisteredApplicationUUIDNotFound(t *testing.T) {
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	targetRoot, _ := deployment.TargetStateRoot(target.Name)

	registerTwoCollidingDeployments(t)

	_, err := resolveRegisteredApplication(t.Context(), target, targetRoot, "dup", "", "show", "00000000-0000-4000-8000-000000000000")
	if err == nil {
		t.Fatal("expected not-found error for unknown --uuid")
	}
	var typed *machine.Error
	if !errors.As(err, &typed) || typed.Code != machine.ErrorNotFound {
		t.Fatalf("expected not-found machine error, got %#v", err)
	}
}
