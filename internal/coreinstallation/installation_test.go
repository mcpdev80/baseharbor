package coreinstallation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testSpec() Spec {
	return Spec{Target: "local", Runtime: "docker", MachineRole: Development}
}

func testSteps(order *[]string) Steps {
	step := func(name string) func(context.Context, State) error {
		return func(_ context.Context, _ State) error { *order = append(*order, name); return nil }
	}
	return Steps{Preflight: step("preflight"), SQL: step("sql"), Secrets: step("secrets"), Identity: func(_ context.Context, _ State) (string, error) {
		*order = append(*order, "identity")
		return "https://identity.localhost:8443/realms/baseharbor", nil
	}, Verify: step("verify")}
}

func TestCoreBootstrapWithoutRepositoryAndRetryPreservesIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installation")
	var order []string
	steps := testSteps(&order)
	first, err := Run(context.Background(), root, testSpec(), steps)
	if err != nil || !first.Ready {
		t.Fatalf("bootstrap: %+v %v", first, err)
	}
	want := []string{"preflight", "sql", "secrets", "identity", "verify"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order: %v", order)
	}
	order = nil
	second, err := Run(context.Background(), root, testSpec(), steps)
	if err != nil || second.ID != first.ID || !second.Ready || !reflect.DeepEqual(order, want) {
		t.Fatalf("retry duplicated or skipped verification: %+v %v %v", second, order, err)
	}
	loaded, err := Load(root)
	if err != nil || !reflect.DeepEqual(loaded, second) {
		t.Fatalf("authoritative state: %+v %v", loaded, err)
	}
	info, err := os.Stat(filepath.Join(root, "installation.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("installation state is not protected")
	}
}

func TestCorePartialFailureReconcilesSameInstallation(t *testing.T) {
	for _, stage := range []string{"sql", "secrets", "identity", "verify"} {
		t.Run(stage, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "installation")
			var order []string
			steps := testSteps(&order)
			fail := func(context.Context, State) error { return errors.New("provider unavailable") }
			switch stage {
			case "sql":
				steps.SQL = fail
			case "secrets":
				steps.Secrets = fail
			case "identity":
				steps.Identity = func(context.Context, State) (string, error) { return "", errors.New("provider unavailable") }
			case "verify":
				steps.Verify = fail
			}
			failed, err := Run(context.Background(), root, testSpec(), steps)
			if err == nil || failed.Ready || failed.Phase != stage+"_failed" {
				t.Fatalf("false readiness: %+v %v", failed, err)
			}
			stored, err := Load(root)
			if err != nil || stored.ID != failed.ID || stored.Ready {
				t.Fatalf("failure not durable: %+v %v", stored, err)
			}
			order = nil
			recovered, err := Run(context.Background(), root, testSpec(), testSteps(&order))
			if err != nil || recovered.ID != failed.ID || !recovered.Ready {
				t.Fatalf("recovery: %+v %v", recovered, err)
			}
		})
	}
}

func TestCorePreflightAndForeignResourcesPreventProvisioning(t *testing.T) {
	for _, resource := range []string{"preflight", "compose.yaml", "runtime.env", "topology.json", "installation.json"} {
		t.Run(resource, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "installation")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			var order []string
			steps := testSteps(&order)
			if resource == "preflight" {
				steps.Preflight = func(context.Context, State) error { return errors.New("insufficient resources") }
			} else if err := os.WriteFile(filepath.Join(root, resource), []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Run(context.Background(), root, testSpec(), steps); err == nil {
				t.Fatal("foreign/preflight failure accepted")
			}
			if len(order) != 0 {
				t.Fatalf("provisioned before refusal: %v", order)
			}
		})
	}
}

func TestCoreConflictAndConcurrentBootstrapFailClosed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installation")
	var order []string
	first, err := Run(context.Background(), root, testSpec(), testSteps(&order))
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []Spec{{Target: "foreign", Runtime: "docker", MachineRole: Development}, {Target: "local", Runtime: "podman", MachineRole: Development}, {Target: "local", Runtime: "docker", MachineRole: Deployment}, {Target: "local", Runtime: "docker", MachineRole: Development, HA: true}} {
		order = nil
		if _, err := Run(context.Background(), root, spec, testSteps(&order)); err == nil || len(order) != 0 {
			t.Fatal("conflicting installation provisioned")
		}
	}
	steps := testSteps(&order)
	steps.SQL = func(ctx context.Context, state State) error {
		var nested []string
		if _, err := Run(ctx, root, testSpec(), testSteps(&nested)); err == nil || len(nested) != 0 {
			t.Fatal("concurrent bootstrap accepted")
		}
		return nil
	}
	second, err := Run(context.Background(), root, testSpec(), steps)
	if err != nil || second.ID != first.ID {
		t.Fatalf("outer reconciliation: %+v %v", second, err)
	}
}

func TestCoreRoleDefaultsDoNotRemoveCapabilitiesOrTLS(t *testing.T) {
	for _, role := range []MachineRole{Development, Deployment} {
		var order []string
		spec := testSpec()
		spec.MachineRole = role
		state, err := Run(context.Background(), filepath.Join(t.TempDir(), "installation"), spec, testSteps(&order))
		if err != nil || !state.Ready || len(state.Capabilities) != 3 || state.Defaults.LocalWorkspaces != (role == Development) || state.Defaults.ExplicitSources != (role == Deployment) {
			t.Fatalf("role changed Core requirements: %+v %v", state, err)
		}
	}
	var order []string
	steps := testSteps(&order)
	steps.Identity = func(context.Context, State) (string, error) { return "http://localhost:8080/realms/baseharbor", nil }
	state, err := Run(context.Background(), filepath.Join(t.TempDir(), "installation"), testSpec(), steps)
	if err == nil || state.Ready || state.Phase != "identity_failed" {
		t.Fatal("plaintext Core Identity accepted")
	}
}
