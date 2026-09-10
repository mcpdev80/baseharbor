package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type workloadServiceStatus struct {
	Service string
	State   string
	Health  string
	Ready   bool
}

type repositoryWorkloadStatus struct {
	Found    bool
	Workload application.WorkloadFiles
	Services []workloadServiceStatus
}

func inspectRepositoryWorkloadStatus(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (repositoryWorkloadStatus, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return repositoryWorkloadStatus{Found: found, Workload: workload}, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, err
	}
	if err := compose.ConfigProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...); err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, err
	}
	active, err := compose.ServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, err
	}
	expected := activeSelectedWorkloadServices(active, workload.Services)
	if len(expected) == 0 {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, fmt.Errorf("application workload has no active selected Compose services")
	}

	states, stateErr := compose.ServiceStatesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if stateErr == nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload, Services: buildWorkloadServiceStatuses(expected, states)}, nil
	}

	readyServices, err := compose.RunningServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, stateErr
	}
	fallback := make([]bhruntime.ServiceState, 0, len(readyServices))
	for _, service := range readyServices {
		fallback = append(fallback, bhruntime.ServiceState{Service: service, State: "running"})
	}
	return repositoryWorkloadStatus{Found: true, Workload: workload, Services: buildWorkloadServiceStatuses(expected, fallback)}, nil
}

func buildWorkloadServiceStatuses(expected []string, states []bhruntime.ServiceState) []workloadServiceStatus {
	byService := make(map[string]bhruntime.ServiceState, len(states))
	for _, state := range states {
		if strings.TrimSpace(state.Service) != "" {
			byService[state.Service] = state
		}
	}
	names := append([]string(nil), expected...)
	sort.Strings(names)
	result := make([]workloadServiceStatus, 0, len(names))
	for _, service := range names {
		state, ok := byService[service]
		if !ok {
			result = append(result, workloadServiceStatus{Service: service, State: "not running"})
			continue
		}
		result = append(result, workloadServiceStatus{
			Service: service,
			State:   normalizedWorkloadState(state.State),
			Health:  strings.ToLower(strings.TrimSpace(state.Health)),
			Ready:   state.Ready(),
		})
	}
	return result
}

func normalizedWorkloadState(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		return "unknown"
	}
	return state
}

func (status repositoryWorkloadStatus) Ready() bool {
	if !status.Found || len(status.Services) == 0 {
		return false
	}
	for _, service := range status.Services {
		if !service.Ready {
			return false
		}
	}
	return true
}

func (status repositoryWorkloadStatus) ReadyCount() int {
	count := 0
	for _, service := range status.Services {
		if service.Ready {
			count++
		}
	}
	return count
}

func formatWorkloadServiceStatus(service workloadServiceStatus) string {
	detail := service.State
	if service.Health != "" {
		detail += " health=" + service.Health
	}
	return detail
}
