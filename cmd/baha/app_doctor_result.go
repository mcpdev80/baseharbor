package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type applicationDoctorWorkloadResult struct {
	Service string `json:"service"`
	Ready   bool   `json:"ready"`
	Detail  string `json:"detail,omitempty"`
}

type applicationDoctorSecretResult struct {
	Name      string `json:"name"`
	Present   bool   `json:"present"`
	Usable    bool   `json:"usable"`
	Generated bool   `json:"generated,omitempty"`
}

type applicationDoctorResult struct {
	ContractVersion string                                       `json:"contract_version"`
	Application     string                                       `json:"application"`
	Environment     string                                       `json:"environment"`
	State           string                                       `json:"state"`
	Healthy         bool                                         `json:"healthy"`
	Checks          []preflight.Result                           `json:"checks"`
	Workload        []applicationDoctorWorkloadResult            `json:"workload,omitempty"`
	RequiredSecrets []applicationDoctorSecretResult              `json:"required_secrets,omitempty"`
	TLS             *applicationTLSObservation                   `json:"tls,omitempty"`
	ServiceTLS      []application.BackendTLSLifecycleObservation `json:"service_tls,omitempty"`

	manifest               application.Manifest
	workloadStatus         repositoryWorkloadStatus
	requiredSecretStatuses []openbao.RequiredSecretStatus
	workloadSecurity       application.WorkloadSecurityReport
	tlsStatus              *applicationTLSStatus
	tlsErr                 error
	serviceTLSErr          error
}

func collectApplicationDoctor(ctx context.Context, store application.Store, args []string) (applicationDoctorResult, error) {
	resolved, err := resolveApplication(ctx, store, args, "doctor")
	if err != nil {
		return applicationDoctorResult{}, err
	}
	m := resolved.Manifest
	result := applicationDoctorResult{
		ContractVersion: machine.ContractVersion,
		Application:     m.Name,
		Environment:     m.Environment,
		State:           "ready",
		Healthy:         true,
		Checks:          []preflight.Result{},
		manifest:        m,
	}

	files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
	if errors.Is(runtimeErr, application.ErrRuntimeNotApplied) {
		result.State = "not_applied"
		result.Healthy = false
		return result, nil
	}
	var serviceTLS []application.BackendTLSLifecycleObservation
	var serviceTLSErr error
	if runtimeErr == nil {
		serviceTLS, serviceTLSErr = application.InspectBackendTLSLifecycle(files, m)
	}

	var compose bhruntime.Compose
	var running []string
	var platformFiles bhruntime.Files
	var requiredStatuses []openbao.RequiredSecretStatus
	var workloadStatus repositoryWorkloadStatus
	var workloadStatusErr error
	var workloadSecurity application.WorkloadSecurityReport

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
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
		{Name: "service TLS lifecycle", Run: func(context.Context) error {
			if runtimeErr != nil {
				return runtimeErr
			}
			if serviceTLSErr != nil {
				return serviceTLSErr
			}
			if !serviceTLSLifecycleHealthy(serviceTLS) {
				return errors.New("one or more service certificates require immediate rotation")
			}
			return nil
		}},
		{Name: "managed runtime definition", Run: func(context.Context) error {
			if runtimeErr != nil {
				return runtimeErr
			}
			return application.CheckManagedRuntimeDefinition(files, m)
		}},
		{Name: "runtime orchestration", Run: func(ctx context.Context) error {
			var err error
			compose, err = detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
			return err
		}},
		{Name: "workload security", Run: func(ctx context.Context) error {
			var err error
			workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, compose, resolved)
			return err
		}},
		{Name: "runtime configuration", Run: func(ctx context.Context) error {
			if runtimeErr != nil {
				return runtimeErr
			}
			return compose.ConfigProject(ctx, files.Project, files.Compose, files.Env)
		}},
		{Name: "running services", Run: func(ctx context.Context) error {
			if runtimeErr != nil {
				return runtimeErr
			}
			var err error
			running, err = compose.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
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

	if application.HasLogsCollection(m) {
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
	if m.Services.SQL {
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
	if m.Services.Cache {
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
			preflight.Check{Name: "OpenBao control-plane runtime", Run: func(ctx context.Context) error {
				var err error
				platformFiles, err = existingTargetRuntimeFiles(ctx)
				if err != nil {
					return err
				}
				state, err := openbao.Inspect(ctx, compose, platformFiles)
				if err != nil {
					return err
				}
				if !state.Initialized {
					return openbao.ErrNotInitialized
				}
				if state.Sealed {
					return openbao.ErrSealed
				}
				return nil
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
			checks = append(checks, preflight.Check{Name: "required application secrets", Run: func(checkCtx context.Context) error {
				if runtimeErr != nil {
					return runtimeErr
				}
				var err error
				requiredStatuses, err = inspectRequiredApplicationSecrets(checkCtx, compose, platformFiles, m, files)
				if err != nil {
					return err
				}
				return openbao.RequireApplicationSecrets(requiredStatuses)
			}})
		}
	}

	results, ok := preflight.Run(checkCtx, checks)
	var tlsStatus *applicationTLSStatus
	var tlsObservation *applicationTLSObservation
	var tlsErr error
	if resolved.FromRepository {
		tlsStatus, tlsObservation, tlsErr = collectApplicationTLSObservation(resolved)
		if tlsErr != nil || (tlsObservation != nil && !tlsObservation.Healthy) {
			ok = false
		}
	}

	workloads := make([]applicationDoctorWorkloadResult, 0, len(workloadStatus.Services))
	for _, service := range workloadStatus.Services {
		workloads = append(workloads, applicationDoctorWorkloadResult{
			Service: service.Service,
			Ready:   service.Ready,
			Detail:  formatWorkloadServiceStatus(service),
		})
	}
	secrets := make([]applicationDoctorSecretResult, 0, len(requiredStatuses))
	for _, status := range requiredStatuses {
		secrets = append(secrets, applicationDoctorSecretResult{
			Name:      status.Name,
			Present:   status.Present,
			Usable:    status.Usable,
			Generated: status.Generated,
		})
	}

	result.Healthy = ok
	if !ok {
		result.State = "degraded"
	}
	result.Checks = results
	result.Workload = workloads
	result.RequiredSecrets = secrets
	result.TLS = tlsObservation
	result.ServiceTLS = serviceTLS
	result.workloadStatus = workloadStatus
	result.requiredSecretStatuses = requiredStatuses
	result.workloadSecurity = workloadSecurity
	result.tlsStatus = tlsStatus
	result.tlsErr = tlsErr
	result.serviceTLSErr = serviceTLSErr
	return result, nil
}
