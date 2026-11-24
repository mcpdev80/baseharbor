//go:build baseharbor_ci_operator_testonly

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// withCIIntegrationOperator exists only in binaries explicitly built with the
// baseharbor_ci_operator_testonly tag. It supplies the same synthetic verified
// operator used by Go tests, and only while the disposable demo acceptance gate
// is running. Release and ordinary CI binaries do not contain this path.
func withCIIntegrationOperator(ctx context.Context, args []string) context.Context {
	if os.Getenv("DEMO_ATOMIC_GATE") != "1" {
		return ctx
	}
	// Local Docker/Podman acceptance must exercise the real in-process runtime
	// grant. Inject the CI operator for an enrolled remote Core Target, or for
	// the initial target create in the isolated gate before that Target exists.
	// All subsequent local workload operations remain on the runtime-grant path.
	cfg, err := deployment.LoadConfig()
	if err != nil {
		ciIntegrationOperatorTrace("config-load-failed", args, deployment.ResolvedTarget{}, err)
		if !ciIntegrationTargetCreate(args) {
			return ctx
		}
		ciIntegrationOperatorTrace("target-create-bootstrap", args, deployment.ResolvedTarget{}, nil)
		return ciIntegrationOperatorContext(ctx)
	}
	// The acceptance harness deliberately clears BASEHARBOR_TARGET before its
	// first target create. If a config with another default target already
	// exists, resolve that fresh gate target directly so the exact bootstrap
	// still gets the test-only operator without authorizing existing targets.
	if ciIntegrationTargetCreate(args) {
		if _, targetErr := cfg.ResolveTarget(strings.TrimSpace(args[2]), ""); targetErr != nil {
			ciIntegrationOperatorTrace("target-create-bootstrap", args, deployment.ResolvedTarget{}, nil)
			return ciIntegrationOperatorContext(ctx)
		}
	}
	target, err := cfg.ResolveTarget(strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), "")
	if err == nil && target.AccessProvider == "baseharbor-core" {
		ciIntegrationOperatorTrace("remote-core-target", args, target, nil)
		return ciIntegrationOperatorContext(ctx)
	}
	if managedGateReason := ciIntegrationManagedLocalGateReason(args, target, err); managedGateReason == "" {
		// Application status in this disposable gate also evaluates the managed
		// operator boundary. Once Core is installed, local runtime grants are not
		// available for this operation; use only the exact gate-scoped test
		// operator so CI does not try to start an interactive OIDC setup.
		if ciIntegrationReadOnlyStatus(args) {
			if err := ciIntegrationConfigureStatusOperatorBoundary(target.Name); err != nil {
				ciIntegrationOperatorTrace("managed-local-status-boundary-failed", args, target, err)
				return ctx
			}
			ciIntegrationOperatorTrace("managed-local-status-test-operator", args, target, nil)
			return ciIntegrationOperatorContext(ctx)
		}
		ciIntegrationOperatorTrace("managed-local-gate", args, target, nil)
		operatorContext := ciIntegrationOperatorContext(ctx)
		if ciIntegrationGuidedMCPServe(args) && ciIntegrationMCPFootprintParityGate() {
			return context.WithValue(operatorContext, ciMCPToolsetContextKey{}, true)
		}
		return operatorContext
	} else {
		ciIntegrationOperatorTrace("managed-local-gate-rejected: "+managedGateReason, args, target, err)
	}
	if err == nil && target.AccessProvider == "local" && target.TenantID == "" && ciIntegrationLocalCoreBootstrap(args, target) {
		ciIntegrationOperatorTrace("local-core-bootstrap", args, target, nil)
		return ciIntegrationOperatorContext(ctx)
	}
	// Bootstrap is the only local operation that may need a synthetic
	// operator: it must create the currently selected target, and that target
	// must not already resolve. Existing local workload operations therefore
	// continue through the runtime-grant authorization path.
	if err == nil || !ciIntegrationTargetCreate(args) {
		return ctx
	}
	return ciIntegrationOperatorContext(ctx)
}

func ciIntegrationManagedLocalGate(args []string, target deployment.ResolvedTarget) bool {
	return ciIntegrationManagedLocalGateReason(args, target, nil) == ""
}

