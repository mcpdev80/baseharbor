package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/cli"
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
	repositoryRoot := resolved.repositoryRoot()
	_, _, err := application.ResolveWorkloadCompose(repositoryRoot, resolved.Manifest)
	return err
}

func preflightRepositoryWorkloadSecurity(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (application.WorkloadSecurityReport, error) {
	if !resolved.FromRepository {
		return application.WorkloadSecurityReport{}, nil
	}
	repositoryRoot := resolved.repositoryRoot()
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
	repositoryRoot := resolved.repositoryRoot()
	return application.MaterializeWorkload(repositoryRoot, resolved.Manifest, files)
}

type renderedComposeConfig struct {
	Services map[string]struct {
		Environment map[string]any `json:"environment"`
	} `json:"services"`
}

func repositoryWorkloadBindingPlan(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, workload application.WorkloadFiles, environment map[string]string) (application.WorkloadBindingPlan, error) {
	// Binding discovery inspects the repository-owned Compose model only.
	// BaseHarbor-generated overrides add platform bindings after discovery and
	// must not change which bindings the application itself declared.
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, workload.Compose)
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

func repositoryWorkloadStopEnvironment(resolved resolvedApplication, files application.RuntimeFiles) (map[string]string, error) {
	environment := workloadSecurityPreflightEnvironment(resolved.Manifest)
	if environment == nil {
		environment = map[string]string{}
	}
	if err := mergeRepositoryDeploymentWorkloadPorts(environment, resolved); err != nil {
		return nil, err
	}
	if err := mergePersistedWorkloadPortOverrides(environment, files); err != nil {
		return nil, err
	}
	if runtimeURL, configured, err := application.ConfiguredRuntimeAPIURL(); err != nil {
		return nil, err
	} else if configured {
		environment["BASEHARBOR_RUNTIME_API_URL"] = runtimeURL
		environment["BASEHARBOR_RUNTIME_TOKEN_FILE"] = application.RuntimeIdentityContainerTokenPath
	}
	return environment, nil
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
	if len(resolved.Manifest.Secrets.Required) == 0 && len(resolved.Manifest.Secrets.Optional) == 0 {
		return environment, nil
	}
	service := applicationsecret.New(resolved.Store)
	requiredNames := application.RequiredSecretNames(resolved.Manifest)
	if len(requiredNames) > 0 {
		values, err := service.GetMany(ctx, resolved.Manifest.Name, requiredNames)
		if err != nil {
			return nil, fmt.Errorf("resolve required workload secrets: %w", err)
		}
		for _, requirement := range resolved.Manifest.Secrets.Required {
			value, ok := values[requirement.Name]
			if !ok {
				return nil, fmt.Errorf("resolve required workload secret %s: missing from batch result", requirement.Name)
			}
			if err := projectWorkloadSecret(environment, files, requirement.Name, value, true); err != nil {
				return nil, err
			}
		}
	}

	if len(resolved.Manifest.Secrets.Optional) > 0 {
		metadata, err := service.List(ctx, resolved.Manifest.Name)
		if err != nil {
			return nil, fmt.Errorf("inspect optional workload secrets: %w", err)
		}
		present := map[string]struct{}{}
		for _, item := range metadata {
			if item.Present && item.Usable && !item.Required {
				present[item.Name] = struct{}{}
			}
		}
		var names []string
		for _, requirement := range resolved.Manifest.Secrets.Optional {
			if _, ok := present[requirement.Name]; ok {
				names = append(names, requirement.Name)
			}
		}
		if len(names) > 0 {
			values, err := service.GetMany(ctx, resolved.Manifest.Name, names)
			if err != nil {
				return nil, fmt.Errorf("resolve optional workload secrets: %w", err)
			}
			for _, name := range names {
				if err := projectWorkloadSecret(environment, files, name, values[name], false); err != nil {
					return nil, err
				}
			}
		}
	}
	return environment, nil
}

func projectWorkloadSecret(environment map[string]string, files application.RuntimeFiles, name string, value []byte, required bool) error {
	label := "optional"
	if required {
		label = "required"
	}
	if !validWorkloadEnvironmentName(name) {
		return fmt.Errorf("%s secret %q cannot be projected as a workload environment variable; use an environment-compatible secret name", label, name)
	}
	if application.RequiredSecretUsesFileBinding(name) {
		path := application.SecretFileHostPath(files, name)
		if err := writeWorkloadSecretFile(path, value); err != nil {
			return fmt.Errorf("materialize %s workload secret file %s: %w", label, name, err)
		}
		environment[name] = application.SecretFileContainerPath(name)
		return nil
	}
	if strings.IndexByte(string(value), 0) >= 0 {
		return fmt.Errorf("%s secret %q contains a NUL byte and cannot be projected to a process environment", label, name)
	}
	environment[name] = string(value)
	return nil
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
	if len(beforeStates) == 0 {
		if err := preflightRepositoryWorkloadPublishedPorts(ctx, runtimeInput, out, workload, files, environment); err != nil {
			return false, err
		}
	}

	buildFingerprints, err := resolveRepositoryWorkloadBuildFingerprints(ctx, compose, workload, environment, expectedServices, composeFiles)
	if err != nil {
		return false, fmt.Errorf("resolve application workload build identity: %w", err)
	}
	buildState, err := loadRepositoryWorkloadBuildState(files)
	if err != nil {
		return false, fmt.Errorf("load application workload build identity: %w", err)
	}
	changedBuildServices := changedRepositoryWorkloadBuildServices(buildFingerprints, buildState)
	if len(buildFingerprints) > 0 {
		if len(changedBuildServices) == 0 {
			cli.ReportActivityDetail(out, "source unchanged")
		} else {
			cli.ReportActivityDetail(out, "source changes detected: "+strings.Join(changedBuildServices, ", "))
			if err := compose.BuildProjectFilesSelectedProgress(ctx, workload.Project, workload.RepositoryRoot, environment, changedBuildServices, func(detail string) {
				cli.ReportActivityDetail(out, detail)
			}, composeFiles...); err != nil {
				return false, fmt.Errorf("rebuild changed application workload: %w", err)
			}
			var replace []string
			for _, service := range changedBuildServices {
				if _, existed := beforeServices[service]; existed {
					replace = append(replace, service)
				}
			}
			if len(replace) > 0 {
				if err := compose.StopProjectFilesSelected(ctx, workload.Project, workload.RepositoryRoot, environment, replace, composeFiles...); err != nil {
					return false, fmt.Errorf("replace changed application workload services: %w", err)
				}
			}
			cli.ReportActivityDetail(out, "rebuilt "+strings.Join(changedBuildServices, ", "))
		}
	}

	cleanupNewResources := func() {
		timeout := 30 * time.Second
		if ctx.Err() != nil {
			timeout = 2 * time.Second
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
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

	cli.ReportActivityDetail(out, "waiting for workload service and HTTP/TLS readiness")
	fmt.Fprintf(out, "[WAIT] workload          waiting up to %s for service and HTTP/TLS readiness\n", repositoryWorkloadReadinessTimeout)
	initState, err := loadRepositoryInitState(workload.RepositoryRoot)
	if err != nil {
		cleanupNewResources()
		return false, fmt.Errorf("load repository deployment state for readiness: %w", err)
	}
	verifyCtx, cancel := context.WithTimeout(ctx, repositoryWorkloadReadinessTimeout)
	defer cancel()
	var lastStatus repositoryWorkloadStatus
	var lastErr error
	for verifyCtx.Err() == nil {
		states, stateErr := compose.ServiceStatesProjectFilesEnv(verifyCtx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
		if stateErr != nil {
			lastErr = stateErr
		} else {
			exposures := inspectWorkloadExposures(verifyCtx, expectedServices, states, initState.Hostname)
			services := attachWorkloadExposures(buildWorkloadServiceStatuses(expectedServices, states), exposures)
			lastStatus = repositoryWorkloadStatus{
				Found:     true,
				Workload:  workload,
				Services:  services,
				Exposures: exposures,
			}
			lastErr = workloadExposureReadinessError(exposures)
		}
		if lastErr == nil && lastStatus.Ready() {
			cli.ReportActivityDetail(out, "workload ready")
			fmt.Fprintf(out, "[READY] workload         %d Compose service(s) ready\n", len(expectedServices))
			if len(lastStatus.Exposures) > 0 {
				fmt.Fprintf(out, "[READY] exposure         %d/%d published HTTP/TLS endpoint(s) ready\n", lastStatus.ExposureReadyCount(), len(lastStatus.Exposures))
			}
			if len(buildFingerprints) > 0 {
				if err := persistRepositoryWorkloadBuildState(files, buildFingerprints); err != nil {
					return false, fmt.Errorf("record verified workload build identity: %w", err)
				}
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
	if errors.Is(err, os.ErrNotExist) && resolved.FromRepository {
		return stopRepositoryWorkloadRecovery(ctx, compose, resolved, files)
	}
	if err != nil || !found {
		return false, err
	}
	environment, err := repositoryWorkloadStopEnvironment(resolved, files)
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

func stopRepositoryWorkloadRecovery(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	repositoryRoot := resolved.repositoryRoot()
	services, composePath, found, err := application.SelectedWorkloadServices(repositoryRoot, resolved.Manifest)
	if err != nil || !found {
		return false, err
	}
	environment, err := repositoryWorkloadStopEnvironment(resolved, files)
	if err != nil {
		return false, err
	}
	project := application.WorkloadProjectName(resolved.Manifest)
	composeFiles := []string{composePath}
	if err := compose.ConfigProjectFilesEnv(ctx, project, repositoryRoot, environment, composeFiles...); err != nil {
		return false, fmt.Errorf("validate application workload before recovery stop: %w", err)
	}
	activeServices, err := compose.ServicesProjectFilesEnv(ctx, project, repositoryRoot, environment, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("resolve active application workload services before recovery stop: %w", err)
	}
	expectedServices := activeSelectedWorkloadServices(activeServices, services)
	partial := len(services) != len(activeServices) || len(resolved.Manifest.Workload.Services) > 0
	if partial {
		if err := compose.StopProjectFilesSelected(ctx, project, repositoryRoot, environment, expectedServices, composeFiles...); err != nil {
			return false, fmt.Errorf("stop selected application workload services during recovery: %w", err)
		}
	} else if err := compose.DownProjectFilesEnv(ctx, project, repositoryRoot, environment, composeFiles...); err != nil {
		return false, fmt.Errorf("stop application workload during recovery: %w", err)
	}
	running, err := compose.RunningServicesProjectFilesEnv(ctx, project, repositoryRoot, environment, composeFiles...)
	if err != nil {
		return false, fmt.Errorf("verify recovered application workload stop: %w", err)
	}
	for _, service := range expectedServices {
		for _, active := range running {
			if service == active {
				return false, fmt.Errorf("verify recovered application workload stop: selected service %s is still running", service)
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
