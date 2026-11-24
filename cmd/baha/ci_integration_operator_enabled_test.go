//go:build baseharbor_ci_operator_testonly

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestCIIntegrationStaticMCPAdmissionIsFixtureAndCommandScoped(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "static")
	t.Setenv("GATE", "mcp")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "mcp")
	target := configureIsolatedEmptyRuntimeTarget(t, "mcp-static", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	for _, args := range [][]string{
		{"init", "--quick", "--json"},
		{"app", "footprint", "show", "-o", "json"},
		{"mcp", "serve"},
	} {
		if !ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("static MCP fixture command %q was not admitted", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); !ok {
			t.Errorf("static MCP fixture command %q did not receive the test-only operator", args)
		}
	}
	for _, args := range [][]string{
		{"init"},
		{"init", "--quick", "--json", "--force"},
		{"app", "apply", "--yes"},
		{"mcp", "serve", "--listen", "0.0.0.0"},
	} {
		if ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("noncanonical static MCP command %q was admitted", args)
		}
	}
	if ciIntegrationManagedLocalGate([]string{"init", "--quick", "--json"}, deployment.ResolvedTarget{
		Name: "other", RuntimeProvider: "docker", AccessProvider: "local",
	}) {
		t.Fatal("static MCP admission escaped its exact fixture Target")
	}
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	if ciIntegrationManagedLocalGate([]string{"init", "--quick", "--json"}, target) {
		t.Fatal("static MCP admission escaped the static runtime")
	}
	if !ciIntegrationStaticMCPTargetCreate([]string{"target", "create", "mcp-static", "--provider", "docker",
		"--access", "local-docker", "--reference", "local", "--scope", "default", "--default"}) {
		t.Fatal("canonical static MCP fixture Target bootstrap was not recognized")
	}
	if ciIntegrationStaticMCPTargetCreate([]string{"target", "create", "other", "--provider", "docker"}) {
		t.Fatal("static MCP fixture bootstrap admitted another Target")
	}
}

func TestCIIntegrationOperatorIsLimitedToRemoteCoreAndTargetBootstrap(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "lifecycle")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
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
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	existingGateTarget := "atomic-docker-lifecycle"
	localDefinition := cfg.Targets[local.Name]
	localDefinition.TargetUID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	cfg.Targets[existingGateTarget] = localDefinition
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", existingGateTarget})); ok {
		t.Fatal("existing atomic gate Target unexpectedly received its CI operator")
	}
	delete(cfg.Targets, existingGateTarget)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "atomic-docker-lifecycle"})); !ok {
		t.Fatal("isolated atomic gate Target bootstrap without an active target did not receive its CI operator")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "atomic-docker-guided"})); ok {
		t.Fatal("Target bootstrap for a different atomic gate unexpectedly received its CI operator")
	}
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "guided")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "atomic-docker-lifecycle"})); ok {
		t.Fatal("Target bootstrap unexpectedly received CI operator when workflow and gate disagree")
	}
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"target", "create", "another-target"})); ok {
		t.Fatal("unselected Target bootstrap unexpectedly received the CI operator")
	}

	cfg, err = deployment.LoadConfig()
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
		if !ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("read-only root status %v was not admitted to the selected local gate", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); !ok {
			t.Errorf("root status %v did not receive the scoped test-only operator for its managed app boundary", args)
		}
	}
	stored, found, err := resolveStoredOperatorAuthBoundary(target.Name, "dev")
	if err != nil || !found || stored.Issuer != "https://issuer.test" || stored.ClientID != "ci-acceptance" {
		t.Fatalf("status gate did not persist its isolated test OIDC boundary: found=%t config=%+v err=%v", found, stored, err)
	}
	for _, args := range [][]string{{"status", "--fix"}, {"status", "--yes"}, {"status", "--json", "--fix"}, {"status", "--output", "text"}, {"status", "-o", "json", "--fix"}} {
		if ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("mutating or noncanonical root status %v was admitted to the selected local gate", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("mutating or noncanonical root status %v unexpectedly received a synthetic CI operator", args)
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
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"destroy", "--all", "--json"})); !ok {
		t.Fatal("exact guided atomic gate did not receive its CI operator for the read-only destroy ownership preview")
	}
	for _, args := range [][]string{{"destroy", "--all", "--force"}, {"destroy", "--all", "--json", "--yes"}, {"destroy", "--all", "--yes", "--force"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Fatal("guided root destroy exception unexpectedly authorized a noncanonical cleanup command")
		}
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

func TestCIIntegrationOperatorAllowsOnlyGuidedDeveloperDomainAndAppNew(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "guided")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "guided")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-guided", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register guided Core fixture: %v", err)
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"dev", "domain"})); !ok {
		t.Fatal("exact guided developer-domain probe did not receive its CI operator")
	}
	for _, args := range [][]string{{"app", "new"}, {"app", "edit"}, {"app", "footprint", "show", "-o", "json"}, {"mcp", "serve"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); !ok {
			t.Errorf("exact guided app acceptance operation %v did not receive its CI operator", args)
		}
	}
	for _, args := range [][]string{{"dev", "domain", "--json"}, {"dev", "credentials"}, {"dev", "token"}, {"app", "new", "--yes"}, {"app", "edit", "--yes"}, {"app", "footprint", "show", "--json"}, {"mcp"}, {"mcp", "serve", "--help"}, {"mcp", "serve", "extra"}} {
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("noncanonical or unrelated developer command %v unexpectedly received the CI operator", args)
		}
	}
	t.Setenv("GATE", "lifecycle")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	lifecycle := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-lifecycle", "docker")
	t.Setenv("BASEHARBOR_TARGET", lifecycle.Name)
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"dev", "domain"})); ok {
		t.Fatal("guided developer-domain exception escaped into another selected gate")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"mcp", "serve"})); ok {
		t.Fatal("guided MCP server exception escaped into another selected gate")
	}
}

