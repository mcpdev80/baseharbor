package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func preflightRepositoryWorkload(resolved resolvedApplication) error {
	if !resolved.FromRepository {
		return nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
	_, _, err := application.ResolveWorkloadCompose(repositoryRoot, resolved.Manifest)
	return err
}

func materializeRepositoryWorkload(resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadFiles, bool, error) {
	if !resolved.FromRepository {
		return application.WorkloadFiles{}, false, nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
	return application.MaterializeWorkload(repositoryRoot, resolved.Manifest, files)
}

func applyRepositoryWorkload(ctx context.Context, out io.Writer, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return false, err
	}
	composeFiles := []string{workload.Compose, workload.Override}
	if err := compose.ConfigProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...); err != nil {
		return false, fmt.Errorf("validate application workload Compose integration: %w", err)
	}
	activeServices, err := compose.ServicesProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("resolve application workload services: %w", err)
	}
	if len(activeServices) == 0 {
		return false, fmt.Errorf("application workload has no active Compose services")
	}
	if err := compose.UpProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...); err != nil {
		return false, fmt.Errorf("start application workload: %w", err)
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var running []string
	for verifyCtx.Err() == nil {
		running, err = compose.RunningServicesProjectFiles(verifyCtx, workload.Project, workload.RepositoryRoot, composeFiles...)
		if err == nil && workloadRunningEnough(activeServices, running, resolved.Manifest.Workload.Services) {
			fmt.Fprintf(out, "[OK] workload          %d Compose service(s) running on BaseHarbor backend network\n", len(running))
			fmt.Fprintf(out, "Workload Compose: %s\n", workload.Compose)
			return true, nil
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		return false, fmt.Errorf("verify application workload: %w", err)
	}
	return false, fmt.Errorf("application workload did not reach the expected running service set; active=%v running=%v", activeServices, running)
}

func stopRepositoryWorkload(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return false, err
	}
	composeFiles := []string{workload.Compose, workload.Override}
	if err := compose.ConfigProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...); err != nil {
		return false, fmt.Errorf("validate application workload before stop: %w", err)
	}
	if err := compose.DownProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...); err != nil {
		return false, fmt.Errorf("stop application workload: %w", err)
	}
	running, err := compose.RunningServicesProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("verify application workload stopped: %w", err)
	}
	if len(running) != 0 {
		return false, fmt.Errorf("verify application workload stopped: services still running: %v", running)
	}
	return true, nil
}

func inspectRepositoryWorkload(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadFiles, []string, bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return workload, nil, found, err
	}
	composeFiles := []string{workload.Compose, workload.Override}
	if err := compose.ConfigProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...); err != nil {
		return workload, nil, true, err
	}
	running, err := compose.RunningServicesProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...)
	return workload, running, true, err
}

func workloadRunningEnough(active, running, requested []string) bool {
	runningSet := make(map[string]struct{}, len(running))
	for _, service := range running {
		runningSet[service] = struct{}{}
	}
	if len(requested) > 0 {
		for _, service := range requested {
			if _, ok := runningSet[service]; !ok {
				return false
			}
		}
		return true
	}
	if len(running) == 0 {
		return false
	}
	activeSet := make(map[string]struct{}, len(active))
	for _, service := range active {
		activeSet[service] = struct{}{}
	}
	for service := range runningSet {
		if _, ok := activeSet[service]; !ok {
			return false
		}
	}
	return true
}