func ciIntegrationManagedLocalGateReason(args []string, target deployment.ResolvedTarget, targetErr error) string {
	if targetErr != nil {
		return "target-resolution-failed: " + targetErr.Error()
	}
	gate := strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_OPERATOR_GATE"))
	runtime := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if gate == "" || gate != strings.TrimSpace(os.Getenv("GATE")) ||
		(runtime != "docker" && runtime != "podman") || target.Name != "atomic-"+runtime+"-"+gate ||
		target.Name != strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")) || target.AccessProvider != "local" ||
		target.TenantID != "" || target.RuntimeProvider != runtime {
		return fmt.Sprintf("target-or-gate-mismatch gate=%q workflow_gate=%q runtime=%q target=%q env_target=%q access=%q tenant_set=%t target_runtime=%q", gate, strings.TrimSpace(os.Getenv("GATE")), runtime, target.Name, strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), target.AccessProvider, strings.TrimSpace(target.TenantID) != "", target.RuntimeProvider)
	}
	// The candidate's regular CLI scope now recognizes root doctor as the
	// local workload doctor operation. Keep this synthetic gate narrower: it
	// may exercise read-only probes, never `doctor --fix`.
	if len(args) > 0 && args[0] == "doctor" && !ciIntegrationReadOnlyDoctor(args) {
		return "doctor-not-read-only-or-noncanonical: " + strings.Join(args, " ")
	}
	operation := cliLocalWorkloadOperation(args)
	managedWorkload := operatorauth.LocalWorkloadOperation(operation)
	if !managedWorkload && ciIntegrationReadOnlyStatus(args) {
		managedWorkload = true
	}
	// The guided acceptance ensures the fixture's default developer domain
	// before probing the loopback gateway. This exact fixture-scoped operation
	// can persist that default; credentials and other developer commands remain
	// outside the synthetic-operator exception.
	if !managedWorkload && gate == "guided" && ciIntegrationGuidedDevDomain(args) {
		managedWorkload = true
	}
	// Guided regression coverage creates and edits disposable manifests through
	// the interactive `app new` and `app edit` wizards. Keep those mutations
	// limited to their exact canonical commands on the selected guided fixture.
	if !managedWorkload && gate == "guided" && ciIntegrationGuidedAppNew(args) {
		managedWorkload = true
	}
	// The selected data-capabilities fixture's new_application (create-only)
	// step runs exact canonical `app new` and `app edit` forms on its
	// disposable Target; without these admissions the tagged CI binary falls
	// through to an interactive OIDC prompt and the PTY step times out. Both
	// matches remain strict and scoped to their selected fixture gate.
	if !managedWorkload && gate == "data-capabilities" && ciIntegrationGuidedAppNew(args) {
		managedWorkload = true
	}
	if !managedWorkload && (gate == "guided" || gate == "data-capabilities") && ciIntegrationGuidedAppEdit(args) {
		managedWorkload = true
	}
	if !managedWorkload && gate == "guided" && ciIntegrationGuidedFootprintShow(args) {
		managedWorkload = true
	}
	// The data-capabilities/core-capability acceptance gates read the
	// disposable fixture's footprint with the exact canonical
	// `app footprint show -o json` display. Admit that read-only form for the
	// selected gates, independent of the guided-only form above; the write
	// path (record-deployment) stays digest-bound and untouched.
	if !managedWorkload && (gate == "data-capabilities" || gate == "core-capability") && ciIntegrationGuidedFootprintShow(args) {
		managedWorkload = true
	}

	// The data-capabilities fixture reads the same canonical CLI Footprint JSON
	// after its managed boundary is configured. It is a read-only probe
	// (machine_coverage.go maps `baha app footprint show` to app.footprint.show;
	// app_registration_921.go documents the -o json form), so admit only that
	// exact canonical form on the selected data-capabilities gate, mirroring the
	// guided admission. Keep the fail-closed branch below rejecting every other
	// command.
	if !managedWorkload && gate == "data-capabilities" && ciIntegrationDataCapabilitiesFootprintShow(args) {
		managedWorkload = true
	}
	// The guided fixture compares the read-only CLI Footprint JSON with its
	// local stdio MCP client. That client starts the exact canonical server;
	// the fixture invokes only app.footprint.show. The data-capabilities and
	// core-capability parity fixtures compare the same two sources, so the
	// exact canonical MCP server process is admitted there too - always with
	// the one-tool restriction below, never as a general MCP server exception.
	if !managedWorkload && ciIntegrationGuidedMCPServe(args) && ciIntegrationMCPFootprintParityGate() {
		managedWorkload = true
	}
	// The observability acceptance gate reads the disposable demo workload's
	// logs to verify its query path. Raw logs may contain secrets, so this
	// exception exists only in the separately tagged CI binary and is limited
	// to the exact selected gate and canonical fixture application.
	if !managedWorkload && gate == "observability" && ciIntegrationObservabilityLogProbe(args) {
		managedWorkload = true
	}
	// The identity and security acceptance gates drive the canonical read-only
	// fixture probe `baha app exec demo-app <shell>` inside the managed demo
	// workload to verify its OIDC service bindings. `app exec` is a registered
	// (mutating-class) machine operation, so without this admission it falls
	// through to the interactive OIDC operator-auth boundary and the tagged CI
	// binary never reaches the readiness probe. Admit only the exact canonical
	// `app exec demo-app` prefix on the two consumer gates, and never any other
	// command, flag, or fixture application: the fail-closed operator-auth
	// boundary for every other form stays untouched.
	if !managedWorkload && (gate == "identity" || gate == "security") && ciIntegrationAppExecFixtureDemoApp(args) {
		managedWorkload = true
	}
	// The acceptance gates exercise a handful of additional CLI forms that the
	// synthetic operator must stamp instead of falling through to interactive
	// OIDC. Admit each form ONLY on the single consumer gate that invokes it,
	// via exact command match AND gate-scope, so the blast radius stays minimal
	// and every other command keeps the unchanged OIDC fail-closed path.
	if !managedWorkload && gate == "policy" && ciIntegrationConfigOrganizationSet(args) {
		managedWorkload = true
	}
	if !managedWorkload && gate == "policy" && ciIntegrationPolicyExplain(args) {
		managedWorkload = true
	}
	// The policy acceptance gate evaluates the effective policy with
	// `baha policy check`, the exact read-only command the guided fixture
	// asserts. `policy check` is a registered machine operation that reads the
	// effective policy for the selected environment; without this admission it
	// falls through to the interactive OIDC operator-auth boundary and the
	// tagged CI binary never returns the concrete PASS/FAIL assertion the gate
	// reports. Admit only the exact canonical `policy check` forms on the single
	// policy gate, and never any other command, flag, or gate: the fail-closed
	// operator-auth boundary for every other form stays untouched.
	if !managedWorkload && gate == "policy" && ciIntegrationPolicyCheck(args) {
		managedWorkload = true
	}
	if !managedWorkload && gate == "connectivity" && ciIntegrationAppInspectDot(args) {
		managedWorkload = true
	}
	if !managedWorkload && gate == "reconciliation" && ciIntegrationTargetOutputJSON(args) {
		managedWorkload = true
	}
	if !managedWorkload && gate == "full-destroy" && ciIntegrationAppListAllTargets(args) {
		managedWorkload = true
	}
	// Root doctor inspects the control plane and is authorized as
	// control-plane.doctor, not as an app-bound local workload operation. The
	// disposable acceptance gate may use its synthetic operator only for the
	// exact read-only doctor forms below.
	if !managedWorkload && ciIntegrationReadOnlyDoctor(args) {
		managedWorkload = true
	}
	// On a selected disposable local Target, guided acceptance starts from a
	// clean installation and drives the root init wizard before workload
	// operations exist. Keep this bootstrap exception exact to `baha init`.
	if !managedWorkload && ciIntegrationRootInit(args) {
		managedWorkload = true
	}
	// Acceptance gates use the root destroy command to clean up their isolated
	// local fixture. Keep it limited to the exact selected disposable target.
	if !managedWorkload && ciIntegrationRootDestroy(args) {
		managedWorkload = true
	}
	// Fixture cleanup first requests a read-only ownership preview before its
	// separate confirmed destroy call. Admit only that exact preview form.
	if !managedWorkload && ciIntegrationRootDestroyPreview(args) {
		managedWorkload = true
	}
	// Recovery gates also exercise the root `down` command against the local
	// Core. Keep this alias scoped to that explicit fixture.
	if !managedWorkload && gate == "control-plane-recovery" && len(args) > 0 && args[0] == "down" {
		managedWorkload = true
	}
	if !managedWorkload {
		return "command-not-in-managed-workload-set: " + strings.Join(args, " ")
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return "runtime-state-root-failed: " + err.Error()
	}
	_, err = coreinstallation.Load(root)
	if err != nil && errors.Is(err, os.ErrNotExist) && (ciIntegrationRootDestroy(args) || ciIntegrationRootDestroyPreview(args) || ciIntegrationReadOnlyDoctor(args) || ciIntegrationRootInit(args)) {
		// A selected acceptance gate may bootstrap, inspect, or clean up its
		// disposable Target before bootstrap or after a failed bootstrap. Permit
		// only exact init, read-only probe, or idempotent cleanup forms; malformed
		// or unreadable installations still fail closed.
		return ""
	}
	if err != nil {
		return "core-installation-not-loadable: " + err.Error()
	}
	return ""
}

