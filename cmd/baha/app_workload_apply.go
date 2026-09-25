package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type repositoryWorkloadExecution struct {
	compose           bhruntime.Compose
	resolved          resolvedApplication
	files             application.RuntimeFiles
	workload          application.WorkloadFiles
	environment       map[string]string
	composeFiles      []string
	expectedServices  []string
	beforeServices    map[string]struct{}
	freshStart        bool
	buildFingerprints map[string]string
}

func prepareRepositoryWorkloadExecution(ctx context.Context, out io.Writer, compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles) (*repositoryWorkloadExecution, bool, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return nil, found, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return nil, false, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return nil, false, err
	}
	if _, err := analyzeResolvedRepositoryWorkloadSecurity(ctx, compose, resolved, workload, environment, composeFiles); err != nil {
		return nil, false, fmt.Errorf("workload security preflight before start: %w", err)
	}
	if err := compose.ConfigProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...); err != nil {
		return nil, false, fmt.Errorf("validate application workload Compose integration: %w", err)
	}
	activeServices, err := compose.ServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, false, fmt.Errorf("resolve application workload services: %w", err)
	}
	expectedServices := activeSelectedWorkloadServices(activeServices, workload.Services)
	if len(expectedServices) == 0 {
		return nil, false, fmt.Errorf("application workload has no active selected Compose services")
	}

	beforeStates, err := compose.ServiceStatesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, false, fmt.Errorf("inspect application workload before start: %w", err)
	}
	beforeServices := make(map[string]struct{}, len(beforeStates))
	for _, state := range beforeStates {
		beforeServices[state.Service] = struct{}{}
	}
	if len(beforeStates) == 0 {
		if err := preflightRepositoryWorkloadPublishedPorts(ctx, runtimeInput, out, workload, files, environment); err != nil {
			return nil, false, err
		}
	}

	buildFingerprints, err := resolveRepositoryWorkloadBuildFingerprints(ctx, compose, workload, environment, expectedServices, composeFiles)
	if err != nil {
		return nil, false, fmt.Errorf("resolve application workload build identity: %w", err)
	}

	return &repositoryWorkloadExecution{
		compose:           compose,
		resolved:          resolved,
		files:             files,
		workload:          workload,
		environment:       environment,
		composeFiles:      composeFiles,
		expectedServices:  expectedServices,
		beforeServices:    beforeServices,
		freshStart:        len(beforeStates) == 0,
		buildFingerprints: buildFingerprints,
	}, true, nil
}

func (e *repositoryWorkloadExecution) rebuildChangedServices(ctx context.Context, out io.Writer) error {
	buildState, err := loadRepositoryWorkloadBuildState(e.files)
	if err != nil {
		return fmt.Errorf("load application workload build identity: %w", err)
	}
	changed := changedRepositoryWorkloadBuildServices(e.buildFingerprints, buildState)
	if len(e.buildFingerprints) == 0 {
		return nil
	}
	if len(changed) == 0 {
		cli.ReportActivityDetail(out, "source unchanged")
		return nil
	}

	cli.ReportActivityDetail(out, "source changes detected: "+strings.Join(changed, ", "))
	if err := e.compose.BuildProjectFilesSelectedProgress(ctx, e.workload.Project, e.workload.RepositoryRoot, e.environment, changed, func(detail string) {
		cli.ReportActivityDetail(out, detail)
	}, e.composeFiles...); err != nil {
		return fmt.Errorf("rebuild changed application workload: %w", err)
	}

	var replace []string
	for _, service := range changed {
		if _, existed := e.beforeServices[service]; existed {
			replace = append(replace, service)
		}
	}
	if len(replace) > 0 {
		if err := e.compose.StopProjectFilesSelected(ctx, e.workload.Project, e.workload.RepositoryRoot, e.environment, replace, e.composeFiles...); err != nil {
			return fmt.Errorf("replace changed application workload services: %w", err)
		}
	}
	cli.ReportActivityDetail(out, "rebuilt "+strings.Join(changed, ", "))
	return nil
}

