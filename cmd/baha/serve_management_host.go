package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

// managementHostKey carries the durable management-host state root into the
// selection context once this serve process has verified (or bootstrapped) it.
// The bound installation runtime and operator control plane read it to refuse to
// run against a different state root than the one this host owns.
type managementHostKey struct{}

// withManagementHostAttached pins the durable management-host state root into the
// selection context so downstream execution never selects a different state root
// than the one this host owns.
func withManagementHostAttached(ctx context.Context, root string) context.Context {
	return context.WithValue(ctx, managementHostKey{}, root)
}

// managementHostAttachedRoot returns the durable state root this serve process
// owns, if any. An empty result means no management-host attachment is in force
// (for example a per-application runtime broker, which owns no installation).
func managementHostAttachedRoot(ctx context.Context) string {
	root, _ := ctx.Value(managementHostKey{}).(string)
	return root
}

// managementHostForServe resolves the durable management-host state root for the
// currently selected installation Target. It returns nil, nil when no local
// installation Target is selected (for example a per-application runtime broker),
// so the durable-authority gate only applies to the installation control plane.
func managementHostForServe(ctx context.Context) (*coreinstallation.ManagementHost, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, err
	}
	// The durable management host owns the local Core installation Target only.
	// A remote Target selects no local state root and is gated elsewhere through
	// the management-connection boundary.
	if target.AccessProvider != "" && target.AccessProvider != string(targetaccess.ProviderLocal) {
		return nil, nil
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	return coreinstallation.NewManagementHost(root)
}
