package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

type fullDestroyExcludeRemoteTargetsKey struct{}

func withFullDestroyExcludeRemoteTargets(ctx context.Context) context.Context {
	return context.WithValue(ctx, fullDestroyExcludeRemoteTargetsKey{}, true)
}

func discoverFullDestroyTargets(contexts ...context.Context) ([]deployment.ResolvedTarget, []fullDestroyResult) {
	var results []fullDestroyResult
	excludeRemote := false
	if len(contexts) > 0 && contexts[0] != nil {
		excludeRemote, _ = contexts[0].Value(fullDestroyExcludeRemoteTargetsKey{}).(bool)
	}
	cfg, cfgErr := deployment.LoadConfig()
	if cfgErr != nil {
		results = append(results, fullDestroyResult{Status: "FAILED", Resource: "config", Detail: cfgErr.Error()})
		cfg = deployment.Config{Version: deployment.ConfigVersion, Access: map[string]deployment.AccessDefinition{}, Targets: map[string]deployment.TargetDefinition{}}
	}

	byName := map[string]deployment.ResolvedTarget{}
	for _, name := range cfg.TargetNames() {
		target, err := cfg.ResolveTarget(name, "")
		if err != nil {
			results = append(results, fullDestroyResult{Status: "FAILED", Target: name, Resource: "target-config", Detail: err.Error()})
			continue
		}
		if excludeRemote && target.AccessProvider != "" && target.AccessProvider != "local" {
			continue
		}
		byName[name] = target
	}

	root, err := deployment.DataRoot()
	if err != nil {
		results = append(results, fullDestroyResult{Status: "FAILED", Resource: "data-root", Detail: err.Error()})
	} else {
		entries, readErr := os.ReadDir(filepath.Join(root, "targets"))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			results = append(results, fullDestroyResult{Status: "FAILED", Resource: "target-state", Detail: readErr.Error()})
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if excludeRemote && isConfiguredRemoteTarget(cfg, root, name) {
				continue
			}
			if _, ok := byName[name]; ok {
				continue
			}
			if err := deployment.ValidateTargetName(name); err != nil {
				results = append(results, fullDestroyResult{Status: "SKIPPED", Target: name, Resource: "target-state", Detail: "invalid target directory name; local BaseHarbor data root will still be removed"})
				continue
			}
			populateFullDestroyTargetFromDataRoot(root, name, byName, &results)
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	targets := make([]deployment.ResolvedTarget, 0, len(names))
	for _, name := range names {
		targets = append(targets, byName[name])
	}
	return targets, results
}

func isConfiguredRemoteTarget(cfg deployment.Config, root, name string) bool {
	if _, err := os.Lstat(filepath.Join(root, "remote-connections", name, "profile.json")); err == nil {
		return true
	}
	if definition, ok := cfg.Targets[name]; ok {
		access := cfg.Access[definition.Access.Reference]
		return access.Provider != "" && access.Provider != "local"
	}
	return false
}

func discoverFullDestroyDeployments(contexts ...context.Context) ([]deployment.DeploymentRecord, []fullDestroyResult) {
	records, warnings, err := deployment.ListAllDeploymentsForDisplay()
	if err != nil {
		return nil, []fullDestroyResult{{Status: "FAILED", Resource: "deployment-registry", Detail: err.Error()}}
	}

	excludeRemote := len(contexts) > 0 && contexts[0] != nil
	if excludeRemote {
		excludeRemote, _ = contexts[0].Value(fullDestroyExcludeRemoteTargetsKey{}).(bool)
	}
	config := deployment.Config{}
	if excludeRemote {
		config, err = deployment.LoadConfig()
		if err != nil {
			return nil, []fullDestroyResult{{Status: "FAILED", Resource: "config", Detail: err.Error()}}
		}
	}
	root, rootErr := deployment.DataRoot()
	if excludeRemote && rootErr != nil {
		return nil, []fullDestroyResult{{Status: "FAILED", Resource: "data-root", Detail: rootErr.Error()}}
	}
	filtered := records[:0]
	for _, record := range records {
		if excludeRemote && isConfiguredRemoteTarget(config, root, record.Identity.Target) {
			continue
		}
		filtered = append(filtered, record)
	}
	results := fullDestroyDeploymentWarnings(warnings)
	return filtered, results
}

func fullDestroyDeploymentWarnings(warnings []error) []fullDestroyResult {
	var results []fullDestroyResult
	for _, warning := range warnings {
		stateErr, ok := deployment.DeploymentRecordState(warning)
		if !ok {
			results = append(results, fullDestroyResult{Status: "FAILED", Resource: "deployment-registry", Detail: warning.Error()})
			continue
		}
		resource := "deployment " + stateErr.Identity.DeploymentID
		if stateErr.Identity.DeploymentID == "" {
			resource = "deployment unknown"
		}
		results = append(results, fullDestroyResult{
			Status: "SKIPPED", Target: stateErr.Identity.Target, Resource: resource,
			Detail:     "incomplete deployment state has no usable deployment.json; refusing to guess semantic/runtime ownership, local BaseHarbor state will still be removed",
			Diagnostic: "unregistered-deployment-state",
		})
	}
	return results
}
