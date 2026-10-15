package main

import (
	"context"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func resolveRepositoryLifecycleIntent(ctx context.Context, target deployment.ResolvedTarget, selection application.RepositoryEnvironmentSelection, command string) (application.RepositoryEnvironmentSelection, error) {
	m := selection.Manifest
	if !application.HasExplicitWorkload(m) {
		inspection, err := repositoryinspect.Inspect(ctx, selection.RepositoryRoot)
		if err != nil {
			return selection, err
		}
		if inspection.SelectedWorkloadSource != nil && inspection.SelectedWorkloadSource.Kind == repositoryinspect.WorkloadSourceCompose {
			analysis, err := repositoryinspect.AnalyzeComposeFile(selection.RepositoryRoot, inspection.SelectedWorkloadSource.Path)
			if err != nil {
				return selection, err
			}
			// Reuse the existing classifier, but consume only actual workload.
			// Detected infrastructure is never promoted to managed intent.
			if len(analysis.WorkloadServices) > 0 {
				m = application.WithWorkloadComponents(m, analysis.WorkloadServices...)
			}
		}
	}
	if err := m.Validate(); err != nil {
		return selection, &machine.Error{Code: machine.ErrorInvalidWorkload, CauseCode: "workload_missing", Message: "Application has no supported workload or runtime service.", Next: "Add a supported Compose source and run baha app init.", Cause: err}
	}
	owner := application.Store{Root: filepath.Join(selection.RepositoryRoot, ".baseharbor", "apps")}
	snapshot, err := repositoryIdentityClaims(target, selection.RepositoryRoot, owner, m)
	if err != nil {
		return selection, err
	}
	m, err = application.ResolveIdentity(m, snapshot)
	if err != nil {
		return selection, machine.Wrap(machine.ErrorOwnershipAmbiguous, err, "Reconcile the repository's existing application identity before continuing.", false)
	}
	if m.ApplicationID == "" {
		if command != "init" && command != "apply" && command != "up" {
			return selection, &machine.Error{Code: machine.ErrorCapabilityMissing, CauseCode: "application_initialization_required", Message: "Application identity has not been initialized.", Next: "Run baha app init once; the authored manifest will remain unchanged."}
		}
		if err := preflightRepositoryWorkload(resolvedApplication{Manifest: m, RepositoryRoot: selection.RepositoryRoot, FromRepository: true}); err != nil {
			return selection, err
		}
		stateRoot, err := deployment.TargetStateRoot(target.Name)
		if err != nil {
			return selection, err
		}
		if _, err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Load(); err != nil {
			return selection, err
		}
		if err := authorizeMCPOperation(ctx, "app.adopt", target.Name, m.Environment, "", selection.ManifestPath); err != nil {
			return selection, err
		}
		m, err = owner.InitializeIdentity(m, snapshot)
		if err != nil {
			return selection, err
		}
	}
	selection.Manifest = m
	return selection, nil
}

func repositoryIdentityClaims(target deployment.ResolvedTarget, root string, owner application.Store, m application.Manifest) (application.IdentitySnapshot, error) {
	snapshot := application.IdentitySnapshot{}
	items, err := owner.List()
	if err != nil {
		return snapshot, err
	}
	for _, item := range items {
		if item.Name == m.Name {
			snapshot.PreviouslyRegistered = true
			snapshot.ApplicationIDs = append(snapshot.ApplicationIDs, item.ApplicationID)
		}
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return snapshot, err
	}
	targets := map[string]bool{target.Name: true}
	for name := range cfg.Targets {
		targets[name] = true
	}
	for name := range targets {
		records, err := deployment.ListDeployments(name)
		if err != nil {
			return snapshot, err
		}
		for _, record := range records {
			if record.Source.Repository != "" && filepath.Clean(record.Source.Repository) == filepath.Clean(root) {
				snapshot.PreviouslyRegistered = true
				snapshot.ApplicationIDs = append(snapshot.ApplicationIDs, record.Identity.ApplicationID)
			}
		}
	}
	return snapshot, nil
}
