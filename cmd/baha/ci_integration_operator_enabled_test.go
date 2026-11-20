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
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes", "--force"})); ok {
		t.Fatal("noncanonical local Core bootstrap unexpectedly received the CI operator")
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
