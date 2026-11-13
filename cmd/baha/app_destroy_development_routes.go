package main

import (
	"context"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
)

func (e *applicationDestroyExecution) cleanupDevelopmentCanonicalRoutes(ctx context.Context) error {
	if !devaccess.Enabled(e.manifest.Environment) {
		return nil
	}
	appOwner := "app/" + e.manifest.Name + "/" + e.manifest.Environment
	present, err := devgateway.OwnerRoutesPresent(ctx, e.compose, e.resolved.Target.Name, appOwner)
	if err != nil {
		return fmt.Errorf("inspect development gateway before cleanup: %w", err)
	}
	if !present {
		e.term.Info("development gateway", "NOT DEPLOYED for this application; no routes to remove")
		return nil
	}
	if err := removeApplicationGatewayOwners(ctx, e.compose, e.resolved, appOwner); err != nil {
		return fmt.Errorf("remove canonical development routes: %w", err)
	}
	return nil
}
