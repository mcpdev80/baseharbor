package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	repositoryWorkloadReadinessTimeout      = 2 * time.Minute
	repositoryWorkloadReadinessPollInterval = time.Second
)

func preflightRepositoryWorkload(resolved resolvedApplication) error {
	if !resolved.FromRepository {
		return nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
	_, _, err := application.ResolveWorkloadCompose(repositoryRoot, resolved.Manifest)
	return err
}

func preflightRepositoryWorkloadSecurity(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (application.WorkloadSecurityReport, error) {
	if !resolved.FromRepository {
		return application.WorkloadSecurityReport{}, nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
	selected, composePath, found, err := application.SelectedWorkloadServices(repositoryRoot, resolved.Manifest)
	if err != nil || !found {
		return application.WorkloadSecurityReport{}, err
	}
	environment := workloadSecurityPreflightEnvironment(resolved.Manifest)
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, application.WorkloadProjectName(resolved.Manifest), repositoryRoot, environment, composePath)
	if err != nil {
		return application.WorkloadSecurityReport{}, fmt.Errorf("render repository Compose for security preflight: %w", err)
	}
	report, err := application.AnalyzeRenderedComposeSecurity(resolved.Manifest, []byte(rendered))
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, service := range selected {
		selectedSet[service] = struct{}{}
	}
	filtered := report.Findings[:0]
	for _, finding := range report.Findings {
		if _, ok := selectedSet[finding.Service]; ok {
			filtered = append(filtered, finding)
		}
	}
	report.Findings = filtered
	return report, report.Error()
}

func workloadSecurityPreflightEnvironment(m application.Manifest) map[string]string {
	required := application.RequiredSecretNames(m)
	if len(required) == 0 {
		return nil
	}
	environment := make(map[string]string, len(required))
	for _, name := range required {
		if application.RequiredSecretUsesFileBinding(name) {
			environment[name] = "/run/baseharbor/preflight/" + name
			continue
		}
		environment[name] = "baseharbor-preflight-secret"
	}
	return environment
}

func preflightResolvedRepositoryWorkloadSecurity(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadSecurityReport, error) {
	if !resolved.FromRepository {
		return application.WorkloadSecurityReport{}, nil
	}
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return application.WorkloadSecurityReport{}, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	return analyzeResolvedRepositoryWorkloadSecurity(ctx, compose, resolved, workload, environment, composeFiles)
}

func analyzeResolvedRepositoryWorkloadSecurity(
	ctx context.Context,
	compose bhruntime.Compose,
	resolved resolvedApplication,
	workload application.WorkloadFiles,
	environment map[string]string,
	composeFiles []string,
) (application.WorkloadSecurityReport, error) {
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return application.WorkloadSecurityReport{}, fmt.Errorf("render resolved repository Compose for security preflight: %w", err)
	}
	report, err := application.AnalyzeRenderedComposeSecurity(resolved.Manifest, []byte(rendered))
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	selectedSet := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selectedSet[service] = struct{}{}
	}
	filtered := report.Findings[:0]
	for _, finding := range report.Findings {
		if _, ok := selectedSet[finding.Service]; ok {
			filtered = append(filtered, finding)
		}
	}
	report.Findings = filtered
	return report, report.Error()
}

func printWorkloadSecurityFindings(out io.Writer, report application.WorkloadSecurityReport) {
	for _, finding := range report.Findings {
		switch finding.Decision {
		case application.WorkloadSecurityWarn:
			fmt.Fprintf(out, "[WARN] workload-security  %s/%s: %s\n", finding.Service, finding.Code, finding.Message)
		case application.WorkloadSecurityAllow:
			fmt.Fprintf(out, "[ALLOW] workload-security %s/%s explicitly acknowledged for development\n", finding.Service, finding.Code)
		}
	}
}

