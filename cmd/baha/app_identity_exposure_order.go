package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/exposure"
)

func managedIdentityNeedsRealizedExposure(prepared *managedIdentityExecution) bool {
	if prepared == nil || prepared.keycloak == nil || devaccess.Enabled(prepared.manifest.Environment) {
		return false
	}
	for _, route := range prepared.manifest.Exposures {
		if !strings.EqualFold(strings.TrimSpace(route.Visibility), "internal") {
			return true
		}
	}
	return false
}

// Provision required identity bindings before starting the workload. Production
// public callback readiness is verified only after actual exposure convergence.
func provisionManagedIdentityBeforeExposure(ctx context.Context, out io.Writer, prepared *managedIdentityExecution, exposed *managedExposureExecution) error {
	if !managedIdentityNeedsRealizedExposure(prepared) {
		return provisionAndVerifyManagedIdentity(ctx, out, prepared, exposed)
	}
	origins, err := existingManagedIdentityExposureOrigins(prepared)
	if err != nil {
		return err
	}
	if err := prepared.keycloak.SetApplicationOrigins(origins); err != nil {
		return err
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	fmt.Fprintln(out, "[PREPARED] identity bindings; public callback verification awaits application exposure.")
	return nil
}

func existingManagedIdentityExposureOrigins(prepared *managedIdentityExecution) ([]string, error) {
	if prepared.files.Dir == "" {
		return nil, fmt.Errorf("identity preparation requires the canonical application runtime owner")
	}
	owner := exposure.FilesFor(prepared.files)
	if _, err := os.Lstat(owner.Dir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("existing application exposure owner could not be inspected")
	}
	inventory, err := exposure.ExistingRuntimeInventory(prepared.manifest, prepared.files)
	if err != nil {
		return nil, fmt.Errorf("existing application exposure owner could not be verified")
	}
	// Preserve the actual protected existing public origins during reapply.
	// Never guess ports from the future desired exposure or rewrite its owner.
	return managedIdentityOriginsForRoutes(prepared, inventory.State.Host, inventory.State.Routes)
}

func verifyManagedIdentityAfterExposure(ctx context.Context, out io.Writer, prepared *managedIdentityExecution, exposed *managedExposureExecution) error {
	if !managedIdentityNeedsRealizedExposure(prepared) {
		return nil
	}
	if exposed == nil || exposed.driver == nil {
		return fmt.Errorf("managed identity requires its realized public exposure")
	}
	state := exposed.driver.State()
	for _, desired := range prepared.manifest.Exposures {
		if strings.EqualFold(strings.TrimSpace(desired.Visibility), "internal") {
			continue
		}
		matched := false
		for _, actual := range state.Routes {
			if actual.Name == desired.Name && actual.Service == desired.Service && actual.TargetPort == desired.Port && actual.Protocol == desired.Protocol && !strings.EqualFold(actual.Visibility, "internal") {
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("managed identity public exposure differs from its declared consumer route")
		}
	}
	// Origin changes invalidate Keycloak's earlier provisioning completion.
	// The same capability lifecycle reconciles existing client material and
	// verifies actual registered redirects/logout origins before emitting READY.
	return provisionAndVerifyManagedIdentity(ctx, out, prepared, exposed)
}
