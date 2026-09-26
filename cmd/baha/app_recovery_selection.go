package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type recoverySelectionArgs struct {
	Include []applicationbackup.RecoveryStateClass
	Exclude []applicationbackup.RecoveryStateClass
}

func extractRecoverySelectionArgs(args []string) ([]string, recoverySelectionArgs, error) {
	filtered := make([]string, 0, len(args))
	var selection recoverySelectionArgs
	for i := 0; i < len(args); i++ {
		value := args[i]
		var target *[]applicationbackup.RecoveryStateClass
		switch value {
		case "--include-state":
			target = &selection.Include
		case "--exclude-state":
			target = &selection.Exclude
		default:
			filtered = append(filtered, value)
			continue
		}
		i++
		if i >= len(args) || strings.TrimSpace(args[i]) == "" {
			return nil, recoverySelectionArgs{}, fmt.Errorf("%s requires a recovery state class", value)
		}
		class, err := applicationbackup.ParseRecoveryStateClass(args[i])
		if err != nil {
			return nil, recoverySelectionArgs{}, err
		}
		*target = append(*target, class)
	}
	return filtered, selection, nil
}

type recoveryWorkloadStorage struct {
	Logical string
	Project string
	Volume  string
}

type recoveryComposeModel struct {
	Services map[string]struct {
		Volumes []struct {
			Type   string `json:"type"`
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"volumes"`
	} `json:"services"`
	Volumes map[string]struct {
		Name     string `json:"name"`
		External bool   `json:"external"`
	} `json:"volumes"`
}

func discoverRecoveryWorkloadStorage(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) ([]applicationbackup.RecoveryContributor, []recoveryWorkloadStorage, error) {
	return resolveRecoveryWorkloadStorage(ctx, compose, resolved, files, true)
}

func resolveRecoveryWorkloadStorage(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles, requireMaterialized bool) ([]applicationbackup.RecoveryContributor, []recoveryWorkloadStorage, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return nil, nil, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return nil, nil, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return nil, nil, err
	}
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, nil, fmt.Errorf("render workload for recovery discovery: %w", err)
	}
	var model recoveryComposeModel
	if err := json.Unmarshal([]byte(rendered), &model); err != nil {
		return nil, nil, fmt.Errorf("decode workload recovery model: %w", err)
	}
	selected := map[string]struct{}{}
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	contributors := []applicationbackup.RecoveryContributor{}
	owned := []recoveryWorkloadStorage{}
	seen := map[string]struct{}{}
	for serviceName, service := range model.Services {
		if _, ok := selected[serviceName]; !ok {
			continue
		}
		for _, mount := range service.Volumes {
			source := strings.TrimSpace(mount.Source)
			if source == "" {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(mount.Type)) {
			case "bind":
				key := "bind:" + source
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				contributors = append(contributors, applicationbackup.RecoveryContributor{
					StateClass: applicationbackup.StateWorkloadStorage, LogicalResource: "bind:" + source,
					Ownership: "external", Support: applicationbackup.RecoveryExternal,
					ExplicitlyExcluded: true, Reason: "bind mounts remain outside BaseHarbor recovery ownership",
				})
			case "volume":
				definition, declared := model.Volumes[source]
				actual := strings.TrimSpace(definition.Name)
				if actual == "" {
					actual = workload.Project + "_" + source
				}
				key := "volume:" + actual
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				if declared && definition.External {
					contributors = append(contributors, applicationbackup.RecoveryContributor{
						StateClass: applicationbackup.StateWorkloadStorage, LogicalResource: source,
						Ownership: "external", Support: applicationbackup.RecoveryExternal,
						ExplicitlyExcluded: true, Durable: true,
						Reason: "external named volume remains outside BaseHarbor recovery ownership",
					})
					continue
				}
				if requireMaterialized {
					exists, inspectErr := compose.InspectProjectResource(ctx, workload.Project, bhruntime.ProjectResource{Kind: "volume", Name: actual})
					if inspectErr != nil {
						return nil, nil, fmt.Errorf("verify workload recovery volume %s ownership: %w", actual, inspectErr)
					}
					if !exists {
						return nil, nil, fmt.Errorf("workload recovery volume %s is declared but not materialized", actual)
					}
				}
				contributors = append(contributors, applicationbackup.RecoveryContributor{
					StateClass: applicationbackup.StateWorkloadStorage, LogicalResource: source,
					Ownership: "application", Support: applicationbackup.RecoverySupported,
					DefaultSelected: true, Durable: true,
				})
				owned = append(owned, recoveryWorkloadStorage{Logical: source, Project: workload.Project, Volume: actual})
			}
		}
	}
	sort.Slice(contributors, func(i, j int) bool { return contributors[i].LogicalResource < contributors[j].LogicalResource })
	sort.Slice(owned, func(i, j int) bool { return owned[i].Logical < owned[j].Logical })
	return contributors, owned, nil
}
