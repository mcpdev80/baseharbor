package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type coreAuthorityContextKey struct{}

// Only Core startup supplies this authority. Application Target preferences
// cannot choose another installation or cause a remote node to bootstrap one.
func withCoreAuthority(ctx context.Context, authority deployment.ResolvedTarget) context.Context {
	return context.WithValue(ctx, coreAuthorityContextKey{}, authority)
}

// Operator API startup pins its installation even when Connector enrollment
// is disabled. Per-application runtime brokers do not use this constructor.
func newInstallationMachineExecutor(ctx context.Context, store application.Store) (machinehttp.Executor, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, err
	}
	if target.AccessProvider != "" && target.AccessProvider != string(targetaccess.ProviderLocal) {
		return nil, machine.NewError(machine.ErrorCapabilityMissing,
			"Core API startup requires its local installation Target.", "Select the local Core installation before starting the API.", false)
	}
	return &bahaMachineExecutor{store: store, coreAuthority: &target}, nil
}

func checkBoundCoreTarget(ctx context.Context, selected deployment.ResolvedTarget) error {
	if authority, bound := ctx.Value(coreAuthorityContextKey{}).(deployment.ResolvedTarget); bound && selected != authority {
		return machine.NewError(machine.ErrorPolicyDenied,
			"Core selection differs from this installation's startup-bound authority.",
			"Use this installation's local Core Target.", false)
	}
	return nil
}

func applicationCoreTarget(ctx context.Context) (deployment.ResolvedTarget, bool, error) {
	selected, err := effectiveTarget(ctx)
	if err != nil {
		return deployment.ResolvedTarget{}, false, err
	}
	if selected.AccessProvider == "" || selected.AccessProvider == string(targetaccess.ProviderLocal) {
		if err := checkBoundCoreTarget(ctx, selected); err != nil {
			return deployment.ResolvedTarget{}, false, err
		}
		return selected, false, nil
	}
	if selected.AccessProvider != string(targetaccess.ProviderNodeConnector) {
		return deployment.ResolvedTarget{}, true, machine.NewError(machine.ErrorCapabilityMissing,
			"Selected remote Target has no supported Core installation binding.", "Select an enrolled Target in this installation.", true)
	}
	tenant, ok := tenancy.FromContext(ctx)
	if !ok || tenant.TenantID == "" || tenant.ExternalIdentityID == "" || tenant.TenantID != selected.TenantID {
		return deployment.ResolvedTarget{}, true, machine.NewError(machine.ErrorPolicyDenied,
			"Remote application Target belongs to a different operator scope.", "Use the authorized installation and Target.", false)
	}
	authority, ok := ctx.Value(coreAuthorityContextKey{}).(deployment.ResolvedTarget)
	if !ok || strings.TrimSpace(authority.Name) == "" || authority.AccessProvider != string(targetaccess.ProviderLocal) ||
		(authority.RuntimeProvider != "docker" && authority.RuntimeProvider != "podman") {
		return deployment.ResolvedTarget{}, true, machine.NewError(machine.ErrorCapabilityMissing,
			"Remote application requires the startup-bound Core installation.", "Connect through the selected installation's protected Core API.", true)
	}
	return authority, true, nil
}
