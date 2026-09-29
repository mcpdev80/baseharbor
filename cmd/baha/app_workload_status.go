package main

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/endpoint"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type workloadServiceStatus struct {
	Service      string
	State        string
	Health       string
	Readiness    string
	Ready        bool
	Terminal     bool
	ExitCode     int
	RuntimeError string
	Exposures    []workloadExposureStatus
}

type workloadExposureStatus = endpoint.ExposureStatus

type repositoryWorkloadStatus struct {
	Found       bool
	Workload    application.WorkloadFiles
	Services    []workloadServiceStatus
	Exposures   []workloadExposureStatus
	BuildDrift  []string
	ConfigDrift []string
}

func inspectRepositoryWorkloadStatus(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) (repositoryWorkloadStatus, error) {
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil || !found {
		return repositoryWorkloadStatus{Found: found, Workload: workload}, err
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, err
	}
	initState, err := loadRepositoryInitState(workload.RepositoryRoot)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, fmt.Errorf("load repository deployment state: %w", err)
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

	buildFingerprints, err := resolveRepositoryWorkloadBuildFingerprints(ctx, compose, workload, environment, expected, composeFiles)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, fmt.Errorf("resolve workload build identity: %w", err)
	}
	buildState, err := loadRepositoryWorkloadBuildState(files)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, fmt.Errorf("load workload build identity: %w", err)
	}
	buildDrift := changedRepositoryWorkloadBuildServices(buildFingerprints, buildState)

	configFingerprints, err := resolveRepositoryWorkloadConfigFingerprints(ctx, compose, workload, environment, expected, composeFiles)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, fmt.Errorf("resolve workload configuration identity: %w", err)
	}
	configState, err := loadRepositoryWorkloadConfigState(files)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, fmt.Errorf("load workload configuration identity: %w", err)
	}
	configDrift := changedRepositoryWorkloadConfigServices(configFingerprints, configState)

	protocols, err := repositoryWorkloadProtocols(workload)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, err
	}
	states, stateErr := compose.ServiceStatesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if stateErr == nil {
		exposures := inspectWorkloadExposures(ctx, expected, states, initState.Hostname, protocols)
		services := attachWorkloadExposures(buildWorkloadServiceStatuses(expected, states), exposures)
		status := repositoryWorkloadStatus{Found: true, Workload: workload, Services: services, Exposures: exposures, BuildDrift: buildDrift, ConfigDrift: configDrift}
		if len(buildDrift) > 0 {
			return status, fmt.Errorf("workload source changed since last verified build: %s; run 'baha up' to rebuild", strings.Join(buildDrift, ", "))
		}
		if len(configDrift) > 0 {
			return status, fmt.Errorf("workload effective configuration changed since last verified convergence: %s; run 'baha up' to recreate", strings.Join(configDrift, ", "))
		}
		if err := workloadExposureReadinessError(status.Exposures); err != nil {
			return status, err
		}
		return status, nil
	}

	readyServices, err := compose.RunningServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return repositoryWorkloadStatus{Found: true, Workload: workload}, stateErr
	}
	fallback := make([]bhruntime.ServiceState, 0, len(readyServices))
	for _, service := range readyServices {
		fallback = append(fallback, bhruntime.ServiceState{Service: service, State: "running"})
	}
	status := repositoryWorkloadStatus{Found: true, Workload: workload, Services: buildWorkloadServiceStatuses(expected, fallback), BuildDrift: buildDrift, ConfigDrift: configDrift}
	if len(buildDrift) > 0 {
		return status, fmt.Errorf("workload source changed since last verified build: %s; run 'baha up' to rebuild", strings.Join(buildDrift, ", "))
	}
	if len(configDrift) > 0 {
		return status, fmt.Errorf("workload effective configuration changed since last verified convergence: %s; run 'baha up' to recreate", strings.Join(configDrift, ", "))
	}
	return status, nil
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
		normalizedState := normalizedWorkloadState(state.State)
		health := strings.ToLower(strings.TrimSpace(state.Health))
		ready := normalizedState == "running" && health == "healthy"
		readiness := "not-ready"
		if ready {
			readiness = "healthcheck"
		} else if normalizedState == "running" && health == "" {
			readiness = "unverified"
		}
		result = append(result, workloadServiceStatus{
			Service: service, State: normalizedState, Health: health, Readiness: readiness, Ready: ready,
			Terminal: state.TerminalFailure(), ExitCode: state.ExitCode, RuntimeError: strings.TrimSpace(state.Error),
		})
	}
	return result
}