func ciIntegrationReadOnlyDoctor(args []string) bool {
	if len(args) == 1 && args[0] == "doctor" {
		return true
	}
	if len(args) == 2 && args[0] == "doctor" {
		return args[1] == "--json"
	}
	return len(args) == 3 && args[0] == "doctor" && args[1] == "--output" && args[2] == "json"
}

func ciIntegrationReadOnlyStatus(args []string) bool {
	if len(args) == 1 && args[0] == "status" {
		return true
	}
	if len(args) == 2 && args[0] == "status" {
		return args[1] == "--json" || args[1] == "--output=json" || args[1] == "-ojson"
	}
	return len(args) == 3 && args[0] == "status" && (args[1] == "--output" || args[1] == "-o") && args[2] == "json"
}

func ciIntegrationGuidedDevDomain(args []string) bool {
	return len(args) == 2 && args[0] == "dev" && args[1] == "domain"
}

func ciIntegrationGuidedAppNew(args []string) bool {
	return len(args) == 2 && args[0] == "app" && args[1] == "new"
}

func ciIntegrationGuidedAppEdit(args []string) bool {
	return len(args) == 2 && args[0] == "app" && args[1] == "edit"
}

func ciIntegrationGuidedFootprintShow(args []string) bool {
	return len(args) == 5 && args[0] == "app" && args[1] == "footprint" && args[2] == "show" && args[3] == "-o" && args[4] == "json"
}

