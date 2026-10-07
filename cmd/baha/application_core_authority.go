package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type coreAuthorityContextKey struct{}

// Only Core startup supplies this authority. Application Target preferences
// cannot choose another installation or cause a remote node to bootstrap one.
func withCoreAuthority(ctx context.Context, authority deployment.ResolvedTarget) context.Context {
	return context.WithValue(ctx, coreAuthorityContextKey{}, authority)
}

func applicationCoreTarget(ctx context.Context) (deployment.ResolvedTarget, bool, error) {
	selected, err := effectiveTarget(ctx)
	if err != nil {
		return deployment.ResolvedTarget{}, false, err
	}
	if selected.AccessProvider == "" || selected.AccessProvider == string(targetaccess.ProviderLocal) {
		if authority, bound := ctx.Value(coreAuthorityContextKey{}).(deployment.ResolvedTarget); bound && selected != authority {
			return deployment.ResolvedTarget{}, false, machine.NewError(machine.ErrorPolicyDenied,
				"Application selection differs from this Core installation's local authority.",
				"Use this installation's local Target or an authorized enrolled execution Target.", false)
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