func TestCIIntegrationOperatorAdmitsFootprintShowOnlyOnDataCapabilitiesGate(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "data-capabilities")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "data-capabilities")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-data-capabilities", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register data-capabilities Core fixture: %v", err)
	}

	// Positive: the canonical read-only footprint probe is admitted on the gate
	// that needs it and receives the scoped test-only operator.
	canonical := []string{"app", "footprint", "show", "-o", "json"}
	if !ciIntegrationManagedLocalGate(canonical, target) {
		t.Fatalf("canonical footprint show was not admitted to the selected data-capabilities gate")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), canonical)); !ok {
		t.Fatalf("canonical footprint show did not receive the scoped operator on the data-capabilities gate")
	}

	// Negative: any other command stays fail-closed, including the mutating and
	// noncanonical footprint forms as well as unrelated read/app commands.
	for _, args := range [][]string{
		{"app", "footprint", "show", "--json"},
		{"app", "footprint", "show"},
		{"app", "footprint", "show", "prod/docker-dev"},
		{"app", "logs", "demo-app"},
	} {
		if ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("noncanonical or unrelated command %v was admitted on the data-capabilities gate", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("noncanonical or unrelated command %v received the operator on the data-capabilities gate", args)
		}
	}

	// The guided admission is unchanged: the same canonical footprint probe is
	// admitted when the selected gate is guided, and rejected on an unrelated
	// gate that selects neither guided nor data-capabilities.
	guidedTarget := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-guided", "docker")
	t.Setenv("GATE", "guided")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "guided")
	t.Setenv("BASEHARBOR_TARGET", guidedTarget.Name)
	if err := registerInstallationForTarget(t, guidedTarget.Name); err != nil {
		t.Fatalf("register guided Core fixture: %v", err)
	}
	if !ciIntegrationManagedLocalGate(canonical, guidedTarget) {
		t.Fatal("canonical footprint show was not admitted on the guided gate")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), canonical)); !ok {
		t.Fatal("canonical footprint show did not receive its operator on the guided gate")
	}

	lifecycleTarget := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-lifecycle", "docker")
	t.Setenv("GATE", "lifecycle")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	t.Setenv("BASEHARBOR_TARGET", lifecycleTarget.Name)
	if err := registerInstallationForTarget(t, lifecycleTarget.Name); err != nil {
		t.Fatalf("register lifecycle Core fixture: %v", err)
	}
	if ciIntegrationManagedLocalGate(canonical, lifecycleTarget) {
		t.Fatal("canonical footprint show escaped into the lifecycle gate")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), canonical)); ok {
		t.Fatal("canonical footprint show received an operator in the lifecycle gate")
	}
}

