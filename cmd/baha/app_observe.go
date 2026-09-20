package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type applicationStatusCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type applicationStatusResult struct {
	Application string                   `json:"application"`
	Environment string                   `json:"environment"`
	Manifest    string                   `json:"manifest,omitempty"`
	Project     string                   `json:"project"`
	State       string                   `json:"state"`
	Ready       bool                     `json:"ready"`
	Checks      []applicationStatusCheck `json:"checks"`
}

func (r *applicationStatusResult) add(name string, ok bool, detail string) {
	r.Checks = append(r.Checks, applicationStatusCheck{Name: name, OK: ok, Detail: detail})
	if !ok {
		r.Ready = false
	}
}

func collectApplicationStatus(ctx context.Context, store application.Store, args []string) (applicationStatusResult, error) {
	resolved, err := resolveApplication(store, args, "status")
	if err != nil {
		return applicationStatusResult{}, err
	}
	m := resolved.Manifest
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return applicationStatusResult{}, err
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		return applicationStatusResult{}, err
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return applicationStatusResult{}, err
	}
	project := application.RuntimeProjectName(m)
	services, err := compose.RunningServicesProject(ctx, project, files.Compose, files.Env)
	if err != nil {
		return applicationStatusResult{}, err
	}
	result := applicationStatusResult{
		Application: m.Name,
		Environment: m.Environment,
		Project:     project,
		State:       "running",
		Ready:       true,
		Checks:      []applicationStatusCheck{},
	}
	if resolved.FromRepository {
		result.Manifest = resolved.ManifestPath
	}

	workloadRunning := []string(nil)
	workloadFound := false
	if _, running, found, inspectErr := inspectRepositoryWorkload(ctx, compose, resolved, files); inspectErr == nil {
		workloadRunning = running
		workloadFound = found
	}
	brokerRunning := false
	if application.RequiresRuntimeBroker(m) {
		if brokerFiles, brokerErr := runtimebroker.Existing(files); brokerErr == nil {
			if running, runErr := compose.RunningServicesProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env); runErr == nil {
				brokerRunning = len(running) > 0
			}
		}
	}
	exposureRunning := managedExposureRunning(ctx, compose, m, files)
	if applicationComponentsStopped(services, workloadRunning, workloadFound, brokerRunning, exposureRunning) && !application.HasObjectStorage(m) {
		result.State = "stopped"
		result.Ready = false
		return result, nil
	}

	if application.HasObjectStorage(m) {
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := objectstorage.VerifyApplicationBuckets(checkCtx, compose, m, files)
		cancel()
		if err != nil {
			result.add("object-storage", false, err.Error())
		} else {
			result.add("object-storage", true, fmt.Sprintf("%d bucket(s) passed authenticated S3 Put/Get", len(application.ObjectStorageBucketNames(m))))
		}
	}
	if application.HasOTLPTelemetry(m) {
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := telemetry.VerifyApplication(checkCtx, m, files)
		cancel()
		if err != nil {
			result.add("telemetry/otlp", false, err.Error())
		} else {
			result.add("telemetry/otlp", true, "real OTLP HTTP/protobuf export accepted")
		}
	}
	if m.Services.Postgres {
		if !containsString(services, "postgres") {
			result.add("postgres", false, "not running")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := application.VerifyPostgresRuntime(checkCtx, compose, m, files)
			cancel()
			if err != nil {
				result.add("postgres", false, "one or more instances failed readiness")
			} else {
				result.add("postgres", true, fmt.Sprintf("%d instance(s) running and authenticated SELECT 1 succeeded", len(application.PostgresInstanceNames(m))))
			}
		}
	}
	if m.Services.Redis {
		if !containsString(services, "valkey") {
			result.add("valkey", false, "not running")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := application.VerifyValkeyRuntime(checkCtx, compose, m, files)
			cancel()
			if err != nil {
				result.add("valkey", false, "one or more instances failed authenticated PING")
			} else {
				result.add("valkey", true, fmt.Sprintf("%d instance(s) running and authenticated PING returned PONG", len(application.RedisInstanceNames(m))))
			}
		}
	}
	if m.Services.Secrets {
		platformFiles, platformErr := bhruntime.ExistingFiles("")
		if platformErr != nil {
			result.add("secrets", false, "BaseHarbor OpenBao runtime is not materialized")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
			err := openbao.InspectApplicationScope(checkCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
			if err != nil {
				result.add("secrets", false, "isolated OpenBao application scope is not ready")
			} else {
				result.add("secrets", true, "isolated OpenBao AppRole authentication succeeded")
				if len(application.RequiredSecretNames(m)) > 0 {
					statuses, statusErr := inspectRequiredApplicationSecrets(checkCtx, compose, platformFiles, m, files)
					if statusErr != nil {
						result.add("required-secrets", false, "readiness inspection failed")
					} else {
						for _, status := range statuses {
							result.add("required-secret/"+status.Name, status.Present && status.Usable, map[bool]string{true: "present and usable", false: "missing or unusable"}[status.Present && status.Usable])
						}
					}
				}
			}
			cancel()
		}
		brokerErr := waitRuntimeBrokerReady(ctx, compose, m, files, 10*time.Second)
		if brokerErr != nil {
			result.add("runtime-broker", false, brokerErr.Error())
		} else {
			result.add("runtime-broker", true, "mTLS identity and app-scoped OpenBao readiness succeeded")
		}
	}

	workloadStatus, workloadErr := inspectRepositoryWorkloadStatus(ctx, compose, resolved, files)
	if workloadStatus.Found {
		for _, service := range workloadStatus.Services {
			result.add("workload/"+service.Service, service.Ready, formatWorkloadServiceStatus(service))
		}
		if workloadErr != nil {
			result.add("workload", false, workloadErr.Error())
		} else {
			result.add("workload", workloadStatus.Ready(), fmt.Sprintf("%d/%d selected Compose service(s) ready", workloadStatus.ReadyCount(), len(workloadStatus.Services)))
		}
	} else if workloadErr != nil {
		result.add("workload", false, "repository Compose integration could not be resolved: "+workloadErr.Error())
	}

	if policy, policyErr := application.LogsPolicy(m); policyErr != nil {
		result.add("logs", false, policyErr.Error())
	} else if policy.Enabled && policy.Collect[application.LogsSourceApplication] && workloadStatus.Found {
		logServices := make([]string, 0, len(workloadStatus.Services))
		for _, service := range workloadStatus.Services {
			logServices = append(logServices, service.Service)
		}
		checkCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
		err := logsprovider.VerifyApplication(checkCtx, m, logServices)
		cancel()
		if err != nil {
			result.add("logs", false, err.Error())
		} else {
			result.add("logs", true, fmt.Sprintf("%d workload log stream(s) queryable", len(logServices)))
		}
	}
	if len(m.Exposures) > 0 {
		_, exposureErr := inspectManagedExposure(ctx, compose, m, files)
		if exposureErr != nil {
			result.add("managed-exposure", false, exposureErr.Error())
		} else {
			result.add("managed-exposure", true, "configured exposure endpoints are ready")
		}
	}
	return result, nil
}