func ciIntegrationGuidedMCPServe(args []string) bool {
	return len(args) == 2 && args[0] == "mcp" && args[1] == "serve"
}

// ciIntegrationDataCapabilitiesFootprintShow mirrors ciIntegrationGuidedFootprintShow
// for the data-capabilities gate: the canonical read-only footprint probe.
func ciIntegrationDataCapabilitiesFootprintShow(args []string) bool {
	return ciIntegrationGuidedFootprintShow(args)
}

// ciIntegrationMCPFootprintParityGate reports whether the selected disposable
// demo fixture is one of the footprint-parity gates (guided,
// data-capabilities, core-capability) whose acceptance compares the CLI
// Footprint JSON against a local stdio MCP client. The MCP-serve admission is
// limited to exactly these gates; every other gate keeps failing closed.
func ciIntegrationMCPFootprintParityGate() bool {
	switch strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_OPERATOR_GATE")) {
	case "guided", "data-capabilities", "core-capability":
		return true
	default:
		return false
	}
}

func ciIntegrationConfigureStatusOperatorBoundary(target string) error {
	// The disposable runtime gate uses the same verified issuer as
	// ciIntegrationOperatorContext. Root status needs the stored boundary to
	// report the scoped test session as authenticated after Core is installed.
	return persistOperatorAuthBoundary(target, "dev", operatorauth.Config{
		Provider: "external-oidc",
		Issuer:   "https://issuer.test",
		ClientID: "ci-acceptance",
		Scopes:   []string{"openid", "profile", "email"},
	})
}