func materializeRepositoryWorkload(resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadFiles, bool, error) {
	if !resolved.FromRepository {
		return application.WorkloadFiles{}, false, nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
	return application.MaterializeWorkload(repositoryRoot, resolved.Manifest, files)
}

type renderedComposeConfig struct {
	Services map[string]struct {
		Environment map[string]any `json:"environment"`
	} `json:"services"`
}

func repositoryWorkloadBindingPlan(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, workload application.WorkloadFiles, environment map[string]string) (application.WorkloadBindingPlan, error) {
	baseFiles := []string{workload.Compose, workload.Override}
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, baseFiles...)
	if err != nil {
		return application.WorkloadBindingPlan{}, fmt.Errorf("render application workload for binding discovery: %w", err)
	}
	var config renderedComposeConfig
	if err := json.Unmarshal([]byte(rendered), &config); err != nil {
		return application.WorkloadBindingPlan{}, fmt.Errorf("decode rendered application workload: %w", err)
	}
	selected := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	plan := application.WorkloadBindingPlan{FileSecretsByService: map[string][]string{}}
	runtimePermissionServices := map[string]struct{}{}
	for _, service := range application.RuntimeAuthorizedServices(resolved.Manifest) {
		runtimePermissionServices[service] = struct{}{}
	}
	for service, definition := range config.Services {
		if _, ok := selected[service]; !ok {
			continue
		}
		_, explicitlyAuthorized := runtimePermissionServices[service]
		_, legacyRuntimeBinding := definition.Environment["BASEHARBOR_RUNTIME_TOKEN_FILE"]
		if explicitlyAuthorized || legacyRuntimeBinding {
			plan.RuntimeIdentityServices = append(plan.RuntimeIdentityServices, service)
		}
		for _, requirement := range resolved.Manifest.Secrets.Required {
			if !application.RequiredSecretUsesFileBinding(requirement.Name) {
				continue
			}
			if _, ok := definition.Environment[requirement.Name]; ok {
				plan.FileSecretsByService[service] = append(plan.FileSecretsByService[service], requirement.Name)
			}
		}
	}
	sort.Strings(plan.RuntimeIdentityServices)
	for service := range plan.FileSecretsByService {
		sort.Strings(plan.FileSecretsByService[service])
	}
	return plan, nil
}

func repositoryWorkloadComposeFiles(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, workload application.WorkloadFiles, files application.RuntimeFiles, environment map[string]string) ([]string, error) {
	composeFiles := []string{workload.Compose, workload.Override}
	plan, err := repositoryWorkloadBindingPlan(ctx, compose, resolved, workload, environment)
	if err != nil {
		return nil, err
	}
	runtimeIdentityOverride, enabled, err := application.MaterializeRuntimeIdentityWorkloadOverride(resolved.Manifest, workload, files, plan)
	if err != nil {
		return nil, err
	}
	if enabled {
		composeFiles = append(composeFiles, runtimeIdentityOverride)
	}
	loggingOverride, enabled, err := logsprovider.ExistingWorkloadOverride(files)
	if err != nil {
		return nil, err
	}
	if enabled {
		composeFiles = append(composeFiles, loggingOverride)
	}
	return composeFiles, nil
}

func repositoryWorkloadEnvironment(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles) (map[string]string, error) {
	environment := map[string]string{}
	if err := mergePersistedWorkloadPortOverrides(environment, files); err != nil {
		return nil, err
	}
	if runtimeURL, configured, err := application.ConfiguredRuntimeAPIURL(); err != nil {
		return nil, err
	} else if configured {
		environment["BASEHARBOR_RUNTIME_API_URL"] = runtimeURL
		environment["BASEHARBOR_RUNTIME_TOKEN_FILE"] = application.RuntimeIdentityContainerTokenPath
	}
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
		if application.RequiredSecretUsesFileBinding(requirement.Name) {
			path := application.SecretFileHostPath(files, requirement.Name)
			if err := writeWorkloadSecretFile(path, value); err != nil {
				return nil, fmt.Errorf("materialize required workload secret file %s: %w", requirement.Name, err)
			}
			environment[requirement.Name] = application.SecretFileContainerPath(requirement.Name)
			continue
		}
		if strings.IndexByte(string(value), 0) >= 0 {
			return nil, fmt.Errorf("required secret %q contains a NUL byte and cannot be projected to a process environment", requirement.Name)
		}
		environment[requirement.Name] = string(value)
	}
	return environment, nil
}