func TestCIIntegrationOperatorDataGateAppNewAdmission(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	for _, probe := range []struct{ runtime, gate string }{
		{"docker", "data-capabilities"},
		{"podman", "data-capabilities"},
	} {
		t.Run(probe.runtime+"/"+probe.gate, func(t *testing.T) {
			t.Setenv("BASEHARBOR_TEST_RUNTIME", probe.runtime)
			t.Setenv("GATE", probe.gate)
			t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", probe.gate)
			target := configureIsolatedEmptyRuntimeTarget(t, "atomic-"+probe.runtime+"-"+probe.gate, probe.runtime)
			if err := registerInstallationForTarget(t, target.Name); err != nil {
				t.Fatalf("register %s %s Core fixture: %v", probe.runtime, probe.gate, err)
			}
			t.Setenv("BASEHARBOR_TARGET", target.Name)
			if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"app", "new"})); !ok {
				t.Fatal("canonical app new did not receive the CI operator in the selected data gate (interactive OIDC fallthrough)")
			}
			if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), []string{"app", "edit"})); !ok {
				t.Fatal("exact data-capabilities app edit did not receive its CI operator")
			}
			for _, args := range [][]string{{"app", "new", "--yes"}, {"app", "new", "extra"}, {"app", "edit", "--yes"}, {"app", "edit", "extra"}} {
				if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
					t.Errorf("non-canonical form %v unexpectedly received the CI operator under %s", args, probe.gate)
				}
			}
		})
	}
	// Other gates keep the guided-only behaviour unchanged (no blanket allow).
	for _, gate := range []string{"core-capability", "lifecycle", "observability"} {
		t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
		t.Setenv("GATE", gate)
		t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", gate)
		target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-"+gate, "docker")
		if err := registerInstallationForTarget(t, target.Name); err != nil {
			t.Fatalf("register %s Core fixture: %v", gate, err)
		}
		t.Setenv("BASEHARBOR_TARGET", target.Name)
		for _, args := range [][]string{{"app", "new"}, {"app", "edit"}} {
			if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
				t.Errorf("app wizard admission %v escaped into gate %q", args, gate)
			}
		}
	}
}
func TestCIIntegrationGuidedMCPPrincipalCanOnlySeeFootprintTool(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "guided")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "guided")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-guided", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register guided Core fixture: %v", err)
	}

	ctx := withCIIntegrationOperator(context.Background(), []string{"mcp", "serve"})
	if _, ok := operatorauth.PrincipalFromContext(ctx); !ok {
		t.Fatal("exact guided MCP process did not receive the test-only principal")
	}
	server := newMCPServerForContext(ctx, application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "guided-footprint-allowlist-test", Version: "v1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	listed, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "baseharbor.app.footprint.show" {
		t.Fatalf("guided test MCP server tools = %v, want only baseharbor.app.footprint.show", listed.Tools)
	}
	if _, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.destroy", Arguments: map[string]any{"name": "demo"}}); err == nil {
		t.Fatal("guided test MCP server exposed a mutating app.destroy tool")
	}
}

func TestCIIntegrationOperatorAllowsOnlyObservabilityLogProbe(t *testing.T) {
	t.Setenv("DEMO_ATOMIC_GATE", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("GATE", "observability")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "observability")
	target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-observability", "docker")
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatalf("register local Core fixture: %v", err)
	}

	probe := []string{"app", "logs", "demo-app"}
	if !ciIntegrationManagedLocalGate(probe, target) {
		t.Fatal("canonical disposable observability log probe was not admitted")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), probe)); !ok {
		t.Fatal("canonical disposable observability log probe did not receive its CI-only operator")
	}

	for _, args := range [][]string{
		{"app", "logs", "demo-app", "--follow"},
		{"app", "logs", "other-app"},
		{"logs", "demo-app"},
	} {
		if ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("noncanonical log probe %v was admitted", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("noncanonical log probe %v received a CI-only operator", args)
		}
	}

	t.Setenv("GATE", "lifecycle")
	t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "lifecycle")
	if ciIntegrationManagedLocalGate(probe, target) {
		t.Fatal("observability log probe escaped into another selected gate")
	}
	if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), probe)); ok {
		t.Fatal("observability log probe received an operator in another selected gate")
	}
}

