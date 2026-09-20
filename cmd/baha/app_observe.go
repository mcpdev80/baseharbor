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

func collectApplicationStatus(ctx context.Context, store application.Store, args []string) (application.StatusResult, error) {
	statusCtx, cancelStatus := context.WithTimeout(ctx, 4*time.Second)
	defer cancelStatus()
	ctx = statusCtx

	resolved, err := resolveApplication(store, args, "status")
	if err != nil {
		return application.StatusResult{}, err
	}
	m := resolved.Manifest
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return application.StatusResult{}, err
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		return application.StatusResult{}, err
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return application.StatusResult{}, err
	}
	project := application.RuntimeProjectName(m)
	services, err := compose.RunningServicesProject(ctx, project, files.Compose, files.Env)
	if err != nil {
		return application.StatusResult{}, err
	}
	result := application.StatusResult{
		Application: m.Name,
		Environment: m.Environment,
		Project:     project,
		State:       "running",
		Ready:       true,
		Checks:      []application.StatusCheck{},
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
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := objectstorage.VerifyApplicationBuckets(checkCtx, compose, m, files)
		cancel()
		if err != nil {
			result.AddCheck("object-storage", false, err.Error())
		} else {
			result.AddCheck("object-storage", true, fmt.Sprintf("%d bucket(s) passed authenticated S3 Put/Get", len(application.ObjectStorageBucketNames(m))))
		}
	}
	if application.HasOTLPTelemetry(m) {
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := telemetry.VerifyApplication(checkCtx, m, files)
		cancel()
		if err != nil {
			result.AddCheck("telemetry/otlp", false, err.Error())
		} else {
			result.AddCheck("telemetry/otlp", true, "real OTLP HTTP/protobuf export accepted")
		}
	}
	if m.Services.Postgres {
		if !containsString(services, "postgres") {
			result.AddCheck("postgres", false, "not running")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := application.VerifyPostgresRuntime(checkCtx, compose, m, files)
			cancel()
			if err != nil {
				result.AddCheck("postgres", false, "one or more instances failed readiness")
			} else {
				result.AddCheck("postgres", true, fmt.Sprintf("%d instance(s) running and authenticated SELECT 1 succeeded", len(application.PostgresInstanceNames(m))))
			}
		}
	}
	if m.Services.Redis {
		if !containsString(services, "valkey") {
			result.AddCheck("valkey", false, "not running")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := application.VerifyValkeyRuntime(checkCtx, compose, m, files)
			cancel()
			if err != nil {
				result.AddCheck("valkey", false, "one or more instances failed authenticated PING")
			} else {
				result.AddCheck("valkey", true, fmt.Sprintf("%d instance(s) running and authenticated PING returned PONG", len(application.RedisInstanceNames(m))))
			}
		}
	}
	if m.Services.Secrets {
		platformFiles, platformErr := bhruntime.ExistingFiles("")
		if platformErr != nil {
			result.AddCheck("secrets", false, "BaseHarbor OpenBao runtime is not materialized")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
			err := openbao.InspectApplicationScope(checkCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
			if err != nil {
				result.AddCheck("secrets", false, "isolated OpenBao application scope is not ready")
			} else {
				result.AddCheck("secrets", true, "isolated OpenBao AppRole authentication succeeded")
				if len(application.RequiredSecretNames(m)) > 0 {
					statuses, statusErr := inspectRequiredApplicationSecrets(checkCtx, compose, platformFiles, m, files)
					if statusErr != nil {
						result.AddCheck("required-secrets", false, "readiness inspection failed")
					} else {
						for _, status := range statuses {
							result.AddCheck("required-secret/"+status.Name, status.Present && status.Usable, map[bool]string{true: "present and usable", false: "missing or unusable"}[status.Present && status.Usable])
						}
					}
				}
			}
			cancel()
		}
		brokerCtx, brokerCancel := context.WithTimeout(ctx, 2*time.Second)
		brokerErr := verifyRuntimeBrokerRunning(brokerCtx, compose, m, files)
		brokerCancel()
		if brokerErr != nil {
			result.AddCheck("runtime-broker", false, brokerErr.Error())
		} else {
			result.AddCheck("runtime-broker", true, "mTLS identity and app-scoped OpenBao readiness succeeded")
		}
	}

	workloadStatus, workloadErr := inspectRepositoryWorkloadStatus(ctx, compose, resolved, files)
	if workloadStatus.Found {
		for _, service := range workloadStatus.Services {
			result.AddCheck("workload/"+service.Service, service.Ready, formatWorkloadServiceStatus(service))
		}
		if workloadErr != nil {
			result.AddCheck("workload", false, workloadErr.Error())
		} else {
			result.AddCheck("workload", workloadStatus.Ready(), fmt.Sprintf("%d/%d selected Compose service(s) ready", workloadStatus.ReadyCount(), len(workloadStatus.Services)))
		}
	} else if workloadErr != nil {
		result.AddCheck("workload", false, "repository Compose integration could not be resolved: "+workloadErr.Error())
	}

	if policy, policyErr := application.LogsPolicy(m); policyErr != nil {
		result.AddCheck("logs", false, policyErr.Error())
	} else if policy.Enabled && policy.Collect[application.LogsSourceApplication] && workloadStatus.Found {
		logServices := make([]string, 0, len(workloadStatus.Services))
		for _, service := range workloadStatus.Services {
			logServices = append(logServices, service.Service)
		}
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := logsprovider.VerifyApplication(checkCtx, m, logServices)
		cancel()
		if err != nil {
			result.AddCheck("logs", false, err.Error())
		} else {
			result.AddCheck("logs", true, fmt.Sprintf("%d workload log stream(s) queryable", len(logServices)))
		}
	}
	if len(m.Exposures) > 0 {
		_, exposureErr := inspectManagedExposure(ctx, compose, m, files)
		if exposureErr != nil {
			result.AddCheck("managed-exposure", false, exposureErr.Error())
		} else {
			result.AddCheck("managed-exposure", true, "configured exposure endpoints are ready")
		}
	}
	return result, nil
}

