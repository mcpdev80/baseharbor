package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
)

// The protected canonical inventory includes management UIs requested by every
// remaining consumer. Retained data alone must not keep a removed UI routable.
func sharedBackendGatewayRetirements(resolved resolvedApplication, routes []devgateway.Route) ([]devgateway.OwnerRoutes, error) {
	found := false
	for _, route := range routes {
		found = found || route.Owner == "shared/postgresql" || route.Owner == "shared/valkey"
	}
	if !found {
		return nil, nil
	}
	inventory, err := application.ExistingSharedBackendInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest.Environment)
	if err != nil {
		return nil, fmt.Errorf("verify shared backend owner before gateway retirement: %w", err)
	}
	return retiredSharedBackendGatewayOwnerRoutes(routes, inventory)
}

func loadSharedBackendGatewayRetirements(resolved resolvedApplication) ([]devgateway.OwnerRoutes, error) {
	routes, err := devgateway.Routes(resolved.Target.Name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sharedBackendGatewayRetirements(resolved, routes)
}

func retiredSharedBackendGatewayOwnerRoutes(routes []devgateway.Route, inventory application.SharedBackendInventory) ([]devgateway.OwnerRoutes, error) {
	services := map[string]bool{}
	for _, names := range inventory.Services {
		for _, name := range names {
			services[name] = true
		}
	}
	groups := []devgateway.OwnerRoutes{}
	for _, ui := range []struct{ owner, service, directory string }{
		{"shared/postgresql", "shared-pgadmin", "postgres"},
		{"shared/valkey", "shared-cache-ui-access", "cache"},
	} {
		if services[ui.service] {
			continue
		}
		found := false
		for _, route := range routes {
			if route.Owner != ui.owner {
				continue
			}
			if route.Key != ui.owner || route.Upstream != "https://"+ui.service+":8443" || route.Network != inventory.Files.Network || route.TrustFile != filepath.Join(inventory.Files.Dir, "management-ui", ui.directory, "pki", "ca.pem") || route.ServerName != "localhost" {
				return nil, fmt.Errorf("refuse shared management route retirement: %s differs from its canonical provider owner", ui.owner)
			}
			found = true
		}
		if found {
			groups = append(groups, devgateway.OwnerRoutes{Owner: ui.owner})
		}
	}
	return groups, nil
}
