//go:build baseharbor_ci_operator_testonly

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestCIIntegrationOperatorIsLimitedToRemoteCoreAndTargetBootstrap(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	local := configureTestTargetEmpty(t)
	t.Setenv("BASEHARBOR_TARGET", local.Name)

	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"app", "apply"})); ok {
		t.Fatal("local workload acceptance unexpectedly received the CI operator")
	}
	projectRoot := filepath.Join(t.TempDir(), "baseharbor-core-bootstrap.test")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_DEMO_ALLOWED_PROJECT_ROOT", projectRoot)
	t.Chdir(projectRoot)
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes"})); !ok {
		t.Fatal("fresh isolated local Core bootstrap did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes", "--json"})); !ok {
		t.Fatal("fresh isolated local Core JSON bootstrap did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes", "--force"})); ok {
		t.Fatal("noncanonical local Core bootstrap unexpectedly received the CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--json", "--yes"})); ok {
		t.Fatal("noncanonical local Core JSON bootstrap unexpectedly received the CI operator")
	}
	t.Setenv("BASEHARBOR_DEMO_ALLOWED_PROJECT_ROOT", "")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes"})); ok {
		t.Fatal("ordinary local Core bootstrap unexpectedly received the CI operator")
	}
	t.Setenv("BASEHARBOR_DEMO_ALLOWED_PROJECT_ROOT", projectRoot)
	t.Chdir(filepath.Dir(projectRoot))
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", local.Name})); ok {
		t.Fatal("existing local Target bootstrap unexpectedly received the CI operator")
	}
	t.Setenv("BASEHARBOR_TARGET", "atomic-test-bootstrap")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "atomic-test-bootstrap"})); !ok {
		t.Fatal("isolated Target bootstrap did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "another-target"})); ok {
		t.Fatal("unselected Target bootstrap unexpectedly received the CI operator")
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	definition := cfg.Targets[local.Name]
	cfg.Access["remote-core"] = deployment.AccessDefinition{Provider: "baseharbor-core", Reference: "https://core.example.test"}
	definition.Access.Reference = "remote-core"
	cfg.Targets[local.Name] = definition
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TARGET", local.Name)
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"app", "apply"})); !ok {
		t.Fatal("enrolled Core Target did not receive its CI operator")
	}

	t.Setenv("DEMO_ATOMIC_GATE", "0")
	t.Setenv("BASEHARBOR_TARGET", local.Name)
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "atomic-test-bootstrap"})); ok {
		t.Fatal("CI operator escaped the disposable acceptance gate")
	}
}

func TestCIIntegrationOperatorIsExplicitlyScopedToManagedLocalGate(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "lifecycle")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-lifecycle", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	plan := []string{"plan"}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), plan)); ok {
		t.Fatal("Coreless local lifecycle unexpectedly received the managed-gate operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"destroy", "--all", "--yes", "--json"})); !ok {
		t.Fatal("exact managed local gate did not receive its CI operator for empty-target cleanup")
	}
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register local Core fixture: %v", err)
	}

	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), plan)); ok {
		t.Fatal("managed local lifecycle unexpectedly received an operator without explicit gate opt-in")
	}
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), plan)); !ok {
		t.Fatal("explicit managed local lifecycle gate did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", target.Name})); ok {
		t.Fatal("target creation unexpectedly received the managed lifecycle operator")
	}
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "backup-restore")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), plan)); ok {
		t.Fatal("mismatched gate opt-in unexpectedly received the CI operator")
	}
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	t.Setenv("DEMO_ATOMIC_GATE", "0")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), plan)); ok {
		t.Fatal("managed local gate operator escaped the atomic acceptance fixture")
	}
}

func TestCIIntegrationOperatorAllowsRootDestroyOnlyForGuidedGate(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "guided")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "guided")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-guided", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	rootDestroy := []string{"destroy", "--all", "--yes"}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"destroy", "--all", "--yes", "--json"})); !ok {
		t.Fatal("exact guided atomic gate did not receive its CI operator for JSON cleanup of an empty Target")
	}
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register local Core fixture: %v", err)
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), rootDestroy)); !ok {
		t.Fatal("exact guided atomic gate did not receive its CI operator for the documented root destroy step")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"destroy", "--all", "--yes", "--force"})); ok {
		t.Fatal("guided root destroy exception unexpectedly authorized a noncanonical cleanup command")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"down"})); ok {
		t.Fatal("guided root destroy exception unexpectedly authorized another root command")
	}
	t.Setenv("GATE", "lifecycle")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"destroy", "--all", "--yes", "--json", "--force"})); ok {
		t.Fatal("guided root destroy exception unexpectedly authorized noncanonical flags")
	}
}
