package main

import (
	"context"
	"errors"
	"os"

	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Destination routes have already been reconciled and verified. Release only
// identity routes still attached to the old realization, never the app owner.
func retireIdentityGatewayRoutes(ctx context.Context, runtime bhruntime.RuntimeProvider, resolved resolvedApplication, old identityprovider.KeycloakFiles) error {
	routes, err := devgateway.Routes(resolved.Target.Name)
	if errors.Is(err, os.ErrNotExist) {
		return removeApplicationGatewayOwners(ctx, runtime, resolved, "app/"+resolved.Manifest.Name+"/"+resolved.Manifest.Environment, "shared/keycloak")
	}
	if err != nil {
		return err
	}
	groups := retainedIdentityGatewayOwnerRoutes(routes, resolved, old.ConsumerNetwork)
	for _, group := range groups {
		if len(group.Routes) == 0 {
			if err := removeApplicationGatewayOwners(ctx, runtime, resolved, group.Owner); err != nil {
				return err
			}
			continue
		}
		issuer, err := resolveApplicationServiceIssuer(ctx, resolved, runtime)
		if err != nil {
			return err
		}
		if err := devgateway.ReplaceOwnerRoutes(ctx, runtime, issuer, resolved.Target.Name, group.Owner, group.Routes); err != nil {
			return err
		}
	}
	return nil
}

func retainedIdentityGatewayOwnerRoutes(routes []devgateway.Route, resolved resolvedApplication, oldNetwork string) []devgateway.OwnerRoutes {
	appOwner := "app/" + resolved.Manifest.Name + "/" + resolved.Manifest.Environment
	groups := []devgateway.OwnerRoutes{}
	for _, owner := range []string{appOwner, "shared/keycloak"} {
		changed := false
		group := devgateway.OwnerRoutes{Owner: owner}
		for _, route := range routes {
			if route.Owner != owner {
				continue
			}
			identityKey := route.Key == appOwner+"/identity" || route.Key == appOwner+"/identity-admin" || route.Key == "shared/keycloak/login" || route.Key == "shared/keycloak/admin"
			if identityKey && oldNetwork != "" && route.Network == oldNetwork {
				changed = true
				continue
			}
			group.Routes = append(group.Routes, route)
		}
		if changed {
			groups = append(groups, group)
		}
	}
	return groups
}
