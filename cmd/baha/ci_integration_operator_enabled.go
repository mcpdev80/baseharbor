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
	target, err := cfg.ResolveTarget(strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), "")
	if err == nil && target.AccessProvider == "baseharbor-core" {
		ciIntegrationOperatorTrace("remote-core-target", args, target, nil)
		return ciIntegrationOperatorContext(ctx)
	}
	if managedGateReason := ciIntegrationManagedLocalGateReason(args, target, err); managedGateReason == "" {
		ciIntegrationOperatorTrace("managed-local-gate", args, target, nil)
		return ciIntegrationOperatorContext(ctx)
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
	// Root doctor inspects the control plane and is authorized as
	// control-plane.doctor, not as an app-bound local workload operation. The
	// disposable acceptance gate may use its synthetic operator only for the
	// exact read-only doctor forms below.
	if !managedWorkload && ciIntegrationReadOnlyDoctor(args) {
		managedWorkload = true
	}
	// Root status resolves the selected local Target and reports its managed
	// application state. The disposable runtime gates need that read-only probe
	// after guided adoption, where the local runtime grant is intentionally not
	// an operator identity. Admit only its exact read-only output forms.
	if !managedWorkload && ciIntegrationReadOnlyStatus(args) {
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
	if err != nil && errors.Is(err, os.ErrNotExist) && (ciIntegrationRootDestroy(args) || ciIntegrationReadOnlyDoctor(args) || ciIntegrationRootInit(args)) {
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

func ciIntegrationRootInit(args []string) bool {
	return len(args) == 1 && args[0] == "init"
}

func ciIntegrationRootDestroy(args []string) bool {
	plain := len(args) == 3 && args[0] == "destroy" && args[1] == "--all" && args[2] == "--yes"
	json := len(args) == 4 && args[0] == "destroy" && args[1] == "--all" && args[2] == "--yes" && args[3] == "--json"
	return plain || json
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