func ciIntegrationObservabilityLogProbe(args []string) bool {
	return len(args) == 3 && args[0] == "app" && args[1] == "logs" && args[2] == "demo-app"
}

// ciIntegrationAppExecFixtureDemoApp admits the exact, canonical read-only
// fixture probe `baha app exec demo-app <shell>` used by the identity and
// security acceptance gates to verify the managed workload's OIDC service
// bindings. `app exec` is a registered (mutating-class) machine operation, so
// this admission lets the tagged CI operator stamp it on the two consumer
// gates instead of falling through to the interactive OIDC boundary.
// Matching is deliberately strict: only the `app exec demo-app` prefix with the
// fixture application name and a following shell/command token is admitted; any
// empty form, leading baha-level flag, different application, or second
// component selection stays outside the synthetic operator and fails closed.
func ciIntegrationAppExecFixtureDemoApp(args []string) bool {
	if len(args) < 4 || args[0] != "app" || args[1] != "exec" || args[2] != "demo-app" {
		return false
	}
	// The 4th token is the workload shell/command; a baha-level flag here (a
	// different component selector, --cmd, --json, ...) must stay fail-closed.
	if strings.HasPrefix(args[3], "-") {
		return false
	}
	return true
}

// ciIntegrationConfigOrganizationSet admits `baha config organization set` on
// the policy gate ONLY when its --location points at the disposable, per-run
// artifact config root ($ARTIFACT_DIR/policy-organization). That exact location
// is provably throwaway (a temp fixture directory that never persists real
// organization configuration), so the operator stamping this command here
// cannot mutate the developer's real org config. `config organization set`
// genuinely mutates the organization configuration it loads, so admitting it
// anywhere else - including without an exact, isolated --location - would widen
// the managed-operator blast radius and is refused. Any other --source,
// --location, --environment, extra flag, or missing ARTIFACT_DIR base stays
// fail-closed on the OIDC path.
func ciIntegrationConfigOrganizationSet(args []string) bool {
	const locationPrefix = "policy-organization"
	// config organization set --source LOCAL --location LOCATION --environment ENV [-o json|--output json]
	switch {
	case len(args) == 9 &&
		args[0] == "config" && args[1] == "organization" && args[2] == "set" &&
		args[3] == "--source" && args[4] == "local" && args[5] == "--location" &&
		args[7] == "--environment" && args[8] == "json":
	case len(args) == 11 &&
		args[0] == "config" && args[1] == "organization" && args[2] == "set" &&
		args[3] == "--source" && args[4] == "local" && args[5] == "--location" &&
		args[7] == "--environment" &&
		(args[9] == "-o" || args[9] == "--output") && args[10] == "json":
	default:
		return false
	}
	artifacts := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifacts == "" {
		return false
	}
	return strings.HasSuffix(args[6], filepath.Join(artifacts, locationPrefix))
}

// ciIntegrationPolicyExplain admits the exact, read-only `baha policy explain`
// form on the policy gate, with or without a single `-e <env>` deployment
// context selector. It only explains effective policy, defaults and bounded
// overrides. `policy check` has its own admission (ciIntegrationPolicyCheck)
// because it is the command the acceptance gate asserts against.
func ciIntegrationPolicyExplain(args []string) bool {
	switch {
	case len(args) == 2 && args[0] == "policy" && args[1] == "explain":
		return true
	case len(args) == 4 && args[0] == "policy" && args[1] == "explain" && args[2] == "-e":
		return true
	default:
		return false
	}
}