func terminalWorkloadServiceError(services []workloadServiceStatus) error {
	var failures []string
	for _, service := range services {
		if service.Terminal {
			failures = append(failures, service.Service+" "+formatWorkloadServiceStatus(service))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("application workload service terminated before readiness: %s", strings.Join(failures, "; "))
}

func attachWorkloadExposures(services []workloadServiceStatus, exposures []workloadExposureStatus) []workloadServiceStatus {
	byService := make(map[string][]workloadExposureStatus)
	for _, exposure := range exposures {
		byService[exposure.Service] = append(byService[exposure.Service], exposure)
	}
	for i := range services {
		services[i].Exposures = byService[services[i].Service]
		if len(services[i].Exposures) == 0 {
			continue
		}
		services[i].Ready = services[i].State == "running" && services[i].Health != "unhealthy"
		services[i].Readiness = "endpoint"
		for _, exposure := range services[i].Exposures {
			if !exposure.Ready {
				services[i].Ready = false
				services[i].Readiness = "not-ready"
			}
		}
	}
	return services
}

func inspectWorkloadExposures(ctx context.Context, expected []string, states []bhruntime.ServiceState, configuredHostname string, protocols map[string]string) []workloadExposureStatus {
	selected := make(map[string]bool, len(expected))
	for _, name := range expected {
		selected[name] = true
	}
	configuredHostname = strings.TrimSpace(configuredHostname)
	seen := map[string]struct{}{}
	var result []workloadExposureStatus
	for _, state := range states {
		if !selected[state.Service] || normalizedWorkloadState(state.State) != "running" || strings.EqualFold(strings.TrimSpace(state.Health), "unhealthy") {
			continue
		}
		for _, publisher := range state.Publishers {
			if publisher.PublishedPort <= 0 || (publisher.Protocol != "" && publisher.Protocol != "tcp") {
				continue
			}
			scheme, httpProbe := workloadExposureSchemeForService(state.Service, protocols, publisher.TargetPort, publisher.PublishedPort)
			if !httpProbe {
				scheme = "tcp"
			}
			host := normalizePublishedHost(publisher.URL)
			key := fmt.Sprintf("%s\x00%s\x00%s\x00%d", state.Service, scheme, host, publisher.PublishedPort)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			logicalHost := host
			if isLoopbackHost(host) {
				logicalHost = "localhost"
				if configuredHostname != "" {
					logicalHost = configuredHostname
				}
			}
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			var ready bool
			var detail string
			if scheme == "tcp" {
				ready, detail = probeTCPExposureTarget(probeCtx, host, publisher.PublishedPort)
			} else {
				ready, detail = probeHTTPExposureTarget(probeCtx, scheme, host, logicalHost, publisher.PublishedPort)
			}
			cancel()
			if !ready && scheme == "http" && strings.TrimSpace(protocols[state.Service]) == "" {
				probeCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
				tlsReady, tlsDetail := probeHTTPExposureTarget(probeCtx, "https", host, logicalHost, publisher.PublishedPort)
				cancel()
				if tlsReady {
					scheme = "https"
					ready = true
					detail = tlsDetail
				}
			}
			result = append(result, workloadExposureStatus{Service: state.Service, Scheme: scheme, Host: logicalHost, Port: publisher.PublishedPort, Ready: ready, Detail: detail})
		}
	}
	endpoint.SortExposureStatuses(result)
	return result
}

func repositoryWorkloadProtocols(workload application.WorkloadFiles) (map[string]string, error) {
	rel, err := filepath.Rel(workload.RepositoryRoot, workload.Compose)
	if err != nil {
		return nil, fmt.Errorf("resolve workload Compose path for transport discovery: %w", err)
	}
	analysis, err := repositoryinspect.AnalyzeComposeFile(workload.RepositoryRoot, filepath.ToSlash(rel))
	if err != nil {
		return nil, fmt.Errorf("resolve workload transport protocol: %w", err)
	}
	return analysis.WorkloadProtocols, nil
}

func workloadExposureSchemeForService(service string, protocols map[string]string, targetPort, publishedPort int) (string, bool) {
	if protocol := strings.ToLower(strings.TrimSpace(protocols[service])); protocol == "http" || protocol == "https" {
		return protocol, true
	}
	return workloadExposureScheme(targetPort, publishedPort)
}

func workloadExposureReadinessError(exposures []workloadExposureStatus) error {
	var failures []string
	for _, exposure := range exposures {
		if !exposure.Ready {
			failures = append(failures, fmt.Sprintf("%s/%s", exposure.Service, formatWorkloadExposureStatus(exposure)))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("exposure readiness failed: %s", strings.Join(failures, "; "))
}

func workloadExposureScheme(targetPort, publishedPort int) (string, bool) {
	return endpoint.HTTPPortScheme(targetPort, publishedPort)
}

func normalizePublishedHost(host string) string {
	return endpoint.NormalizePublishedHost(host)
}

func isLoopbackHost(host string) bool {
	return endpoint.IsLoopbackHost(host)
}

func probeHTTPExposure(ctx context.Context, scheme, host string, port int) (bool, string) {
	status := endpoint.ProbeHTTP(ctx, endpoint.Endpoint{Service: "workload", Scheme: scheme, Host: host, Port: port})
	return status.Ready, status.Detail
}

func probeHTTPExposureTarget(ctx context.Context, scheme, dialHost, requestHost string, port int) (bool, string) {
	status := endpoint.ProbeHTTPDialTarget(ctx, endpoint.Endpoint{Service: "workload", Scheme: scheme, Host: requestHost, Port: port}, dialHost, port)
	return status.Ready, status.Detail
}

func probeTCPExposureTarget(ctx context.Context, host string, port int) (bool, string) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, "TCP listener accepted connection"
}

func normalizedWorkloadState(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		return "unknown"
	}
	return state
}

func (status repositoryWorkloadStatus) RunningUnverified() bool {
	if !status.Found || len(status.Services) == 0 || len(status.BuildDrift) > 0 || len(status.ConfigDrift) > 0 {
		return false
	}
	unverified := false
	for _, service := range status.Services {
		if service.Ready {
			continue
		}
		if service.State != "running" || service.Health != "" || len(service.Exposures) != 0 || service.Readiness != "unverified" {
			return false
		}
		unverified = true
	}
	return unverified
}

func (status repositoryWorkloadStatus) Ready() bool {
	if !status.Found || len(status.Services) == 0 || len(status.BuildDrift) > 0 || len(status.ConfigDrift) > 0 {
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

func (status repositoryWorkloadStatus) ExposureReadyCount() int {
	count := 0
	for _, exposure := range status.Exposures {
		if exposure.Ready {
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
	if service.Readiness != "" {
		detail += " readiness=" + service.Readiness
	}
	if service.ExitCode != 0 {
		detail += fmt.Sprintf(" exit_code=%d", service.ExitCode)
	}
	if service.RuntimeError != "" {
		detail += " runtime_error=" + service.RuntimeError
	}
	for _, exposure := range service.Exposures {
		detail += " exposure=" + formatWorkloadExposureStatus(exposure)
	}
	return detail
}

func formatWorkloadExposureStatus(exposure workloadExposureStatus) string {
	return fmt.Sprintf("%s://%s:%d %s", exposure.Scheme, exposure.Host, exposure.Port, exposure.Detail)
}