func TestCIIntegrationOperatorAdmitsFiveCoreFixtureFormsOnlyOnConsumerGate(t *testing.T) {
	// Card 0428-250: five exact, managed/inspection command forms stamp the
	// synthetic operator ONLY on the single consumer gate that invokes them, and
	// stay fail-closed everywhere else. Each form is gate-scoped, so the same
	// exact form on any other gate must NOT reach the operator.
	type formCase struct {
		name string
		gate string
		args []string
	}
	cases := []formCase{
		{"config organization set", "policy", []string{"config", "organization", "set", "--source", "local", "--location", "$ARTIFACT_DIR/policy-organization", "--environment", "dev", "-o", "json"}},
		{"policy explain", "policy", []string{"policy", "explain"}},
		{"policy explain -e dev", "policy", []string{"policy", "explain", "-e", "dev"}},
		// Root `up` is an existing managed-local `apply` operation. Cover both
		// consent forms on multiple matching gates; it is not a one-off,
		// gate-specific command-form admission.
		{"up --yes", "policy", []string{"up", "--yes"}},
		{"up --yes --confirm-candidate", "policy", []string{"up", "--yes", "--confirm-candidate", "d6d633d0a68fd231d3fccfcd7d88994cd7834db748a80544d12dc7a288f6256e"}},
		{"up --yes on connectivity", "connectivity", []string{"up", "--yes"}},
		{"up --yes --confirm-candidate on connectivity", "connectivity", []string{"up", "--yes", "--confirm-candidate", "d6d633d0a68fd231d3fccfcd7d88994cd7834db748a80544d12dc7a288f6256e"}},
		{"app inspect .", "connectivity", []string{"app", "inspect", "."}},
		{"app inspect . -o json", "connectivity", []string{"app", "inspect", ".", "-o", "json"}},
		{"target -o json", "reconciliation", []string{"target", "-o", "json"}},
		{"target --output json", "reconciliation", []string{"target", "--output", "json"}},
		{"app list --all-targets", "full-destroy", []string{"app", "list", "--all-targets"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEMO_ATOMIC_GATE", "1")
			t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
			t.Setenv("GATE", tc.gate)
			t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", tc.gate)
			artifacts := t.TempDir()
			t.Setenv("ARTIFACT_DIR", artifacts)
			args := tc.args
			// The config organization set form passes the exact, disposable
			// artifact config root as its --location. The real shell expands
			// $ARTIFACT_DIR before Go sees the args, so resolve it here against
			// the test's ARTIFACT_DIR instead of a literal shell variable.
			if tc.name == "config organization set" {
				args = []string{"config", "organization", "set", "--source", "local", "--location", filepath.Join(artifacts, "policy-organization"), "--environment", "dev", "-o", "json"}
			}
			target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-"+tc.gate, "docker")
			t.Setenv("BASEHARBOR_TARGET", target.Name)
			if err := registerInstallationForTarget(t, target.Name); err != nil {
				t.Fatalf("register %s Core fixture: %v", tc.gate, err)
			}
			if !ciIntegrationManagedLocalGate(args, target) {
				t.Fatalf("canonical %v was not admitted on its consumer gate %q", args, tc.gate)
			}
			if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); !ok {
				t.Fatalf("canonical %v did not receive its scoped operator on gate %q", args, tc.gate)
			}
		})
	}

	// Negative: each admitted exact form on the WRONG consumer gate must not be
	// admitted and must not reach the operator (fail-closed preserved).
	wrongGateForms := []struct {
		name  string
		args  []string
		wrong string
	}{
		{"config organization set on connectivity", []string{"config", "organization", "set", "--source", "local", "--location", "$ARTIFACT_DIR/policy-organization", "--environment", "dev", "-o", "json"}, "connectivity"},
		{"policy explain on full-destroy", []string{"policy", "explain"}, "full-destroy"},
		{"app inspect . on policy", []string{"app", "inspect", "."}, "policy"},
		{"target -o json on policy", []string{"target", "-o", "json"}, "policy"},
		{"app list --all-targets on connectivity", []string{"app", "list", "--all-targets"}, "connectivity"},
	}
	for _, tc := range wrongGateForms {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEMO_ATOMIC_GATE", "1")
			t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
			t.Setenv("GATE", tc.wrong)
			t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", tc.wrong)
			artifacts := t.TempDir()
			t.Setenv("ARTIFACT_DIR", artifacts)
			args := tc.args
			// Resolve the artifact config root so this negative is meaningful:
			// a broken (gate-independent) matcher would admit the exact form.
			if tc.name == "config organization set on connectivity" {
				args = []string{"config", "organization", "set", "--source", "local", "--location", filepath.Join(artifacts, "policy-organization"), "--environment", "dev", "-o", "json"}
			}
			target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-"+tc.wrong, "docker")
			t.Setenv("BASEHARBOR_TARGET", target.Name)
			if err := registerInstallationForTarget(t, target.Name); err != nil {
				t.Fatalf("register %s Core fixture: %v", tc.wrong, err)
			}
			if ciIntegrationManagedLocalGate(args, target) {
				t.Errorf("form %v was admitted on the wrong gate %q (gate-scoping broken)", args, tc.wrong)
			}
			if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
				t.Errorf("form %v reached a scoped operator on the wrong gate %q", args, tc.wrong)
			}
		})
	}

	// Negative: mutating / noncanonical / non-artifact forms stay fail-closed on
	// their consumer gate, preserving the unchanged OIDC default. Each config
	// organization set variant here is a MUTATION variant that must be refused:
	// location outside the isolated ARTIFACT_DIR/policy-organization root, or a
	// source other than local, or an extra positional argument.
	artifacts := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifacts)
	for _, args := range [][]string{
		{"config", "organization", "set", "--source", "local", "--location", "/home/real/policy-organization", "--environment", "dev", "-o", "json"},
		{"config", "organization", "set", "--source", "git", "--location", filepath.Join(artifacts, "policy-organization"), "--environment", "dev", "-o", "json"},
		{"config", "organization", "set", "--source", "local", "--location", filepath.Join(artifacts, "policy-organization"), "--environment", "dev", "-o", "json", "--yes"},
		{"config", "organization", "show", "--environment", "dev"},
		{"policy", "check", "-e", "dev"},
		{"app", "inspect", ".."},
		{"app", "inspect", ".", "--verbose"},
		{"app", "inspect", ".", "-o", "text"},
		{"target", "--json"},
		{"target", "list"},
		{"app", "list"},
		{"app", "list", "--all-targets", "--json"},
	} {
		t.Setenv("DEMO_ATOMIC_GATE", "1")
		t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
		t.Setenv("GATE", "policy")
		t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "policy")
		target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-policy", "docker")
		t.Setenv("BASEHARBOR_TARGET", target.Name)
		if err := registerInstallationForTarget(t, target.Name); err != nil {
			t.Fatalf("register policy Core fixture: %v", err)
		}
		if ciIntegrationManagedLocalGate(args, target) {
			t.Errorf("form %v was admitted on the policy gate (fail-closed broken)", args)
		}
		if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
			t.Errorf("form %v reached a scoped operator on the policy gate (fail-closed broken)", args)
		}
	}

	// Wrong-gate negative: a form ADMITTED on its consumer gate (policy explain
	// on the policy gate) must NOT be admitted on a different gate. This
	// exercises gate-scoping directly, complementing the wrong-gate table above.
	t.Run("policy explain rejected on connectivity", func(t *testing.T) {
		t.Setenv("DEMO_ATOMIC_GATE", "1")
		t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
		t.Setenv("GATE", "connectivity")
		t.Setenv("BASEHARBOR_DEMO_OPERATOR_GATE", "connectivity")
		t.Setenv("ARTIFACT_DIR", t.TempDir())
		target := configureIsolatedEmptyRuntimeTarget(t, "atomic-docker-connectivity", "docker")
		t.Setenv("BASEHARBOR_TARGET", target.Name)
		if err := registerInstallationForTarget(t, target.Name); err != nil {
			t.Fatalf("register connectivity Core fixture: %v", err)
		}
		for _, args := range [][]string{{"policy", "explain"}, {"policy", "explain", "-e", "dev"}} {
			if ciIntegrationManagedLocalGate(args, target) {
				t.Errorf("policy explain admitted on the connectivity gate (gate-scoping broken): %v", args)
			}
			if _, ok := operatorauth.PrincipalFromContext(withCIIntegrationOperator(context.Background(), args)); ok {
				t.Errorf("policy explain reached a scoped operator on the connectivity gate: %v", args)
			}
		}
	})
}