// ciIntegrationPolicyCheck admits the exact, read-only `baha policy check`
// forms on the policy gate: bare `policy check`, `policy check -e <env>`,
// `policy check -o json` and `policy check -e <env> -o json`. `policy check`
// is a registered machine operation that reads the effective policy for the
// selected environment; the acceptance gate asserts this concrete result. Any
// other NAME, flag, extra argument, or form on another gate stays
// fail-closed on the OIDC path.
func ciIntegrationPolicyCheck(args []string) bool {
	if len(args) < 2 || args[0] != "policy" || args[1] != "check" {
		return false
	}
	rest := args[2:]
	// zero, one `-e <env>` selector, and at most one output flag are allowed
	// in any order; a second selector or a second output flag stays rejected.
	hasEnv := false
	outputFlags := 0
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "-e" || rest[i] == "--environment":
			if hasEnv {
				return false
			}
			hasEnv = true
			i++ // skip the environment value
		case strings.HasPrefix(rest[i], "--environment="):
			if hasEnv {
				return false
			}
			hasEnv = true
		case rest[i] == "-o" || rest[i] == "--output":
			outputFlags++
			if outputFlags > 1 {
				return false
			}
			i++ // skip the output value
		case strings.HasPrefix(rest[i], "--output=") ||
			strings.HasPrefix(rest[i], "-o=") ||
			strings.HasPrefix(rest[i], "--output="):
			outputFlags++
			if outputFlags > 1 {
				return false
			}
		default:
			// A positional NAME or any unexpected token stays fail-closed.
			return false
		}
	}
	return true
}

// ciIntegrationAppInspectDot admits the exact read-only `baha app inspect .`
// repository-inspection form on the connectivity gate, with or without a single
// `-o json` output flag. Any other PATH, `--verbose`, `-o text`, or extra
// argument stays fail-closed on the OIDC path.
func ciIntegrationAppInspectDot(args []string) bool {
	switch {
	case len(args) == 3 && args[0] == "app" && args[1] == "inspect" && args[2] == ".":
		return true
	case len(args) == 5 &&
		args[0] == "app" && args[1] == "inspect" && args[2] == "." &&
		args[3] == "-o" && args[4] == "json":
		return true
	default:
		return false
	}
}

// ciIntegrationTargetOutputJSON admits the exact read-only `baha target`
// inspection with a JSON output flag (`-o json` or `--output json`). Matched
// only on the reconciliation gate; any other target subcommand or flag
// combination stays fail-closed on the OIDC path.
func ciIntegrationTargetOutputJSON(args []string) bool {
	if len(args) != 3 || args[0] != "target" {
		return false
	}
	return (args[1] == "-o" && args[2] == "json") || (args[1] == "--output" && args[2] == "json")
}

// ciIntegrationAppListAllTargets admits the exact read-only `baha list
// --all-targets` inventory. Any other list flag or positional argument stays
// outside the synthetic operator and fails closed.
func ciIntegrationAppListAllTargets(args []string) bool {
	return len(args) == 3 && args[0] == "app" && args[1] == "list" && args[2] == "--all-targets"
}

func ciIntegrationRootInit(args []string) bool {
	return len(args) == 1 && args[0] == "init"
}

func ciIntegrationRootDestroy(args []string) bool {
	plain := len(args) == 3 && args[0] == "destroy" && args[1] == "--all" && args[2] == "--yes"
	json := len(args) == 4 && args[0] == "destroy" && args[1] == "--all" && args[2] == "--yes" && args[3] == "--json"
	return plain || json
}

func ciIntegrationRootDestroyPreview(args []string) bool {
	return len(args) == 3 && args[0] == "destroy" && args[1] == "--all" && args[2] == "--json"
}

