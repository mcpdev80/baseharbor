package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Restore first admits the current recovery plan, then reuses selective broker
// activation before authenticating the retained manager for service PKI.
func prepareRestoreRuntimeDependencies(ctx context.Context, resolved resolvedApplication, compose bhruntime.RuntimeProvider, restoreData applicationRestoreData, out io.Writer) (platformFiles bhruntime.Files, issuer serviceaccess.Issuer, err error) {
	m := resolved.Manifest
	if err := requireResolvedApplicationCore(ctx, resolved, applicationInput(ctx, runtimeInput), out); err != nil {
		return platformFiles, issuer, fmt.Errorf("restore prerequisite plan: %w", err)
	}
	if _, err := preflightRepositoryWorkloadSecurity(ctx, compose, resolved); err != nil {
		return platformFiles, issuer, fmt.Errorf("restore preflight workload security: %w", err)
	}
	if err := materializeRestoreIdentity(ctx, resolved, restoreData); err != nil {
		return platformFiles, issuer, fmt.Errorf("restore identity initialization: %w", err)
	}
	// Default retirement removes unneeded broker/Secrets runtime while retaining
	// its data and credentials. Reuse normal selective activation before the
	// manager login needed for service PKI and archive restoration.
	if err := materializeResolvedApplicationBroker(ctx, resolved, applicationInput(ctx, runtimeInput), out); err != nil {
		return platformFiles, issuer, fmt.Errorf("restore broker prerequisites: %w", err)
	}
	if m.Services.Secrets || application.RequiresRuntimeBroker(m) {
		platformFiles, err = existingTargetRuntimeFiles(ctx)
		if err != nil {
			return platformFiles, issuer, fmt.Errorf("restore preflight BaseHarbor control plane: %w", err)
		}
	}
	if requiresManagedServiceIssuer(m) {
		issuer, err = resolveApplicationServiceIssuer(ctx, resolved, compose)
		if err != nil {
			return platformFiles, issuer, fmt.Errorf("restore preflight managed service PKI: %w", err)
		}
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		if err := openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity); err != nil {
			return platformFiles, issuer, fmt.Errorf("restore preflight OpenBao provisioning: %w", err)
		}
	}
	return platformFiles, issuer, nil
}
