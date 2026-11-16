package main

import (
	"context"
	"errors"
	"os"

	"github.com/mcpdev80/baseharbor/internal/devgateway"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Route removal follows the existing route owner, including routes left by a
// prior intent. The last route does not need a functioning issuing provider to
// retire its owned gateway while retaining protected certificate material.
func removeApplicationGatewayOwners(ctx context.Context, runtime bhruntime.RuntimeProvider, resolved resolvedApplication, owners ...string) error {
	routes, err := devgateway.Routes(resolved.Target.Name)
	if errors.Is(err, os.ErrNotExist) {
		// Missing registration is safe only when the existing authority also
		// verifies no retained projection or runtime without its owner record.
		for _, owner := range owners {
			if _, inspectErr := devgateway.OwnerRoutesPresent(ctx, runtime, resolved.Target.Name, owner); inspectErr != nil {
				return inspectErr
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, owner := range owners {
		wanted[owner] = true
	}
	found, remaining := false, false
	for _, route := range routes {
		if wanted[route.Owner] {
			found = true
		} else {
			remaining = true
		}
	}
	if !found {
		return nil
	}
	var issuer serviceaccess.Issuer
	if remaining {
		issuer, err = resolveApplicationServiceIssuer(ctx, resolved, runtime)
		if err != nil {
			return err
		}
	}
	return devgateway.RemoveOwners(ctx, runtime, issuer, resolved.Target.Name, owners...)
}
