package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// resolveTargetBelowStateRoot is the state-root-owned selection chain
// (BF-01, h-d28 akte): the effective Target of an isolated installation
// comes from that installation's own state only. The chain, in order:
//
//  1. the activation persisted below the root,
//  2. the root's own organization state — organization state whose source
//     location lives below the root (the way the CLI records it;
//     docs/explanation/targets.md: the override owns the organization
//     state location), preferring the environment's target and then the
//     organization default,
//  3. with no activation and no organization state of its own, the single
//     registration the root carries — the root-local first-run selection.
//
// Anything else — the user installation's ambient activation, another
// installation's organization default, the builtin `local` fallback — is
// foreign state: it never resolves, and never acts on, this root's
// registrations. With several registrations and no root-local selection
// the call fails closed, naming this root's own targets.
func resolveTargetBelowStateRoot(ctx context.Context, cfg deployment.Config, state orgconfig.ActiveState, configured bool, explicit, selected string) (deployment.ResolvedTarget, error) {
	if explicit != "" {
		return cfg.ResolveTarget(explicit, "")
	}
	if selected != "" {
		if _, ok := cfg.Targets[selected]; ok || selected == "local" {
			return cfg.ResolveTarget(selected, "")
		}
	}
	persisted, err := readPersistedTarget()
	if err != nil {
		return deployment.ResolvedTarget{}, err
	}
	if persisted != "" {
		if _, ok := cfg.Targets[persisted]; ok || persisted == "local" {
			return cfg.ResolveTarget(persisted, "")
		}
		// A selection persisted below this root that names no registration
		// of this root is a stale selection: fail closed, never resolve
		// through an organization default or the builtin fallback.
		return deployment.ResolvedTarget{}, machine.NewError(machine.ErrorNotFound,
			fmt.Sprintf("activated target %q no longer exists", persisted),
			"Run baha target deactivate and choose an existing target.", false)
	}
	if configured {
		if orgconfig.StateRootOwnsOrganizationState(state) {
			candidate := orgconfig.OrganizationTargetSelection(state)
			if _, ok := cfg.Targets[candidate]; ok {
				return cfg.ResolveTarget(candidate, "")
			}
			if len(cfg.Targets) > 1 {
				// The root-local organization default names no registration
				// of this root: a default from another installation, or a
				// stale root-local default. Never resolve foreign
				// registrations.
				return deployment.ResolvedTarget{}, fmt.Errorf("organization default target %q is not registered below the selected state root", candidate)
			}
		}
	}
	if names := cfg.TargetNames(); len(names) == 1 && names[0] != "local" {
		return cfg.ResolveTarget(names[0], "")
	}
	if len(cfg.Targets) > 1 {
		choices := cfg.TargetNames()
		sort.Strings(choices)
		return deployment.ResolvedTarget{}, machine.NewError(machine.ErrorConflict,
			fmt.Sprintf("multiple deployment targets configured; choose one of: %s", strings.Join(choices, ", ")),
			"Run baha target activate NAME below the state root, or set BASEHARBOR_TARGET=NAME for this command. Use baha target list to inspect available targets.", false)
	}
	if configured && !orgconfig.StateRootOwnsOrganizationState(state) && orgconfig.OrganizationTargetSelection(state) != "" {
		// The root has no organization state of its own; an organization
		// default from a different installation must not resolve this
		// root's registrations. Fail closed instead.
		return deployment.ResolvedTarget{}, fmt.Errorf("organization default target %q is not registered below the selected state root", orgconfig.OrganizationTargetSelection(state))
	}
	return cfg.ResolveTarget(selected, "")
}
