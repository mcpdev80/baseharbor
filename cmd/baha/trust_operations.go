package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
)

type managedTrustResult struct {
	CoreState string                     `json:"core_state,omitempty"`
	Next      string                     `json:"next,omitempty"`
	Anchors   []hosttrust.RecordedStatus `json:"anchors,omitempty"`
	Managed   bool                       `json:"managed"`
	Status    *hosttrust.Status          `json:"status,omitempty"`
	Path      string                     `json:"path,omitempty"`
}

func inspectManagedTrust(ctx context.Context) (managedTrustResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "trust.status", "", "", ""); err != nil {
		return managedTrustResult{}, err
	}
	// Inspect state before runtime detection: an uninstalled Core needs no engine.
	state, err := managedTrustCoreState(ctx)
	if err != nil {
		return managedTrustResult{}, err
	}
	if state != "installed" {
		dataDir, err := bhruntime.DataDir("")
		if err != nil {
			return managedTrustResult{}, err
		}
		anchors, err := hosttrust.InspectRecorded(dataDir)
		if err != nil {
			return managedTrustResult{}, err
		}
		return managedTrustResult{CoreState: state, Anchors: anchors, Next: "Use baha target list to select a Core, or baha up to initialize it. Retained owned anchors can be reviewed with baha trust uninstall."}, nil
	}
	bundle, managed, err := currentManagedTrustBundle(ctx)
	if err != nil {
		return managedTrustResult{}, err
	}
	result := managedTrustResult{Managed: managed, CoreState: "installed"}
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

type trustUninstallResult struct {
	ContractVersion string `json:"contract_version"`
	Removed         int    `json:"removed"`
}

func ownedTrustRecords() ([]hosttrust.AnchorRecord, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return nil, err
	}
	records, err := hosttrust.StateRecords(dataDir)
	if records == nil {
		records = []hosttrust.AnchorRecord{}
	}
	return records, err
}

func uninstallManagedTrust(ctx context.Context, approved bool) (trustUninstallResult, error) {
	result := trustUninstallResult{ContractVersion: "v1"}
	if err := authorizeCurrentMCPContext(ctx, "trust.uninstall", "", "", ""); err != nil {
		return result, err
	}
	if err := applicationlifecycle.RequireApproval("trust.uninstall", approved); err != nil {
		return result, err
	}
	// This mutation is host-local and independent of OpenBao availability:
	// an old issuer may no longer exist, yet its recorded anchor must be removable.
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return result, err
	}
	removed, err := hosttrust.RemoveOwned(ctx, dataDir)
	result.Removed = removed
	return result, err
}

func managedTrustCoreState(ctx context.Context) (string, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return "", err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return "", err
	}
	_, err = existingTargetRuntimeFiles(ctx)
	if err == nil {
		return "installed", nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("Core runtime state is unreadable; run baha doctor --verbose")
	}
	// Missing one file in a retained runtime is an incomplete installation.
	for _, name := range []string{"compose.yaml", "runtime.env", "topology.json"} {
		_, statErr := os.Stat(filepath.Join(root, name))
		if statErr == nil {
			return "incomplete", nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", errors.New("Core runtime state is unreadable; run baha doctor --verbose")
		}
	}
	return "not_installed", nil
}
