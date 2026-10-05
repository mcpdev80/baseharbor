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
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
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
	composeSource, err := selectedRepositoryComposeSource(repositoryRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	if composeSource == "" {
		return nil
	}
	composePath, err := application.ResolveWorkloadComposeSource(repositoryRoot, composeSource)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(repositoryRoot, composePath)
	if err != nil {
		return err
	}
	analysis, err := repositoryinspect.AnalyzeComposeFile(repositoryRoot, filepath.ToSlash(rel))
	if err != nil {
		return fmt.Errorf("inspect repository workload adoption requirements: %w", err)
	}
	if len(application.SQLInstanceNames(resolved.Manifest)) > 0 && len(analysis.DatabaseBootstrapServices) > 0 {
		return fmt.Errorf(
			"repository database bootstrap for Compose service(s) %s would be discarded by managed SQL replacement; keep the database service in the workload or migrate the bootstrap explicitly before apply",
			strings.Join(analysis.DatabaseBootstrapServices, ", "),
		)
	}
	selected := map[string]struct{}{}
	for _, service := range application.WorkloadComponentNames(resolved.Manifest) {
		selected[service] = struct{}{}
	}
	for service, protocol := range analysis.WorkloadProtocols {
		if len(selected) > 0 {
			if _, ok := selected[service]; !ok {
				continue
			}
		}
		if !strings.EqualFold(strings.TrimSpace(protocol), "https") {
			continue
		}
		hasExposure := false
		for _, exposure := range resolved.Manifest.Exposures {
			if exposure.Service == service {
				hasExposure = true
				break
			}
		}
		if !hasExposure {
			return usageError(
				fmt.Sprintf("repository workload service %s requires HTTPS but the application contract does not declare exposure.http for that service", service),
				"Add an exposure.http entry to baseharbor.yaml for the named workload service, its container target port and protocol https; inspect the repository and review that contract before retrying.",
			)
		}
	}
	return nil
}

func selectedRepositoryComposeSource(repositoryRoot string, manifest application.Manifest) (string, error) {
	if !application.HasExplicitWorkload(manifest) {
		return "", nil
	}
	result, err := repositoryinspect.Inspect(context.Background(), repositoryRoot)
	if err != nil {
		return "", &machine.Error{
			Code:      machine.ErrorInvalidWorkload,
			CauseCode: "workload_source_inspection_failed",
			Message:   "BaseHarbor could not resolve the repository workload source before runtime realization.",
			Resource:  repositoryRoot,
			Next:      "Run 'baha app inspect . --verbose' and resolve the reported source ambiguity or invalid source metadata.",
			Cause:     err,
		}
	}
	if result.SelectedWorkloadSource == nil {
		return "", &machine.Error{
			Code:      machine.ErrorUnsupported,
			CauseCode: "workload_source_selection_required",
			Message:   "The portable workload has no unambiguous repository source selected for runtime realization.",
			Resource:  repositoryRoot,
			Next:      "Select the authoritative workload source with 'baha app init' or commit baseharbor.repository.yaml when multiple sources are intentional.",
		}
	}
	if result.SelectedWorkloadSource.Kind != repositoryinspect.WorkloadSourceCompose {
		return "", &machine.Error{
			Code:      machine.ErrorUnsupported,
			CauseCode: "workload_source_runtime_unsupported",
			Message:   fmt.Sprintf("BaseHarbor understands the %s workload source but the current v0.4.20 repository runtime path realizes Compose sources only.", result.SelectedWorkloadSource.Kind),
			Resource:  result.SelectedWorkloadSource.Path,
			Next:      "Use inspection/adoption now or choose a Runtime Provider that realizes this workload source when available.",
		}
	}
	return result.SelectedWorkloadSource.Path, nil
}

func preflightRepositoryWorkloadSourceRealization(repositoryRoot string, manifest application.Manifest) error {
	if !application.HasExplicitWorkload(manifest) {
		return nil
	}
	_, err := selectedRepositoryComposeSource(repositoryRoot, manifest)
	return err
}

