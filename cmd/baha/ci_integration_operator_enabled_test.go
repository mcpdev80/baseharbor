//go:build baseharbor_ci_operator_testonly

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestCIIntegrationOperatorTraceKeepsMachineErrorStreamClean(t *testing.T) {
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_TRACE", "1")
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	t.Setenv("ARTIFACT_DIR", dir)
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStderr := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = previousStderr }()

	ciIntegrationOperatorTrace("managed-local-gate", []string{"destroy", "--all", "--yes", "--json"}, deployment.ResolvedTarget{Name: "atomic-docker-guided", RuntimeProvider: "docker", AccessProvider: "local"}, nil)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(stderr) != 0 {
		t.Fatalf("test-only trace contaminated machine error stream: %q", stderr)
	}
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `decision=managed-local-gate command="destroy --all --yes --json"`) {
		t.Fatalf("test-only operator trace missing from job summary: %s", data)
	}
	data, err = os.ReadFile(filepath.Join(dir, "runtime-diagnostics", "ci-operator.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `decision=managed-local-gate command="destroy --all --yes --json"`) {
		t.Fatalf("test-only operator trace missing from runtime diagnostics: %s", data)
	}
}

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

func TestCIIntegrationOperatorAllowsOnlyGuidedCoreRebootstrapRoot(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "guided")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "guided")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-guided", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	demoRoot := t.TempDir()
	coreRoot := filepath.Join(demoRoot, "tests", ".guided-control-plane")
	if err := os.MkdirAll(coreRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEMO_ROOT", demoRoot)
	t.Chdir(coreRoot)
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes"})); !ok {
		t.Fatal("guided Core rebootstrap in its exact disposable fixture root did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes", "--json"})); !ok {
		t.Fatal("canonical guided Core JSON rebootstrap did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes", "--force"})); ok {
		t.Fatal("noncanonical guided Core rebootstrap unexpectedly received its CI operator")
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "installation.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes"})); ok {
		t.Fatal("guided Core rebootstrap unexpectedly received its CI operator for a present but unreadable installation")
	}
	t.Setenv("GATE", "lifecycle")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	lifecycle := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-lifecycle", "docker")
	t.Setenv("BASEHARBOR_TARGET", lifecycle.Name)
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"up", "--yes"})); ok {
		t.Fatal("guided Core rebootstrap exception escaped into another selected gate")
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
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"init"})); !ok {
		t.Fatal("exact guided init on the disposable gate target did not receive its CI operator")
	}
	for _, args := range [][]string{{"init", "--fix"}, {"init", "--json"}, {"version"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("noncanonical or unrelated command %v unexpectedly received the CI operator", args)
		}
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"destroy", "--all", "--yes", "--json"})); !ok {
		t.Fatal("exact managed local gate did not receive its CI operator for empty-target cleanup")
	}
	for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}, {"doctor", "--output", "json"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); !ok {
			t.Errorf("read-only managed-gate doctor %v did not receive its CI operator", args)
		}
	}
	for _, args := range [][]string{{"doctor", "--fix"}, {"doctor", "--yes"}, {"doctor", "--json", "--fix"}, {"doctor", "--output", "text"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("mutating or noncanonical managed-gate doctor %v unexpectedly received its CI operator", args)
		}
	}
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register local Core fixture: %v", err)
	}
	for _, args := range [][]string{{"status"}, {"status", "--json"}, {"status", "-o", "json"}, {"status", "--output", "json"}, {"status", "--output=json"}, {"status", "-ojson"}} {
		if !ciIntegrationReadOnlyStatus(args) {
			t.Errorf("canonical read-only status %v was not recognized", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); !ok {
			t.Errorf("read-only managed-gate status %v did not receive its CI operator", args)
		}
	}
	for _, args := range [][]string{{"status", "--fix"}, {"status", "--yes"}, {"status", "--json", "--fix"}, {"status", "--output", "text"}, {"status", "-o", "json", "--fix"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("mutating or noncanonical managed-gate status %v unexpectedly received its CI operator", args)
		}
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