func renderApplicationStatus(ctx context.Context, out, errOut io.Writer, result application.StatusResult) {
	renderApplicationStatusWithExtra(ctx, out, errOut, result, nil)
}

func renderApplicationStatusWithExtra(ctx context.Context, out, errOut io.Writer, result application.StatusResult, extra func(*cli.Terminal)) {
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(result.Application, result.Environment)
	term.Section("Application")
	if result.State == "stopped" {
		term.Result("STOPPED", "application", "persistent application state preserved")
		fmt.Fprintln(out, "\nSTOPPED")
		return
	}

	term.Result(map[bool]string{true: "READY", false: "DEGRADED"}[result.Ready], "application", map[bool]string{true: "all requested components verified", false: "one or more components need attention"}[result.Ready])

	sections := map[string][]application.StatusCheck{}
	order := []string{"Services", "Workload", "Observability", "Exposure", "Other"}
	for _, check := range result.Checks {
		section := "Other"
		switch {
		case strings.HasPrefix(check.Name, "postgres"), strings.HasPrefix(check.Name, "valkey"), strings.HasPrefix(check.Name, "secrets"), strings.HasPrefix(check.Name, "runtime-broker"), strings.HasPrefix(check.Name, "object-storage"), strings.HasPrefix(check.Name, "required-secret"):
			section = "Services"
		case strings.HasPrefix(check.Name, "workload"):
			section = "Workload"
		case strings.HasPrefix(check.Name, "logs"), strings.HasPrefix(check.Name, "telemetry"), strings.HasPrefix(check.Name, "traces"), strings.HasPrefix(check.Name, "metrics"):
			section = "Observability"
		case strings.Contains(check.Name, "exposure"):
			section = "Exposure"
		}
		sections[section] = append(sections[section], check)
	}
	for _, section := range order {
		checks := sections[section]
		if len(checks) == 0 {
			continue
		}
		term.Section(section)
		for _, check := range checks {
			state := "READY"
			if section == "Observability" {
				state = "VERIFIED"
			}
			if !check.OK {
				state = "FAILED"
			}
			term.Result(state, check.Name, statusHumanDetail(term, check))
		}
	}
	if extra != nil {
		extra(term)
	}

	if result.Ready {
		fmt.Fprintln(out, "\nREADY")
	} else {
		fmt.Fprintln(out, "\nDEGRADED")
		fmt.Fprintln(out, "\nNext:")
		fmt.Fprintln(out, "  baha doctor")
		fmt.Fprintln(out, "  baha status --verbose")
	}
}

func statusHumanDetail(term *cli.Terminal, check application.StatusCheck) string {
	if term.Verbose() || check.OK {
		if !term.Verbose() {
			switch check.Name {
			case "postgres":
				return "authenticated and ready"
			case "valkey":
				return "authenticated and ready"
			case "secrets":
				return "OpenBao application scope ready"
			case "runtime-broker":
				return "mTLS readiness verified"
			case "managed-exposure":
				return "configured endpoint(s) ready"
			}
		}
		return check.Detail
	}

	switch {
	case check.Name == "secrets":
		return "OpenBao application scope unavailable"
	case check.Name == "runtime-broker":
		return "runtime broker is not ready"
	case check.Name == "workload" && strings.Contains(strings.ToLower(check.Detail), "openbao"):
		return "required secrets unavailable because OpenBao is not running"
	case strings.HasPrefix(check.Name, "workload/"):
		return "workload service is not ready"
	case check.Name == "logs":
		return "log ingestion is not ready"
	case check.Name == "telemetry/otlp":
		return "telemetry endpoint is not ready"
	case check.Name == "object-storage":
		return "object storage is not ready"
	default:
		return check.Detail
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
				renderApplicationStatus(ctx, out, errOut, result)
			}
			if result.State == "stopped" {
				return nil
			}
			if !result.Ready {
				return cli.Presented(errors.New("application is not ready"))
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
				var tlsStatus *applicationTLSStatus
				var tlsErr error
				if resolved.FromRepository {
					status, inspectErr := inspectApplicationTLS(resolved)
					if inspectErr != nil {
						tlsErr = inspectErr
					} else if status.State.TLSMode != "" {
						tlsStatus = &status
					}
				}
				if tlsErr != nil {
					ok = false
				}
				renderApplicationDoctor(ctx, out, errOut, m, results, workloadStatus, requiredStatuses, workloadSecurity, ok, tlsStatus, tlsErr)
			}
			if !ok {
				return cli.Presented(errors.New("application doctor found one or more failures"))
			}
			return nil
		},
	}
}