func preflightRepositoryWorkloadSecurity(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication) (application.WorkloadSecurityReport, error) {
	if !resolved.FromRepository {
		return application.WorkloadSecurityReport{}, nil
	}
	repositoryRoot := resolved.repositoryRoot()
	composeSource, err := selectedRepositoryComposeSource(repositoryRoot, resolved.Manifest)
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	if composeSource == "" {
		return application.WorkloadSecurityReport{}, nil
	}
	selected, composePath, err := application.SelectedWorkloadServicesFromCompose(repositoryRoot, composeSource, resolved.Manifest)
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	if compose == nil {
		return application.WorkloadSecurityReport{}, machine.NewError(machine.ErrorRuntimeUnavailable, "Runtime provider unavailable for workload security preflight.", "Resolve runtime orchestration before retrying.", false)
	}
	environment := workloadSecurityPreflightEnvironment(resolved.Manifest)
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, application.WorkloadProjectName(resolved.Manifest), repositoryRoot, environment, composePath)
	if err != nil {
		return application.WorkloadSecurityReport{}, fmt.Errorf("render repository Compose for security preflight: %w", err)
	}
	if _, err := analyzeManagedServiceReferenceRewrites(resolved.Manifest, []byte(rendered), selected); err != nil {
		return application.WorkloadSecurityReport{}, fmt.Errorf("managed service replacement would leave an unresolved repository reference: %w", err)
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

func preflightRepositoryWorkloadSecuritySource(resolved resolvedApplication) (application.WorkloadSecurityReport, error) {
	if !resolved.FromRepository {
		return application.WorkloadSecurityReport{}, nil
	}
	repositoryRoot := resolved.repositoryRoot()
	composeSource, err := selectedRepositoryComposeSource(repositoryRoot, resolved.Manifest)
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	if composeSource == "" {
		return application.WorkloadSecurityReport{}, nil
	}
	selected, composePath, err := application.SelectedWorkloadServicesFromCompose(repositoryRoot, composeSource, resolved.Manifest)
	if err != nil {
		return application.WorkloadSecurityReport{}, err
	}
	data, err := os.ReadFile(composePath)
	if err != nil {
		return application.WorkloadSecurityReport{}, fmt.Errorf("read repository Compose for security preflight: %w", err)
	}
	report, err := application.AnalyzeComposeSecuritySource(resolved.Manifest, data)
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

func preflightResolvedRepositoryWorkloadSecurity(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadSecurityReport, error) {
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
	compose bhruntime.RuntimeProvider,
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
	composeSource, err := selectedRepositoryComposeSource(repositoryRoot, resolved.Manifest)
	if err != nil {
		return application.WorkloadFiles{}, false, err
	}
	if composeSource == "" {
		return application.WorkloadFiles{}, false, nil
	}
	return application.MaterializeWorkloadFromCompose(repositoryRoot, composeSource, resolved.Manifest, files)
}

type renderedComposeConfig struct {
	Services map[string]struct {
		Image       string         `json:"image"`
		Environment map[string]any `json:"environment"`
	} `json:"services"`
}

func repositoryWorkloadBindingPlan(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, workload application.WorkloadFiles, environment map[string]string) (application.WorkloadBindingPlan, error) {
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

func repositoryWorkloadComposeFiles(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, workload application.WorkloadFiles, files application.RuntimeFiles, environment map[string]string) ([]string, error) {
	composeFiles := []string{workload.Compose, workload.Override}
	rewriteOverride, enabled, err := materializeManagedServiceReferenceRewrite(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return nil, err
	}
	if enabled {
		composeFiles = append(composeFiles, rewriteOverride)
	}
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
	fixedPortOverride, enabled, err := existingFixedWorkloadPortOverride(files)
	if err != nil {
		return nil, err
	}
	if enabled {
		composeFiles = append(composeFiles, fixedPortOverride)
	}
	return composeFiles, nil
}

func repositoryWorkloadStopEnvironment(resolved resolvedApplication, files application.RuntimeFiles) (map[string]string, error) {
	environment := workloadSecurityPreflightEnvironment(resolved.Manifest)
	if environment == nil {
		environment = map[string]string{}
	}
	if err := mergeResolvedRepositoryWorkloadPorts(environment, resolved, files); err != nil {
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
	if err := mergeResolvedRepositoryWorkloadPorts(environment, resolved, files); err != nil {
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
	compose, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityServiceExec)
	if err != nil {
		return nil, err
	}
	platformFiles, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return nil, err
	}
	service := applicationsecret.NewForApplicationRuntime(resolved.Store, compose, platformFiles, resolved.Manifest, files)
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

func applyRepositoryWorkload(ctx context.Context, out io.Writer, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	execution, found, err := prepareRepositoryWorkloadExecution(ctx, out, compose, resolved, files)
	if err != nil || !found {
		return false, err
	}
	if err := execution.rebuildChangedServices(ctx, out); err != nil {
		return false, err
	}
	if err := execution.recreateConfigurationChangedServices(ctx, out); err != nil {
		return false, err
	}
	if err := execution.start(ctx, out); err != nil {
		if invalidateErr := execution.invalidateFailedBuildCandidate(); invalidateErr != nil {
			err = fmt.Errorf("%w; invalidate failed workload candidate: %v", err, invalidateErr)
		}
		execution.cleanup(ctx)
		return false, err
	}
	if err := execution.waitReady(ctx, out); err != nil {
		if invalidateErr := execution.invalidateFailedBuildCandidate(); invalidateErr != nil {
			err = fmt.Errorf("%w; invalidate failed workload candidate: %v", err, invalidateErr)
		}
		execution.cleanup(ctx)
		return false, err
	}
	return true, nil
}

func stopRepositoryWorkload(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	if !resolved.FromRepository {
		return false, nil
	}
	project := application.WorkloadProjectNameForRuntime(resolved.Manifest, files)
	containers, err := compose.ListRuntimeContainers(ctx)
	if err != nil {
		return false, fmt.Errorf("inspect application workload ownership before stop: %w", err)
	}
	found := false
	for _, container := range containers {
		if container.Project == project {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	if err := compose.StopOwnedProjectContainers(ctx, project); err != nil {
		return false, fmt.Errorf("stop application workload by observed runtime ownership: %w", err)
	}
	return true, nil
}

func destroyRepositoryWorkloadRuntime(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	if !resolved.FromRepository {
		return false, nil
	}
	project := application.WorkloadProjectNameForRuntime(resolved.Manifest, files)
	resources, err := compose.ListOwnedProjectResources(ctx, project)
	if err != nil {
		return false, fmt.Errorf("inventory application workload runtime resources: %w", err)
	}
	cleanup := workloadRuntimeCleanupResources(resources)
	if len(cleanup) == 0 {
		return false, nil
	}
	if err := compose.DestroyOwnedProjectResources(ctx, project, cleanup); err != nil {
		return false, fmt.Errorf("remove application workload containers/networks: %w", err)
	}
	remaining, err := compose.InspectProjectResources(ctx, project, cleanup)
	if err != nil {
		return false, fmt.Errorf("verify application workload runtime cleanup: %w", err)
	}
	if err := verifyRepositoryWorkloadCleanup(remaining); err != nil {
		return false, err
	}
	return true, nil
}

func verifyRepositoryWorkloadCleanup(remaining []bhruntime.ProjectResource) error {
	for _, resource := range remaining {
		if resource.Kind != "network" {
			return fmt.Errorf("verify application workload runtime cleanup: owned %s %s remains", resource.Kind, resource.Name)
		}
	}
	return nil
}

func workloadRuntimeCleanupResources(resources []bhruntime.ProjectResource) []bhruntime.ProjectResource {
	cleanup := make([]bhruntime.ProjectResource, 0, len(resources))
	for _, resource := range resources {
		switch resource.Kind {
		case "container", "network":
			cleanup = append(cleanup, resource)
		}
	}
	return cleanup
}

func inspectRepositoryWorkload(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (application.WorkloadFiles, []string, bool, error) {
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

func checkRepositoryWorkloadReady(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (int, bool, error) {
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