func writeWorkloadSecretFile(path string, value []byte) error {
	if len(value) == 0 {
		return fmt.Errorf("secret value is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(value)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(path, 0o600)
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
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return false, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return false, err
	}
	if _, err := analyzeResolvedRepositoryWorkloadSecurity(ctx, compose, resolved, workload, environment, composeFiles); err != nil {
		return false, fmt.Errorf("workload security preflight before start: %w", err)
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
	beforeStates, err := compose.ServiceStatesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("inspect application workload before start: %w", err)
	}
	beforeServices := make(map[string]struct{}, len(beforeStates))
	for _, state := range beforeStates {
		beforeServices[state.Service] = struct{}{}
	}
	cleanupNewResources := func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if len(beforeStates) == 0 {
			_ = compose.DownProjectFilesEnv(cleanupCtx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
			return
		}
		var newlyCreated []string
		for _, service := range expectedServices {
			if _, existed := beforeServices[service]; !existed {
				newlyCreated = append(newlyCreated, service)
			}
		}
		if len(newlyCreated) > 0 {
			_ = compose.StopProjectFilesSelected(cleanupCtx, workload.Project, workload.RepositoryRoot, environment, newlyCreated, composeFiles...)
		}
	}
	startServices := []string(nil)
	if workload.Partial || len(resolved.Manifest.Workload.Services) > 0 {
		startServices = expectedServices
	}
	if err := startRepositoryWorkloadWithPortFallback(ctx, runtimeInput, out, compose, workload, files, environment, startServices, composeFiles); err != nil {
		cleanupNewResources()
		return false, fmt.Errorf("start application workload: %w", err)
	}

	fmt.Fprintf(out, "[WAIT] workload          waiting up to %s for service and HTTP/TLS readiness\n", repositoryWorkloadReadinessTimeout)
	verifyCtx, cancel := context.WithTimeout(ctx, repositoryWorkloadReadinessTimeout)
	defer cancel()
	var lastStatus repositoryWorkloadStatus
	var lastErr error
	for verifyCtx.Err() == nil {
		lastStatus, lastErr = inspectRepositoryWorkloadStatus(verifyCtx, compose, resolved, files)
		if lastErr == nil && lastStatus.Found && lastStatus.Ready() {
			fmt.Fprintf(out, "[READY] workload         %d Compose service(s) ready\n", len(expectedServices))
			if len(lastStatus.Exposures) > 0 {
				fmt.Fprintf(out, "[READY] exposure         %d/%d published HTTP/TLS endpoint(s) ready\n", lastStatus.ExposureReadyCount(), len(lastStatus.Exposures))
			}
			fmt.Fprintf(out, "Workload Compose: %s\n", workload.Compose)
			return true, nil
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(repositoryWorkloadReadinessPollInterval):
		}
	}
	cleanupNewResources()
	if lastErr != nil {
		return false, fmt.Errorf("verify application workload readiness after %s: %w", repositoryWorkloadReadinessTimeout, lastErr)
	}
	return false, fmt.Errorf("application workload did not reach readiness within %s; services=%d/%d exposures=%d/%d", repositoryWorkloadReadinessTimeout, lastStatus.ReadyCount(), len(expectedServices), lastStatus.ExposureReadyCount(), len(lastStatus.Exposures))
}

func stopRepositoryWorkload(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return false, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return false, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
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
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return workload, nil, true, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
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
	status, err := inspectRepositoryWorkloadStatus(ctx, compose, resolved, files)
	if err != nil || !status.Found {
		return status.ReadyCount(), status.Found, err
	}
	if !status.Ready() {
		return status.ReadyCount(), true, fmt.Errorf("application workload is not ready; services=%d/%d exposures=%d/%d", status.ReadyCount(), len(status.Services), status.ExposureReadyCount(), len(status.Exposures))
	}
	return status.ReadyCount(), true, nil
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