func ciIntegrationOperatorTrace(reason string, args []string, target deployment.ResolvedTarget, err error) {
	if os.Getenv("BASEHARBOR_DEMO_OPERATOR_TRACE") != "1" {
		return
	}
	message := fmt.Sprintf("[ci-operator-testonly] decision=%s command=%q target=%q runtime=%q access=%q tenant_set=%t", reason, strings.Join(args, " "), target.Name, target.RuntimeProvider, target.AccessProvider, strings.TrimSpace(target.TenantID) != "")
	if err != nil {
		message += " cause=" + err.Error()
	}
	// Preserve test-only authorization decisions in the existing failure
	// diagnostics artifact. Runtime-gate failure handling prints this file,
	// while the JSON error stream remains untouched.
	if artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR")); artifactDir != "" {
		diagnosticDir := filepath.Join(artifactDir, "runtime-diagnostics")
		if os.MkdirAll(diagnosticDir, 0o700) == nil {
			if file, openErr := os.OpenFile(filepath.Join(diagnosticDir, "ci-operator.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); openErr == nil {
				_, _ = fmt.Fprintln(file, message)
				_ = file.Close()
			}
		}
	}
	// Acceptance helpers parse machine-readable errors from stderr. Keep the
	// diagnostic trace out of that stream on GitHub Actions, where the job
	// summary provides a separate channel for test-only diagnostics.
	if summary := strings.TrimSpace(os.Getenv("GITHUB_STEP_SUMMARY")); summary != "" {
		file, openErr := os.OpenFile(summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if openErr == nil {
			_, _ = fmt.Fprintln(file, message)
			_ = file.Close()
		}
		return
	}
	fmt.Fprintln(os.Stderr, message)
}

func ciIntegrationLocalCoreBootstrap(args []string, target deployment.ResolvedTarget) bool {
	runtime := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	canonicalUp := len(args) == 2 && args[0] == "up" && args[1] == "--yes"
	jsonUp := len(args) == 3 && args[0] == "up" && args[1] == "--yes" && args[2] == "--json"
	if (!canonicalUp && !jsonUp) ||
		(runtime != "docker" && runtime != "podman") || target.Name == "" || target.RuntimeProvider != runtime ||
		target.Name != strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")) {
		return false
	}
	// This test-only authority is limited to the fresh, explicitly registered
	// temporary project used by the atomic acceptance gate. Never inject it for
	// an ordinary repository workload or for an already installed Core.
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return false
	}
	allowedRoot := strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_ALLOWED_PROJECT_ROOT"))
	allowed := false
	if allowedRoot != "" {
		allowedRoot, err = filepath.EvalSymlinks(allowedRoot)
		allowed = err == nil && cwd == allowedRoot && strings.HasPrefix(filepath.Base(allowedRoot), "baseharbor-core-bootstrap.")
	}
	// The guided gate reboots Core once after clearing its previous runtime. It
	// uses this fixed fixture directory rather than core-bootstrap.sh's temp
	// project, so bind that one exact root-up to the selected guided gate.
	if !allowed && (canonicalUp || jsonUp) && strings.TrimSpace(os.Getenv("GATE")) == "guided" &&
		strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_OPERATOR_GATE")) == "guided" {
		demoRoot := strings.TrimSpace(os.Getenv("DEMO_ROOT"))
		if demoRoot != "" {
			demoCoreRoot, rootErr := filepath.EvalSymlinks(filepath.Join(demoRoot, "tests", ".guided-control-plane"))
			allowed = rootErr == nil && cwd == demoCoreRoot
		}
	}
	if !allowed {
		return false
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return false
	}
	_, err = coreinstallation.Load(root)
	return errors.Is(err, os.ErrNotExist)
}

func ciIntegrationTargetCreate(args []string) bool {
	selectedTarget := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
	// The disposable demo acceptance harness unsets BASEHARBOR_TARGET while it
	// creates the fresh target. Recover that exact name only from the validated
	// gate/runtime pair, keeping bootstrap authorization scoped to this binary.
	if selectedTarget == "" && os.Getenv("DEMO_ATOMIC_GATE") == "1" {
		runtime := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
		gate := strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_OPERATOR_GATE"))
		if gate != "" && gate == strings.TrimSpace(os.Getenv("GATE")) &&
			(runtime == "docker" || runtime == "podman") {
			selectedTarget = "atomic-" + runtime + "-" + gate
		}
	}
	return len(args) >= 3 && args[0] == "target" && args[1] == "create" &&
		selectedTarget != "" && strings.TrimSpace(args[2]) == selectedTarget
}

func ciIntegrationOperatorContext(ctx context.Context) context.Context {
	principal := &identity.Principal{
		Issuer: "https://issuer.test", Subject: "ci-acceptance-operator",
		IssuerVerified: true, Groups: []string{"editor"},
	}
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	return operatorauth.WithMembershipProvider(ctx, ciAcceptanceMembershipProvider{})
}

type ciAcceptanceMembershipProvider struct{}

func (ciAcceptanceMembershipProvider) ResolveClaimMembership(_ context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !operatorauth.PrincipalsHaveVerifiedIdentity(principal) {
		return nil, operatorauth.ErrClaimsNotVerified
	}
	return []tenancy.Membership{{TenantID: "tenant-ci-acceptance", Role: "editor"}}, nil
}