func renderApplicationStatus(out io.Writer, result applicationStatusResult) {
	fmt.Fprintf(out, "Application %s (%s)\n", result.Application, result.Environment)
	if result.Manifest != "" {
		fmt.Fprintf(out, "Manifest: %s\n", result.Manifest)
	}
	fmt.Fprintf(out, "Project: %s\n", result.Project)
	if result.State == "stopped" {
		fmt.Fprintln(out, "State: STOPPED (persistent application state is preserved)")
		return
	}
	for _, check := range result.Checks {
		status := "OK"
		if !check.OK {
			status = "FAIL"
		}
		if check.Detail == "" {
			fmt.Fprintf(out, "[%s] %s\n", status, check.Name)
		} else {
			fmt.Fprintf(out, "[%s] %-20s %s\n", status, check.Name, check.Detail)
		}
	}
}

func appStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show application runtime and readiness status",
		Usage:   "baha app status [NAME] [-o json|--output json]",
		Long:    "Reports materialized backend state, repository workload state and protocol-level readiness. Human and structured output are rendered from the same secret-safe readiness result.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app status")
			if err != nil {
				return err
			}
			result, err := collectApplicationStatus(ctx, store, filtered)
			if err != nil {
				return err
			}
			if format == outputJSON {
				if err := writeJSON(out, result); err != nil {
					return err
				}
			} else {
				renderApplicationStatus(out, result)
			}
			if result.State == "stopped" {
				return nil
			}
			if !result.Ready {
				return errors.New("application is not ready")
			}
			return nil
		},
	}
}

func appDoctorCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose an application's runtime",
		Usage:   "baha app doctor [NAME] [-o json|--output json]",
		Long:    "Checks desired state, local runtime files, Compose configuration, backend service state, repository workload state, authenticated protocol readiness, managed OpenBao secret scope health, per-application mTLS broker readiness and required-secret presence/usability. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app doctor")
			if err != nil {
				return err
			}
			resolved, err := resolveApplication(store, filtered, "doctor")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
			var compose bhruntime.Compose
			var running []string
			var platformFiles bhruntime.Files
			var requiredStatuses []openbao.RequiredSecretStatus
			var workloadStatus repositoryWorkloadStatus
			var workloadStatusErr error
			var workloadSecurity application.WorkloadSecurityReport
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "workload discovery", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }},
				{Name: "runtime state", Run: func(context.Context) error { return runtimeErr }},
				{Name: "runtime permissions", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckRuntimePermissions(files)
				}},
				{Name: "managed runtime definition", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckManagedRuntimeDefinition(files, m)
				}},
				{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}},
				{Name: "workload security", Run: func(ctx context.Context) error {
					var err error
					workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, compose, resolved)
					return err
				}},
				{Name: "compose configuration", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
				}},
				{Name: "running services", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					var err error
					running, err = compose.RunningServicesProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
					return err
				}},
				{Name: "repository workload", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					workloadStatus, workloadStatusErr = inspectRepositoryWorkloadStatus(ctx, compose, resolved, files)
					if workloadStatusErr != nil {
						return workloadStatusErr
					}
					if workloadStatus.Found && !workloadStatus.Ready() {
						return fmt.Errorf("%d/%d selected workload services ready", workloadStatus.ReadyCount(), len(workloadStatus.Services))
					}
					return nil
				}},
			}
			if policy, policyErr := application.LogsPolicy(m); policyErr != nil {
				checks = append(checks, preflight.Check{Name: "logs deployment policy", Run: func(context.Context) error { return policyErr }})
			} else if policy.Enabled && policy.Collect[application.LogsSourceApplication] {
				checks = append(checks, preflight.Check{Name: "Loki log ingestion", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					status, err := inspectRepositoryWorkloadStatus(ctx, compose, resolved, files)
					if err != nil {
						return err
					}
					if !status.Found {
						return nil
					}
					services := make([]string, 0, len(status.Services))
					for _, service := range status.Services {
						services = append(services, service.Service)
					}
					return logsprovider.VerifyApplication(ctx, m, services)
				}})
			}
			if len(m.Exposures) > 0 {
				checks = append(checks, preflight.Check{Name: "managed HTTP exposure", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					_, err := inspectManagedExposure(ctx, compose, m, files)
					return err
				}})
			}
			if application.HasObjectStorage(m) {
				checks = append(checks, preflight.Check{Name: "object-storage S3 readiness", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return objectstorage.VerifyApplicationBuckets(ctx, compose, m, files)
				}})
			}
			if application.HasOTLPTelemetry(m) {
				checks = append(checks, preflight.Check{Name: "OTLP telemetry export", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return telemetry.VerifyApplication(ctx, m, files)
				}})
			}
			if m.Services.Postgres {
				checks = append(checks,
					preflight.Check{Name: "postgres running", Run: func(context.Context) error {
						if !containsString(running, "postgres") {
							return errors.New("no postgres instance is running")
						}
						return nil
					}},
					preflight.Check{Name: "postgres readiness", Run: func(ctx context.Context) error {
						if !containsString(running, "postgres") {
							return errors.New("no postgres instance is running")
						}
						return application.VerifyPostgresRuntime(ctx, compose, m, files)
					}},
				)
			}
			if m.Services.Redis {
				checks = append(checks,
					preflight.Check{Name: "valkey running", Run: func(context.Context) error {
						if !containsString(running, "valkey") {
							return errors.New("no valkey instance is running")
						}
						return nil
					}},
					preflight.Check{Name: "valkey readiness", Run: func(ctx context.Context) error {
						if !containsString(running, "valkey") {
							return errors.New("no valkey instance is running")
						}
						return application.VerifyValkeyRuntime(ctx, compose, m, files)
					}},
				)
			}
			if m.Services.Secrets {
				checks = append(checks,
					preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						return err
					}},
					preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor OpenBao runtime is not materialized")
						}
						identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
						return openbao.InspectApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					}},
					preflight.Check{Name: "application runtime broker", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						return verifyRuntimeBrokerRunning(ctx, compose, m, files)
					}},
				)
				if len(application.RequiredSecretNames(m)) > 0 {
					checks = append(checks, preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						var err error
						requiredStatuses, err = inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
						if err != nil {
							return err
						}
						return openbao.RequireApplicationSecrets(requiredStatuses)
					}})
				}
			}
			results, ok := preflight.Run(checkCtx, checks)
			if format == outputJSON {
				type workloadResult struct {
					Service string `json:"service"`
					Ready   bool   `json:"ready"`
					Detail  string `json:"detail,omitempty"`
				}
				workloads := make([]workloadResult, 0, len(workloadStatus.Services))
				for _, service := range workloadStatus.Services {
					workloads = append(workloads, workloadResult{Service: service.Service, Ready: service.Ready, Detail: formatWorkloadServiceStatus(service)})
				}
				secretStatus := make([]map[string]any, 0, len(requiredStatuses))
				for _, status := range requiredStatuses {
					secretStatus = append(secretStatus, map[string]any{
						"name": status.Name, "present": status.Present, "usable": status.Usable, "generated": status.Generated,
					})
				}
				payload := struct {
					Application     string             `json:"application"`
					Environment     string             `json:"environment"`
					Healthy         bool               `json:"healthy"`
					Checks          []preflight.Result `json:"checks"`
					Workload        []workloadResult   `json:"workload,omitempty"`
					RequiredSecrets []map[string]any   `json:"required_secrets,omitempty"`
				}{
					Application: m.Name, Environment: m.Environment, Healthy: ok,
					Checks: results, Workload: workloads, RequiredSecrets: secretStatus,
				}
				if err := writeJSON(out, payload); err != nil {
					return err
				}
			} else {
				preflight.Format(out, results)
				printWorkloadSecurityFindings(out, workloadSecurity)
				printRequiredSecretStatus(out, requiredStatuses)
				if workloadStatus.Found {
					fmt.Fprintln(out, "WORKLOAD SERVICE      STATE")
					for _, service := range workloadStatus.Services {
						marker := "OK"
						if !service.Ready {
							marker = "FAIL"
						}
						fmt.Fprintf(out, "[%s] %-20s %s\n", marker, service.Service, formatWorkloadServiceStatus(service))
					}
				}
				if workloadStatusErr != nil {
					fmt.Fprintf(out, "Workload diagnosis: %v\n", workloadStatusErr)
				}
			}
			if !ok {
				return errors.New("application doctor found one or more failures")
			}
			if format != outputJSON {
				fmt.Fprintln(out, "Application runtime is healthy.")
			}
			return nil
		},
	}
}

func ownerOnly(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == wanted || strings.HasPrefix(value, wanted+"-") {
			return true
		}
	}
	return false
}

func applicationComponentsStopped(managedRunning, workloadRunning []string, workloadFound, brokerRunning, exposureRunning bool) bool {
	if len(managedRunning) != 0 || brokerRunning || exposureRunning {
		return false
	}
	if workloadFound && len(workloadRunning) != 0 {
		return false
	}
	return true
}
