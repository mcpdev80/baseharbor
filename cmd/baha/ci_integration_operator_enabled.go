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
	operation := cliLocalWorkloadOperation(args)
	managedWorkload := operatorauth.LocalWorkloadOperation(operation)
	// The guided acceptance suite first removes the previous installation
	// before constructing a clean application fixture. Keep this root command
	// exception inside the exact disposable guided gate.
	if !managedWorkload && gate == "guided" && len(args) > 0 && args[0] == "destroy" {
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
	if err != nil {
		return "core-installation-not-loadable: " + err.Error()
	}
	return ""
}

func ciIntegrationOperatorTrace(reason string, args []string, target deployment.ResolvedTarget, err error) {
	if os.Getenv("BASEHARBOR_DEMO_OPERATOR_TRACE") != "1" {
		return
	}
	message := fmt.Sprintf("[ci-operator-testonly] decision=%s command=%q target=%q runtime=%q access=%q tenant_set=%t", reason, strings.Join(args, " "), target.Name, target.RuntimeProvider, target.AccessProvider, strings.TrimSpace(target.TenantID) != "")
	if err != nil {
		message += " cause=" + err.Error()
	}
	fmt.Fprintln(os.Stderr, message)
}

func ciIntegrationLocalCoreBootstrap(args []string, target deployment.ResolvedTarget) bool {
	runtime := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if len(args) != 2 || args[0] != "up" || args[1] != "--yes" ||
		(runtime != "docker" && runtime != "podman") || target.Name == "" || target.RuntimeProvider != runtime ||
		target.Name != strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")) {
		return false
	}
	// This test-only authority is limited to the fresh, explicitly registered
	// temporary project used by the atomic acceptance gate. Never inject it for
	// an ordinary repository workload or for an already installed Core.
	allowedRoot := strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_ALLOWED_PROJECT_ROOT"))
	if allowedRoot == "" {
		return false
	}
	allowedRoot, err := filepath.EvalSymlinks(allowedRoot)
	if err != nil {
		return false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil || cwd != allowedRoot || !strings.HasPrefix(filepath.Base(allowedRoot), "baseharbor-core-bootstrap.") {
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