func renderApplicationDoctor(
	ctx context.Context,
	out io.Writer,
	errOut io.Writer,
	m application.Manifest,
	results []preflight.Result,
	workload repositoryWorkloadStatus,
	requiredSecrets []openbao.RequiredSecretStatus,
	workloadSecurity application.WorkloadSecurityReport,
	healthy bool,
	tlsStatus *applicationTLSStatus,
	tlsErr error,
) {
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(m.Name, m.Environment)

	sections := map[string][]preflight.Result{}
	order := []string{"Core", "Services", "Workload", "Observability", "Exposure"}
	for _, result := range results {
		section := applicationDoctorSection(result.Name)
		sections[section] = append(sections[section], result)
	}
	for _, section := range order {
		items := sections[section]
		if len(items) == 0 {
			continue
		}
		term.Section(section)
		for _, result := range items {
			state := "OK"
			if !result.OK {
				state = "FAILED"
			}
			detail := ""
			if !result.OK || term.Verbose() {
				detail = doctorHumanDetail(term, result)
			}
			term.Result(state, result.Name, detail)
		}
	}

	if workload.Found && len(workload.Services) > 0 {
		term.Section("Workload services")
		for _, service := range workload.Services {
			state := "READY"
			if !service.Ready {
				state = "FAILED"
			}
			term.Result(state, service.Service, formatWorkloadServiceStatus(service))
		}
	}

	if len(requiredSecrets) > 0 {
		term.Section("Required secrets")
		for _, status := range requiredSecrets {
			state := "READY"
			detail := "present and usable"
			if !status.Present || !status.Usable {
				state = "MISSING"
				detail = "missing or unusable"
			} else if status.Generated {
				detail = "present and usable · managed generation enabled"
			}
			term.Result(state, status.Name, detail)
		}
	}

	if term.Verbose() {
		printWorkloadSecurityFindings(out, workloadSecurity)
	}
	if tlsStatus != nil || tlsErr != nil {
		term.Section("TLS")
		if tlsErr != nil {
			term.Result("FAILED", "certificate", conciseTLSStatusError(tlsErr))
		} else {
			renderApplicationTLSStatus(term, *tlsStatus)
		}
	}

	if healthy && tlsErr == nil {
		fmt.Fprintln(out, "\nREADY")
		return
	}
	fmt.Fprintln(out, "\nNext:")
	fmt.Fprintln(out, "  baha status --verbose")
	fmt.Fprintln(out, "  baha doctor --verbose")
	fmt.Fprintln(out, "  baha app doctor --fix")
	fmt.Fprintln(out, "\nDEGRADED · one or more checks require attention")
}

func doctorHumanDetail(term *cli.Terminal, result preflight.Result) string {
	if term.Verbose() || result.OK {
		return result.Detail
	}
	lowerName := strings.ToLower(result.Name)
	lowerDetail := strings.ToLower(result.Detail)
	switch {
	case strings.Contains(lowerName, "managed runtime definition"):
		return "runtime definition differs from BaseHarbor-managed state"
	case strings.Contains(lowerName, "openbao application scope"):
		return "OpenBao application scope unavailable"
	case strings.Contains(lowerName, "application runtime broker"):
		return "runtime broker is not ready"
	case strings.Contains(lowerName, "required application secrets"):
		return "required secrets could not be verified because OpenBao is unavailable"
	case strings.Contains(lowerName, "workload security") && strings.Contains(lowerDetail, "required variable"):
		if strings.Contains(lowerDetail, "secret_key") {
			return "workload requires a BaseHarbor/OpenBao managed secret before security preflight can complete"
		}
		return "workload requires a missing environment value before security preflight can complete"
	case strings.Contains(lowerName, "repository workload") && strings.Contains(lowerDetail, "openbao"):
		return "workload cannot resolve required secrets because OpenBao is unavailable"
	default:
		return result.Detail
	}
}

func applicationDoctorSection(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "postgres"),
		strings.Contains(lower, "valkey"),
		strings.Contains(lower, "openbao"),
		strings.Contains(lower, "secret"),
		strings.Contains(lower, "broker"),
		strings.Contains(lower, "object-storage"),
		strings.Contains(lower, "running services"):
		return "Services"
	case strings.Contains(lower, "workload"):
		return "Workload"
	case strings.Contains(lower, "log"),
		strings.Contains(lower, "telemetry"),
		strings.Contains(lower, "otlp"),
		strings.Contains(lower, "metric"),
		strings.Contains(lower, "trace"):
		return "Observability"
	case strings.Contains(lower, "exposure"),
		strings.Contains(lower, "http"),
		strings.Contains(lower, "tls"):
		return "Exposure"
	default:
		return "Core"
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