func (e *repositoryWorkloadExecution) start(ctx context.Context, out io.Writer) error {
	var startServices []string
	if e.workload.Partial || len(e.resolved.Manifest.Workload.Services) > 0 {
		startServices = e.expectedServices
	}
	if err := startRepositoryWorkloadWithPortFallback(ctx, runtimeInput, out, e.compose, e.workload, e.files, e.environment, startServices, e.composeFiles); err != nil {
		return fmt.Errorf("start application workload: %w", err)
	}
	return nil
}

func (e *repositoryWorkloadExecution) cleanup(ctx context.Context) {
	timeout := 30 * time.Second
	if ctx.Err() != nil {
		timeout = 2 * time.Second
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	if e.freshStart {
		_ = e.compose.DownProjectFilesEnv(cleanupCtx, e.workload.Project, e.workload.RepositoryRoot, e.environment, e.composeFiles...)
		return
	}
	var newlyCreated []string
	for _, service := range e.expectedServices {
		if _, existed := e.beforeServices[service]; !existed {
			newlyCreated = append(newlyCreated, service)
		}
	}
	if len(newlyCreated) > 0 {
		_ = e.compose.StopProjectFilesSelected(cleanupCtx, e.workload.Project, e.workload.RepositoryRoot, e.environment, newlyCreated, e.composeFiles...)
	}
}

func (e *repositoryWorkloadExecution) waitReady(ctx context.Context, out io.Writer) error {
	cli.ReportActivityDetail(out, "waiting for workload service and HTTP/TLS readiness")
	fmt.Fprintf(out, "[WAIT] workload          waiting up to %s for service and HTTP/TLS readiness\n", repositoryWorkloadReadinessTimeout)

	initState, err := loadRepositoryInitState(e.workload.RepositoryRoot)
	if err != nil {
		return fmt.Errorf("load repository deployment state for readiness: %w", err)
	}

	verifyCtx, cancel := context.WithTimeout(ctx, repositoryWorkloadReadinessTimeout)
	defer cancel()
	var lastStatus repositoryWorkloadStatus
	var lastErr error
	for verifyCtx.Err() == nil {
		states, stateErr := e.compose.ServiceStatesProjectFilesEnv(verifyCtx, e.workload.Project, e.workload.RepositoryRoot, e.environment, e.composeFiles...)
		if stateErr != nil {
			lastErr = stateErr
		} else {
			exposures := inspectWorkloadExposures(verifyCtx, e.expectedServices, states, initState.Hostname)
			services := attachWorkloadExposures(buildWorkloadServiceStatuses(e.expectedServices, states), exposures)
			lastStatus = repositoryWorkloadStatus{Found: true, Workload: e.workload, Services: services, Exposures: exposures}
			lastErr = workloadExposureReadinessError(exposures)
		}
		if lastErr == nil && lastStatus.Ready() {
			return e.recordReady(out, lastStatus)
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(repositoryWorkloadReadinessPollInterval):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("verify application workload readiness after %s: %w", repositoryWorkloadReadinessTimeout, lastErr)
	}
	return fmt.Errorf("application workload did not reach readiness within %s; services=%d/%d exposures=%d/%d", repositoryWorkloadReadinessTimeout, lastStatus.ReadyCount(), len(e.expectedServices), lastStatus.ExposureReadyCount(), len(lastStatus.Exposures))
}

func (e *repositoryWorkloadExecution) recordReady(out io.Writer, status repositoryWorkloadStatus) error {
	cli.ReportActivityDetail(out, "workload ready")
	fmt.Fprintf(out, "[READY] workload         %d Compose service(s) ready\n", len(e.expectedServices))
	if len(status.Exposures) > 0 {
		fmt.Fprintf(out, "[READY] exposure         %d/%d published HTTP/TLS endpoint(s) ready\n", status.ExposureReadyCount(), len(status.Exposures))
	}
	if len(e.buildFingerprints) > 0 {
		if err := persistRepositoryWorkloadBuildState(e.files, e.buildFingerprints); err != nil {
			return fmt.Errorf("record verified workload build identity: %w", err)
		}
	}
	fmt.Fprintf(out, "Workload Compose: %s\n", e.workload.Compose)
	return nil
}
