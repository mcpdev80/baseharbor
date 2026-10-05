package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type managedTrustResult struct {
	Managed bool              `json:"managed"`
	Status  *hosttrust.Status `json:"status,omitempty"`
	Path    string            `json:"path,omitempty"`
}

func inspectManagedTrust(ctx context.Context) (managedTrustResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "trust.status", "", "", ""); err != nil {
		return managedTrustResult{}, err
	}
	bundle, managed, err := currentManagedTrustBundle(ctx)
	if err != nil {
		return managedTrustResult{}, err
	}
	result := managedTrustResult{Managed: managed}
	if !managed {
		return result, nil
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return result, err
	}
	status, err := hosttrust.Inspect(dataDir, bundle.PEM)
	result.Status = &status
	return result, err
}
func exportManagedTrust(ctx context.Context, path string) (managedTrustResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "trust.export", "", "", ""); err != nil {
		return managedTrustResult{}, err
	}
	bundle, managed, err := currentManagedTrustBundle(ctx)
	if err != nil {
		return managedTrustResult{}, err
	}
	if !managed {
		return managedTrustResult{}, errors.New("active trust root is operator-owned; BaseHarbor will not export or claim it")
	}
	if err := hosttrust.Export(path, bundle.PEM); err != nil {
		return managedTrustResult{}, err
	}
	return managedTrustResult{Managed: true, Path: path}, nil
}
func installManagedTrust(ctx context.Context, approved bool) (managedTrustResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "trust.install", "", "", ""); err != nil {
		return managedTrustResult{}, err
	}
	if err := applicationlifecycle.RequireApproval("trust.install", approved); err != nil {
		return managedTrustResult{}, err
	}
	bundle, managed, err := currentManagedTrustBundle(ctx)
	if err != nil {
		return managedTrustResult{}, err
	}
	if !managed {
		return managedTrustResult{}, errors.New("active trust root is operator-owned; BaseHarbor will not install or claim it")
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return managedTrustResult{}, err
	}
	status, err := hosttrust.Install(ctx, dataDir, bundle.PEM, bundle.IssuerReference, nil)
	return managedTrustResult{Managed: true, Status: &status}, err
}
