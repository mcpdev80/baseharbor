package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
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

func repositoryWorkloadComposeFiles(resolved resolvedApplication, workload application.WorkloadFiles, files application.RuntimeFiles) ([]string, error) {
	composeFiles := []string{workload.Compose, workload.Override}
	runtimeIdentityOverride, enabled, err := application.MaterializeRuntimeIdentityWorkloadOverride(resolved.Manifest, workload, files)
	if err != nil {
		return nil, err
	}
	if enabled {
		composeFiles = append(composeFiles, runtimeIdentityOverride)
	}
	return composeFiles, nil
}

func repositoryWorkloadEnvironment(ctx context.Context, resolved resolvedApplication) (map[string]string, error) {
	environment := map[string]string{}
	if len(resolved.Manifest.Secrets.Required) == 0 {
		return environment, nil
	}
	service := applicationsecret.New(resolved.Store)
	for _, requirement := range resolved.Manifest.Secrets.Required {
		if !validWorkloadEnvironmentName(requirement.Name) {
			return nil, fmt.Errorf("required secret %q cannot be projected as a workload environment variable; use an environment-compatible secret name", requirement.Name)
		}
		value, err := service.Get(ctx, resolved.Manifest.Name, requirement.Name)
		if err != nil {
			return nil, fmt.Errorf("resolve required workload secret %s: %w", requirement.Name, err)
		}
		if strings.IndexByte(string(value), 0) >= 0 {
			return nil, fmt.Errorf("required secret %q contains a NUL byte and cannot be projected to a process environment", requirement.Name)
		}
		environment[requirement.Name] = string(value)
	}
	return environment, nil
}

func validWorkloadEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_') {
				return false
			}
			continue
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func activeSelectedWorkloadServices(active, selected []string) []string {
	activeSet := make(map[string]struct{}, len(active))
	for _, service := range active {
		activeSet[service] = struct{}{}
	}
	result := make([]string, 0, len(selected))
	for _, service := range selected {
		if _, ok := activeSet[service]; ok {
			result = append(result, service)
		}
	}
	return result
}

func applyRepositoryWorkload(ctx context.Context, out io.Writer, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return false, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(resolved, workload, files)
	if err != nil {
		return false, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved)
	if err != nil {
		return false, err
	}
	if err := compose.ConfigProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...); err != nil {
		return false, fmt.Errorf("validate application workload Compose integration: %w", err)
	}
	activeServices, err := compose.ServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("resolve application workload services: %w", err)
	}
	expectedServices := activeSelectedWorkloadServices(activeServices, workload.Services)
	if len(expectedServices) == 0 {
		return false, fmt.Errorf("application workload has no active selected Compose services")
	}
	startServices := []string(nil)
	if workload.Partial || len(resolved.Manifest.Workload.Services) > 0 {
		startServices = expectedServices
	}
	if err := compose.UpProjectFilesSelected(ctx, workload.Project, workload.RepositoryRoot, environment, startServices, composeFiles...); err != nil {
		return false, fmt.Errorf("start application workload: %w", err)
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var running []string
	for verifyCtx.Err() == nil {
		running, err = compose.RunningServicesProjectFilesEnv(verifyCtx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
		if err == nil && workloadRunningEnough(activeServices, running, expectedServices) {
			fmt.Fprintf(out, "[OK] workload          %d Compose service(s) running on BaseHarbor backend network\n", len(expectedServices))
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
	return false, fmt.Errorf("application workload did not reach the expected running service set; expected=%v running=%v", expectedServices, running)
}

func stopRepositoryWorkload(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return false, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(resolved, workload, files)
	if err != nil {
		return false, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved)
	if err != nil {
		return false, err
	}
	if err := compose.ConfigProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...); err != nil {
		return false, fmt.Errorf("validate application workload before stop: %w", err)
	}
	activeServices, err := compose.ServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("resolve active application workload services before stop: %w", err)
	}
	expectedServices := activeSelectedWorkloadServices(activeServices, workload.Services)
	if workload.Partial || len(resolved.Manifest.Workload.Services) > 0 {
		if err := compose.StopProjectFilesSelected(ctx, workload.Project, workload.RepositoryRoot, environment, expectedServices, composeFiles...); err != nil {
			return false, fmt.Errorf("stop selected application workload services: %w", err)
		}
	} else if err := compose.DownProjectFiles(ctx, workload.Project, workload.RepositoryRoot, composeFiles...); err != nil {
		return false, fmt.Errorf("stop application workload: %w", err)
	}
	running, err := compose.RunningServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("verify application workload stopped: %w", err)
	}
	for _, service := range expectedServices {
		for _, active := range running {
			if service == active {
				return false, fmt.Errorf("verify application workload stopped: selected service %s is still running", service)
			}
		}
	}
	return true, nil
}

func inspectRepositoryWorkload(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadFiles, []string, bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return workload, nil, found, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(resolved, workload, files)
	if err != nil {
		return workload, nil, true, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved)
	if err != nil {
		return workload, nil, true, err
	}
	if err := compose.ConfigProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...); err != nil {
		return workload, nil, true, err
	}
	running, err := compose.RunningServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	return workload, running, true, err
}

func checkRepositoryWorkloadReady(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (int, bool, error) {
	workload, running, found, err := inspectRepositoryWorkload(ctx, compose, resolved, files)
	if err != nil || !found {
		return len(running), found, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(resolved, workload, files)
	if err != nil {
		return len(running), true, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved)
	if err != nil {
		return len(running), true, err
	}
	active, err := compose.ServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return len(running), true, err
	}
	expected := activeSelectedWorkloadServices(active, workload.Services)
	if !workloadRunningEnough(active, running, expected) {
		return len(running), true, fmt.Errorf("application workload is not ready; expected=%v running=%v", expected, running)
	}
	return len(running), true, nil
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
