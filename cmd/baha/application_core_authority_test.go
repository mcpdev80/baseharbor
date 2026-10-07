package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func remoteApplicationCoreFixture(t *testing.T) (context.Context, deployment.ResolvedTarget, deployment.ResolvedTarget) {
	core := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	const owner = "11111111-1111-4111-8111-111111111111"
	cfg.Access["connector"] = deployment.AccessDefinition{Provider: "baseharbor-node-connector", Reference: "node-owned"}
	cfg.Targets["remote"] = deployment.TargetDefinition{TenantID: owner, Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "connector"}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	node, err := cfg.ResolveTarget("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	// Unit boundary fixture: production OIDC/enrollment qualification remains
	// the separate native test, rather than a claim made by this context.
	ctx := operatorauth.WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "verified-subject"})
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: owner, ExternalIdentityID: "verified-subject", Roles: []string{"editor"}})
	return withTargetOverride(ctx, "remote"), core, node
}

func TestRemoteApplicationUsesStartupCoreWithoutChangingExecutionNode(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	ctx = withCoreAuthority(ctx, core)
	selected, remote, err := applicationCoreTarget(ctx)
	if err != nil || !remote || selected != core {
		t.Fatal("installation authority differs", selected, err)
	}
	execution, err := effectiveTarget(ctx)
	if err != nil || execution != node {
		t.Fatal("Core selection changed execution placement", execution, err)
	}
	// An unreadied installation cannot be bootstrapped on the execution node,
	// even when a caller supplies --yes and affirmative input.
	previous := applicationCoreBootstrap
	t.Cleanup(func() { applicationCoreBootstrap = previous })
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		t.Fatal("remote application attempted another Core bootstrap")
		return coreinstallation.State{}, nil
	}
	err = requireApplicationCore(withAssumeYes(ctx, true), strings.NewReader("y\n1\n"), io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "core_required" {
		t.Fatal("missing real Core readiness was accepted", err)
	}
	for _, target := range []deployment.ResolvedTarget{core, node} {
		root, err := targetRuntimeStateRoot(target)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("prerequisite created installation state", err)
		}
	}
}

func TestRemoteApplicationRejectsMissingForeignOrRemoteAuthority(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	for _, kind := range []string{"missing", "tenant", "identity", "remote-authority", "runtime"} {
		t.Run(kind, func(t *testing.T) {
			check := ctx
			authority := core
			switch kind {
			case "tenant":
				check = tenancy.WithContext(check, &tenancy.Context{TenantID: "22222222-2222-4222-8222-222222222222", ExternalIdentityID: "foreign-subject"})
			case "identity":
				check = tenancy.WithContext(check, &tenancy.Context{TenantID: node.TenantID})
			case "remote-authority":
				authority = node
			case "runtime":
				authority.RuntimeProvider = "kubernetes"
			}
			if kind != "missing" {
				check = withCoreAuthority(check, authority)
			}
			if _, _, err := applicationCoreTarget(check); err == nil {
				t.Fatal("unbound installation authority admitted")
			}
		})
	}
	// Existing local application setup remains independently selected.
	local := withTargetOverride(ctx, core.Name)
	selected, remote, err := applicationCoreTarget(local)
	if err != nil || remote || selected != core {
		t.Fatal("local prerequisite changed", selected, err)
	}
}

func TestBoundApplicationCoreRejectsAnotherLocalInstallationBeforeBootstrap(t *testing.T) {
	ctx, core, _ := remoteApplicationCoreFixture(t)
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Targets["foreign-local"] = cfg.Targets[core.Name]
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	foreign, err := cfg.ResolveTarget("foreign-local", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx = withCoreAuthority(withTargetOverride(ctx, foreign.Name), core)
	previous := applicationCoreBootstrap
	t.Cleanup(func() { applicationCoreBootstrap = previous })
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		t.Fatal("bound Core API attempted to bootstrap another local installation")
		return coreinstallation.State{}, nil
	}
	err = requireApplicationCore(withAssumeYes(ctx, true), strings.NewReader("y\n1\n"), io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorPolicyDenied {
		t.Fatal("another local installation escaped the startup authority", err)
	}
	_, err = installCore(ctx, strings.NewReader(""), io.Discard, runtimeUpOptions{Yes: true, ControlPlaneOnly: true})
	if !errors.As(err, &failure) || failure.Code != machine.ErrorPolicyDenied {
		t.Fatal("explicit bootstrap escaped the owning local installation", err)
	}
	selected, err := effectiveTarget(ctx)
	if err != nil || selected != foreign {
		t.Fatal("denial silently changed the execution Target", selected, err)
	}
	for _, target := range []deployment.ResolvedTarget{core, foreign} {
		root, err := targetRuntimeStateRoot(target)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("denied installation selection created Core state", err)
		}
	}
	selected, remote, err := applicationCoreTarget(withTargetOverride(ctx, core.Name))
	if err != nil || remote || selected != core {
		t.Fatal("owning local installation was denied", selected, err)
	}
}

func TestCoreSetupHTTPRejectsExecutionNodeWithoutInstallationState(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	for _, bound := range []bool{false, true} {
		executor := &bahaMachineExecutor{}
		if bound {
			executor.coreAuthority = &core
		}
		_, err := executor.Execute(ctx, machine.Operation{ID: "control-plane.up"},
			machine.OperationContext{Target: node.Name, Environment: "dev"}, json.RawMessage(`{"target":"remote"}`), nil)
		var failure *machine.Error
		if !errors.As(err, &failure) || (failure.Code != machine.ErrorPolicyDenied && failure.Code != machine.ErrorCapabilityMissing) {
			t.Fatal("HTTP Core setup admitted an execution node", bound, err)
		}
		for _, target := range []deployment.ResolvedTarget{core, node} {
			root, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("denied HTTP Core setup created installation state", bound, err)
			}
		}
	}
}
